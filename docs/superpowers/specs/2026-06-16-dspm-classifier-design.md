# DSPM Classifier Service — Design

**Date:** 2026-06-16
**Status:** Approved (brainstorming phase)
**Author:** Kyle Bradshaw (with Claude)
**Purpose:** Interview practice for Senior Software Engineer (Python) at Varonis DAC.
The service is designed to exercise the role's mandatory skills: async Python,
Kafka at scale, Kubernetes-deployable microservices, AWS-compatible storage,
and Gen-AI integration. The domain (sensitive-data classification) mirrors
Varonis's DSPM product surface.

---

## 1. Overview

`dspm-classifier` is a Python microservice that consumes object-storage events
from Kafka, classifies object content for sensitive data (PII, secrets,
financial, health, etc.) via a tiered regex → NER → LLM pipeline, persists
findings to Postgres, and emits findings to a downstream Kafka topic.

It ships as **two K8s deployments built from one Docker image**:

- **worker** — Kafka consumer + classification pipeline (write path).
- **api** — FastAPI read API over the findings index (read path).

This separation lets the two scale independently and gives a clean interview
answer to "how do you scale reads without overloading the LLM workers?"

---

## 2. Architecture

```
S3 (MinIO local / AWS S3 deployed)
        │  ObjectCreated events
        ▼
  Kafka topic: data-events  (keyed by tenant_id, N partitions)
        │
        ▼
  ┌─────────────────────────────────┐
  │  worker (asyncio consumer pool) │
  │   1. fetch object from S3        │
  │   2. regex pass                  │
  │   3. Presidio/spaCy NER pass     │
  │   4. LLM tiered (mock-ollama)    │
  │   5. upsert finding → Postgres   │
  │   6. emit → findings topic       │
  │   7. commit Kafka offset         │
  └─────────────────────────────────┘
        │                    │
        ▼                    ▼
  findings topic       Postgres (findings index)
                              │
                              ▼
                       ┌─────────────────┐
                       │  api (FastAPI)  │
                       │  /findings      │
                       │  /tenants/{id}/ │
                       │  /healthz       │
                       │  /metrics       │
                       └─────────────────┘

  Failure paths:
   - transient → retry topic (exp. backoff)
   - permanent → dead-letter topic (with reason)
```

### Stack

- Python 3.12, async throughout
- `aiokafka` (consumer + producer)
- `asyncpg` + SQLAlchemy 2.x async, Alembic migrations
- `aioboto3` for S3/MinIO (single SDK, swap by `S3_ENDPOINT_URL`)
- `presidio-analyzer` + `spacy` for NER
- LLM via `httpx` against `mock-ollama` (already in repo); real provider is
  a one-env-var swap
- FastAPI + uvicorn for the read API
- `prometheus-client` metrics, `structlog` JSON logs
- Pydantic v2 models, ruff + mypy strict

### AWS positioning

`boto3`/`aioboto3` targets MinIO locally and real AWS S3 in deploy via
`S3_ENDPOINT_URL`. Same SDK, same IAM patterns, zero code change. Matches
the repo's "no paid cloud unless already part of the project" rule while
keeping the AWS interview story intact.

---

## 3. Components

### Directory layout

```
services/dspm-classifier/
├── app/
│   ├── __init__.py
│   ├── config.py              # pydantic-settings, env-driven
│   ├── models/
│   │   ├── events.py          # DataEvent, Finding (pydantic)
│   │   └── db.py              # SQLAlchemy models
│   ├── db/
│   │   ├── session.py         # async engine + session factory
│   │   └── repository.py      # FindingRepo: upsert_finding, list_findings
│   ├── kafka/
│   │   ├── consumer.py        # KafkaConsumerRunner (partition-aware)
│   │   ├── producer.py        # FindingsProducer + RetryProducer + DLQProducer
│   │   └── backpressure.py    # BoundedWorkPool: pause/resume partitions
│   ├── storage/
│   │   └── s3.py              # S3Fetcher (aioboto3)
│   ├── classifiers/
│   │   ├── base.py            # Classifier protocol → ClassificationResult
│   │   ├── regex_pass.py      # SSN, CC, JWT, API keys, AWS keys
│   │   ├── ner_pass.py        # Presidio + spaCy en_core_web_sm
│   │   ├── llm_pass.py        # tiered: only invoked on ambiguous/low-conf
│   │   └── pipeline.py        # orchestrates passes, merges results
│   ├── idempotency.py         # processed_messages table + check-then-process
│   ├── telemetry/
│   │   ├── logging.py         # structlog setup; binds tenant_id/event_id
│   │   └── metrics.py         # Prometheus collectors
│   └── lifespan.py            # shared startup/shutdown (db, kafka, sigterm)
├── worker/
│   └── main.py                # entrypoint: assemble consumer + pipeline
├── api/
│   └── main.py                # FastAPI app
├── migrations/                # Alembic
│   └── versions/
├── tests/
│   ├── unit/
│   ├── integration/
│   └── e2e/
├── Dockerfile                 # multi-stage, single image, two entrypoints
├── requirements.txt
└── README.md
```

### Key boundary decisions

1. **`classifiers/` is a Protocol-based pipeline.** Each pass returns
   `ClassificationResult(matches, confidence, needs_escalation)`. `pipeline.py`
   decides whether to escalate. Each classifier is independently testable; the
   LLM tier is config-disablable without touching pipeline logic.
2. **`kafka/backpressure.py` is its own unit.** It owns an `asyncio.Semaphore`
   of in-flight work and the `consumer.pause(partitions)` / `consumer.resume()`
   calls. The consumer loop just asks "can I dispatch?" — it does not know how
   backpressure works. This is the answer to "what happens when the LLM is
   slow?"

---

## 4. Data flow — happy path

1. **Producer** (out of scope for this service — assume an S3 → Kafka bridge,
   or a small `scripts/` helper for local dev) publishes to `data-events`:
   ```json
   {
     "event_id": "evt_01HZ...",
     "tenant_id": "acme-corp",
     "bucket": "acme-uploads",
     "key": "contracts/2026/q2-deal.pdf",
     "size_bytes": 184321,
     "occurred_at": "2026-06-16T14:22:01Z"
   }
   ```
   Partition key = `tenant_id`. All of one tenant's events are ordered and
   isolated to a single partition.

2. **Consumer** (`worker/main.py`) reads a batch with
   `enable_auto_commit=False` and dispatches each message to the work pool.

3. **Idempotency check** — `idempotency.already_processed(event_id)` hits
   `processed_messages(event_id PK, tenant_id, processed_at, pipeline_version)`.
   On hit, skip and commit offset.

4. **S3 fetch** — `S3Fetcher.fetch(bucket, key)` streams the object, capped at
   `MAX_OBJECT_BYTES` (default 10 MB). Objects over the cap emit a
   `too_large` finding and skip classification.

5. **`Pipeline.run(content, metadata)`**:
   - **Regex pass** — SSN, credit card (Luhn-validated), JWT, AWS access key,
     generic API keys. Cheap, deterministic, high precision.
   - **NER pass** — Presidio + spaCy `en_core_web_sm` for PERSON, EMAIL,
     PHONE_NUMBER, LOCATION, ORG.
   - **LLM pass (tiered)** — invoked only if NER hit with confidence below
     threshold, OR sensitive-document heuristic fires while regex/NER were
     silent. Structured output via Pydantic schema:
     `{category, sensitivity, reasoning, entities[]}`.
   - Pipeline merges matches, dedupes overlapping spans, computes top-level
     `sensitivity` (none/low/medium/high) and `categories` (PII, FINANCIAL,
     SECRETS, HEALTH, …).

6. **Persist** — `FindingRepo.upsert(Finding(...))`. Upsert key
   `(tenant_id, event_id)`. Index `(tenant_id, sensitivity, classified_at DESC)`
   serves the read API.

7. **Emit** — `FindingsProducer.send(findings_topic, key=tenant_id,
   value=Finding.model_dump())`.

8. **Commit offset** — only after persist + emit succeed. At-least-once on the
   wire; the idempotency table makes it effectively-once end-to-end.

9. **Metrics** — `findings_total{tenant_id, sensitivity}`,
   `pipeline_stage_latency_seconds{stage}`, `kafka_consumer_lag{partition}`.

---

## 5. Failure handling & reliability

### Per-stage failure policy

| Failure | Classification | Action |
|---|---|---|
| Idempotency check shows already-processed | expected | skip, commit, `duplicate_total++` |
| S3 fetch: 404 / NoSuchKey | permanent | emit `object_missing` finding, persist, commit |
| S3 fetch: 5xx / network | transient | retry topic with attempt counter |
| Object > MAX_OBJECT_BYTES | permanent | persist `too_large` finding, no classification, commit |
| Regex/NER pass raises | unexpected (bug) | DLQ with traceback; do not retry |
| LLM: timeout / 5xx / 429 | transient | retry topic |
| LLM: malformed JSON after N parse retries | permanent | persist with `llm_failed=true` using regex+NER only, commit |
| Postgres: connection error | transient | back off, do not commit; partition pauses naturally |
| Kafka produce to findings fails | transient | retry the produce; do not commit consume offset |

### Retry topic

`data-events.retry`. Envelope wraps the original event with
`{attempt, first_failed_at, last_error_kind}`. A second consumer reads `retry`
with sleep proportional to `attempt` (1s, 5s, 30s, 2m, 10m). After
`MAX_ATTEMPTS=5` → DLQ.

### Dead-letter topic

`data-events.dlq`. Envelope adds `failure_reason`, `failed_at`, last error
class, truncated traceback. Not consumed in the live system. A small
`scripts/dlq-inspect.py` is out of v1 scope but noted.

### Idempotency

`processed_messages(event_id PK, tenant_id, processed_at, pipeline_version)`.
Insert happens in the **same transaction** as the findings upsert. Duplicate
event → PK violation → transaction rolls back → offset commits. Changing
`pipeline_version` lets a re-classification pass intentionally reprocess
events.

### Backpressure — `BoundedWorkPool(max_in_flight=N)`

- `asyncio.Semaphore(N)` + a set of currently-paused `TopicPartition`s.
- On dispatch: if acquire would block over X ms, call
  `consumer.pause(partitions)`, queue the message, stop polling new ones until
  in-flight drops to the low-water mark.
- On completion: `release()`; if paused and in-flight ≤ low-water, `resume()`.
- `N` is per-pod and derived from `LLM_CONCURRENCY` (the actual bottleneck);
  default 8.

### Graceful shutdown (SIGTERM)

- Stop polling new messages.
- `await asyncio.gather(*in_flight)` with a 25s deadline (K8s default
  `terminationGracePeriodSeconds: 30`).
- Anything still in-flight at deadline: do NOT commit its offset → redelivery
  on restart, idempotency catches it.
- Close producers (flush), then consumer, then DB pool.

### Observability hooks

- `kafka_consumer_lag_seconds{topic, partition}` — bounded cardinality.
- `pipeline_stage_latency_seconds{stage}` histogram.
- `in_flight_work` and `partitions_paused` gauges — backpressure visible.
- `dlq_total{reason}` counter — alertable.
- Structlog binds `tenant_id`, `event_id`, `partition`, `offset` to every log
  in a message's scope via a contextvar.

---

## 6. Testing strategy

### Unit (`tests/unit/`, pytest + pytest-asyncio, no I/O)

- `test_regex_pass.py` — table-driven per pattern; positives (SSN, CC w/ valid
  Luhn, JWT, AWS access key), negatives (SSN-shaped but invalid, expired-looking
  JWT), boundary cases (overlapping spans, unicode).
- `test_ner_pass.py` — Presidio with fixed seeds; assert entity types and span
  offsets on canned text.
- `test_pipeline.py` — escalation logic: regex-only when high-conf; NER → LLM
  when ambiguous; LLM skipped by config; merge/dedupe of overlapping spans.
- `test_llm_pass.py` — `httpx.MockTransport` stubs; happy JSON, malformed JSON
  + retry, timeout, 429.
- `test_backpressure.py` — synthetic semaphore + fake consumer; `pause()` at
  high-water, `resume()` at low-water; no message loss under contention.
- `test_idempotency.py` — duplicate `event_id` rolls back; `pipeline_version`
  change allows reprocessing.
- `test_repository.py` — upsert semantics; `EXPLAIN` assertion that the
  composite index serves the read query.

### Integration (`tests/integration/`, testcontainers; Docker-gated, graceful skip)

- Redpanda (Kafka-compatible, fast), Postgres, MinIO via
  `testcontainers-python`.
- `test_consumer_e2e.py` — produce S3 event to MinIO + Kafka, run worker,
  assert finding row + findings-topic message.
- `test_retry_dlq.py` — inject S3 5xx, assert retry topic delivery with
  `attempt=1`, eventual DLQ after MAX_ATTEMPTS.
- `test_idempotency_e2e.py` — duplicate event → one row, two offset commits,
  `duplicate_total` increments.
- `test_graceful_shutdown.py` — SIGTERM mid-batch; in-flight messages
  redelivered, end up exactly once.

### API (`tests/integration/api/`)

- FastAPI `TestClient` + Postgres container.
- `GET /findings?tenant_id=X&sensitivity=high` — pagination, tenant isolation
  (header swap blocked), filter combinations.
- `GET /tenants/{id}/summary` — aggregation correctness on seeded data.
- `GET /healthz` returns 503 when DB unreachable.

### Load / perf (`loadtest/dspm-classifier/`)

- Locust or asyncio script producing N events/sec across M tenants.
- Measures consumer lag, p50/p95/p99 pipeline latency, in-flight gauge,
  partitions-paused count.
- Target: 500 events/sec sustained with mock-ollama, lag < 10s, no DLQ.
- Not run in CI; README documents the local "show me a graph" demo. Interview
  talking point.

### Coverage

- Unit ≥ 90%.
- Integration covers every failure-path row from §5.

### CI

- Add `services/dspm-classifier/` to `make preflight-python` (ruff, mypy
  strict, pytest unit).
- Integration tests run via `make test-integration`; documentation-only on CI
  unless Docker is available (matches repo's existing
  testcontainers-skip-gracefully pattern).

---

## 7. Deployment & configuration

### Image

Single multi-stage Docker image (`python:3.12-slim` runtime). Default `CMD` is
the worker. The api Deployment overrides `command`/`args` to launch uvicorn.
One image guarantees worker and api stay in lockstep.

### Kubernetes (`k8s/dspm-classifier/`)

- `worker-deployment.yaml` — `replicas: 3` (≤ Kafka partitions),
  `terminationGracePeriodSeconds: 30`, liveness probe on a side-port
  `/healthz`. No Kafka readiness probe — the consumer manages its own
  lifecycle.
- `api-deployment.yaml` — `replicas: 2`, HTTP readiness + liveness on
  `/healthz`.
- `service.yaml` — ClusterIP for the api. No service for the worker.
- `configmap.yaml` — non-secret config: Kafka brokers, topic names, S3
  endpoint, `MAX_OBJECT_BYTES`, `LLM_CONCURRENCY`, pipeline thresholds,
  `pipeline_version`.
- `secret.yaml` (sealed) — Kafka SASL creds, AWS keys, Postgres DSN, LLM API
  key.
- `servicemonitor.yaml` — Prometheus scrape of `/metrics` on both deployments.
- `hpa.yaml` (worker) — HPA on consumer-lag via prometheus-adapter; documented
  CPU-HPA fallback.
- `migrations-job.yaml` — Alembic migration as a pre-deploy K8s Job.

### Configuration surface (`app/config.py`, pydantic-settings)

```
KAFKA_BROKERS, KAFKA_CONSUMER_GROUP, KAFKA_TOPIC_EVENTS,
KAFKA_TOPIC_FINDINGS, KAFKA_TOPIC_RETRY, KAFKA_TOPIC_DLQ
S3_ENDPOINT_URL (empty = real AWS), S3_REGION, AWS_* via standard SDK chain
POSTGRES_DSN
LLM_BASE_URL, LLM_MODEL, LLM_TIMEOUT_S, LLM_CONCURRENCY
MAX_OBJECT_BYTES, MAX_RETRY_ATTEMPTS, PIPELINE_VERSION
NER_CONFIDENCE_THRESHOLD, ESCALATE_TO_LLM (bool)
LOG_LEVEL, METRICS_PORT
```

### Local dev

- `docker-compose.yml` additions: redpanda, postgres, minio, mock-ollama
  (already exists).
- `make dev-dspm` — bring services up, apply migrations, run worker + api with
  reload, seed a tenant and a sample S3 object.

---

## 8. Out of v1 scope (documented in README)

- AuthN/AuthZ on the read API (header-trusted `tenant_id` for now).
- TTL job for `processed_messages`.
- DLQ inspector CLI.
- Real LLM provider wiring (mock-ollama covers the seam).
- Multi-region / cross-cluster Kafka mirroring.

---

## 9. Interview talking points this design enables

- **Async + performance-critical**: backpressure design, work pool, partition
  pause/resume, tiered classifier so cheap path stays cheap.
- **Kafka at scale**: tenant-keyed partitioning, consumer group, manual
  commits + idempotency table for effectively-once, retry topic with backoff,
  DLQ, graceful shutdown without redelivery storms.
- **Kubernetes/microservices**: shared-image two-deployment topology, HPA on
  consumer lag, sealed-secret pattern, migration Job, ServiceMonitor.
- **AWS**: aioboto3 against real S3 in deploy, same SDK against MinIO locally,
  standard credential chain.
- **Gen-AI**: structured-output LLM pass with Pydantic schema, retry/parse
  loop, cost-aware tiering, model swap by env var.
- **DSPM domain**: PII/SECRETS/FINANCIAL/HEALTH categories, sensitivity
  levels, tenant-scoped findings, `pipeline_version` for re-classification.
- **Production rigor**: idempotency, DLQ, structured logs with bound
  contextvars, Prometheus metrics that map to alerts, graceful shutdown that
  respects K8s lifecycle.
