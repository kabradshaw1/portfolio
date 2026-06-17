import type {
  BuildState,
} from "@/components/dspm/BuildStateBadge";
import type { FailureRow } from "@/components/dspm/FailurePolicyTable";
import type { MetricCardData } from "@/components/dspm/MetricsGrid";
import type { PipelineStageCardProps } from "@/components/dspm/PipelineStageCard";

export const planStatuses: readonly {
  plan: 1 | 2 | 3;
  status: BuildState;
  label: string;
}[] = [
  { plan: 1, status: "in-progress", label: "Plan 1 — engine (in progress)" },
  { plan: 2, status: "designed", label: "Plan 2 — Kafka worker (designed)" },
  { plan: 3, status: "designed", label: "Plan 3 — FastAPI read API (designed)" },
];

export const sectionPlanStatus: Record<
  "architecture" | "pipeline" | "reliability" | "deployment" | "observability",
  { plan: 1 | 2 | 3; status: BuildState }
> = {
  architecture: { plan: 2, status: "designed" },
  pipeline: { plan: 1, status: "in-progress" },
  reliability: { plan: 2, status: "designed" },
  deployment: { plan: 3, status: "designed" },
  observability: { plan: 2, status: "designed" },
};

export const architectureDiagram = `flowchart TD
  subgraph Ingest["Ingest"]
    direction LR
    S3["S3 / MinIO<br/><i>aioboto3 · S3_ENDPOINT_URL swap</i>"]
    EVENTS["Kafka topic: data-events<br/><i>partition key = tenant_id</i>"]
  end

  subgraph Process["Process (worker)"]
    direction TB
    CONS["Consumer pool<br/><i>aiokafka · manual commit</i>"]
    FETCH["S3 fetch<br/><i>cap = MAX_OBJECT_BYTES</i>"]
    IDEM["Idempotency check<br/><i>processed_messages PK</i>"]
    PIPE["Pipeline.run<br/><i>regex → NER → LLM</i>"]
  end

  subgraph Persist["Persist"]
    direction LR
    PG[("Postgres<br/><i>findings index</i>")]
    FOUT["Kafka topic: findings"]
  end

  subgraph Read["Read (api)"]
    direction LR
    API["FastAPI<br/><i>/findings · /tenants/:id · /healthz</i>"]
  end

  subgraph Fail["Failure paths"]
    direction LR
    RETRY["data-events.retry<br/><i>attempt + backoff</i>"]
    DLQ["data-events.dlq<br/><i>final · alertable</i>"]
  end

  S3 -->|ObjectCreated| EVENTS
  EVENTS --> CONS
  CONS --> FETCH --> IDEM --> PIPE
  PIPE -->|upsert| PG
  PIPE -->|emit| FOUT
  PG --> API
  PIPE -.transient.-> RETRY
  RETRY -.exhausted.-> DLQ
  PIPE -.permanent.-> DLQ`;

export const pipelineDiagram = `flowchart LR
  IN["Object content"] --> REGEX["Regex pass<br/><i>SSN, CC+Luhn, JWT, AWS</i>"]
  REGEX --> NER["NER pass<br/><i>Presidio · spaCy</i>"]
  NER -- "conf ≥ threshold" --> MERGE["Merge + dedupe spans"]
  NER -- "conf < threshold or silent" --> LLM["LLM pass<br/><i>structured JSON</i>"]
  LLM --> MERGE
  REGEX -.always passes through.-> NER
  MERGE --> OUT["Finding<br/><i>sensitivity · categories</i>"]`;

export const retryDlqDiagram = `sequenceDiagram
  participant W as Worker
  participant E as data-events
  participant R as data-events.retry
  participant D as data-events.dlq
  participant LLM as LLM tier

  E->>W: event_id=evt_X
  W->>LLM: classify
  LLM-->>W: 503 / timeout
  W->>R: envelope(attempt=1, last_error=timeout)
  Note over R: sleep ∝ attempt (1s · 5s · 30s · 2m · 10m)
  R->>W: redeliver (attempt=2)
  W->>LLM: classify
  LLM-->>W: 503
  W->>R: envelope(attempt=2)
  Note over W: ... up to MAX_ATTEMPTS=5
  W->>D: envelope(failure_reason, traceback)`;

export const k8sTopologyDiagram = `flowchart TD
  IMG["Docker image<br/><i>multi-stage · python:3.12-slim</i>"]
  subgraph Cluster["Kubernetes cluster"]
    direction TB
    WORKER["Deployment: worker<br/><i>replicas ≤ partitions</i>"]
    API["Deployment: api<br/><i>replicas = 2</i>"]
    SVC["Service (ClusterIP)<br/><i>api only</i>"]
    CM["ConfigMap<br/><i>brokers · topics · thresholds</i>"]
    SEC["Sealed Secret<br/><i>SASL · AWS · Postgres · LLM key</i>"]
    JOB["Job: alembic upgrade head"]
    SM["ServiceMonitor<br/><i>/metrics scrape</i>"]
    HPA["HPA<br/><i>consumer-lag via prometheus-adapter</i>"]
  end

  IMG --> WORKER
  IMG --> API
  API --> SVC
  CM --> WORKER
  CM --> API
  SEC --> WORKER
  SEC --> API
  JOB -->|pre-deploy| WORKER
  SM --> WORKER
  SM --> API
  HPA --> WORKER`;

export const pipelineStages: readonly PipelineStageCardProps[] = [
  {
    name: "Regex",
    tagline:
      "Deterministic, high-precision pass for known-shape secrets and identifiers.",
    detects: [
      "SSN (rejects 000/666/9xx and 00 group)",
      "Credit card (Luhn-validated)",
      "JWT (3-segment base64url)",
      "AWS access key (AKIA/ASIA prefix)",
      "Generic API key (assignment pattern)",
    ],
    cost: "cheap",
    escalatesWhen:
      "Never escalates further on its own — always passes through to NER so it can contribute entities to the merged span set.",
    example: {
      input: "Card 4111 1111 1111 1111, key AKIAIOSFODNN7EXAMPLE",
      matches: ["FINANCIAL.CREDIT_CARD", "SECRETS.AWS_KEY"],
    },
  },
  {
    name: "NER",
    tagline:
      "Presidio + spaCy named-entity recognition for PII the regex tier cannot model.",
    detects: [
      "PERSON, EMAIL, PHONE, LOCATION",
      "IP_ADDRESS, IBAN, US_SSN (cross-check)",
      "MEDICAL_LICENSE",
    ],
    cost: "moderate",
    escalatesWhen:
      "Any returned entity score falls below NER_CONFIDENCE_THRESHOLD (default 0.6), or the sensitive-document heuristic fires while regex/NER were silent.",
    example: {
      input: "Contact Alice at alice@example.com or 415-555-1212.",
      matches: ["PII.PERSON", "PII.EMAIL", "PII.PHONE"],
    },
  },
  {
    name: "LLM",
    tagline:
      "Tiered LLM pass invoked only on ambiguous content, with structured JSON output.",
    detects: [
      "Category (PII / FINANCIAL / HEALTH / SECRETS / NONE)",
      "Sensitivity (none / low / medium / high)",
      "Free-form entities + reasoning",
    ],
    cost: "expensive",
    escalatesWhen:
      "Final tier — never escalates further. On parse failure or 5xx after retries, pipeline persists the finding with llm_failed=true using regex + NER results only.",
    example: {
      input:
        '"Re: signed NDA — please share the new contract on the shared drive."',
      matches: ["PII (medium)", "reasoning: contract context + names"],
    },
  },
];

export const failurePolicy: readonly FailureRow[] = [
  {
    failure: "Idempotency check shows already-processed",
    classification: "expected",
    action: "Skip, commit offset, increment duplicate_total.",
  },
  {
    failure: "S3 fetch: 404 / NoSuchKey",
    classification: "permanent",
    action: "Emit object_missing finding, persist, commit.",
  },
  {
    failure: "S3 fetch: 5xx / network",
    classification: "transient",
    action: "Send to retry topic with attempt counter.",
  },
  {
    failure: "Object > MAX_OBJECT_BYTES",
    classification: "permanent",
    action: "Persist too_large finding, no classification, commit.",
  },
  {
    failure: "Regex / NER pass raises",
    classification: "unexpected",
    action: "DLQ with traceback; do not retry (treat as a code bug).",
  },
  {
    failure: "LLM: timeout / 5xx / 429",
    classification: "transient",
    action: "Send to retry topic.",
  },
  {
    failure: "LLM: malformed JSON after N parse retries",
    classification: "permanent",
    action:
      "Persist finding with llm_failed=true using regex + NER results; commit.",
  },
  {
    failure: "Postgres: connection error",
    classification: "transient",
    action:
      "Back off; do not commit offset; partition pauses naturally via backpressure.",
  },
  {
    failure: "Kafka produce to findings fails",
    classification: "transient",
    action: "Retry the produce; do not commit consume offset.",
  },
];

export const metrics: readonly MetricCardData[] = [
  {
    name: "findings_total",
    labels: ["tenant_id", "sensitivity"],
    meaning: "Counter of classifications produced, partitioned per tenant and sensitivity.",
    alertIntent: "Volume sanity — sudden drops mean the pipeline is starved or stuck.",
  },
  {
    name: "pipeline_stage_latency_seconds",
    labels: ["stage"],
    meaning: "Per-tier latency histogram (regex / ner / llm).",
    alertIntent: "LLM-tier slow path: alert on p95 sustained above SLO.",
  },
  {
    name: "kafka_consumer_lag_seconds",
    labels: ["topic", "partition"],
    meaning: "End-to-end backlog measured from producer timestamp to consumer commit.",
    alertIntent: "Feeds the HPA; pages oncall above a hard ceiling.",
  },
  {
    name: "in_flight_work",
    labels: [],
    meaning: "Gauge of concurrent classifications currently held by the work pool.",
    alertIntent: "Backpressure visibility — sustained pinning means the LLM tier is the bottleneck.",
  },
  {
    name: "partitions_paused",
    labels: [],
    meaning: "Gauge of TopicPartitions currently paused by the BoundedWorkPool.",
    alertIntent: "Sustained pause across partitions = consumer is overwhelmed, scale or shed load.",
  },
  {
    name: "dlq_total",
    labels: ["reason"],
    meaning: "Counter of permanent failures sent to the dead-letter topic.",
    alertIntent: "Alertable on any non-zero rate; reason label tells you which path failed.",
  },
  {
    name: "duplicate_total",
    labels: [],
    meaning: "Counter of events rejected by the idempotency table.",
    alertIntent: "Replay health — sustained increase signals upstream is replaying without intent.",
  },
];

export const cliOutputExample = `$ python scripts/classify_one.py s3://acme-uploads/contracts/q2.pdf
[regex]   matched 1 (FINANCIAL.CREDIT_CARD)            t=2.1ms
[ner]     matched 3 (PII.PERSON, PII.EMAIL, PII.PHONE) conf=0.92  t=87ms
[llm]     skipped (NER confidence above threshold)
─────────────────────────────────────────────
finding   tenant=acme-corp  event=evt_01HZABCXYZ  sensitivity=HIGH
          categories=[PII, FINANCIAL]  match_count=4
          pipeline_version=1  llm_failed=false
persisted to findings(tenant_id=acme-corp, event_id=evt_01HZABCXYZ)`;

export const interviewBullets: readonly { tag: string; body: string }[] = [
  {
    tag: "Async + perf",
    body: "Backpressure via BoundedWorkPool (semaphore + partition pause/resume), tiered classifier so the cheap path stays cheap, asyncio throughout.",
  },
  {
    tag: "Kafka at scale",
    body: "Tenant-keyed partitioning, consumer group, manual commits + idempotency table for effectively-once, retry topic with exponential backoff, DLQ, graceful shutdown without redelivery storms.",
  },
  {
    tag: "K8s / microservices",
    body: "Shared-image two-deployment topology, HPA on consumer lag via prometheus-adapter, sealed-secret pattern, Alembic migration Job, ServiceMonitor for scrape.",
  },
  {
    tag: "AWS",
    body: "aioboto3 against real S3 in deploy and MinIO locally; same SDK, standard credential chain, IAM patterns unchanged.",
  },
  {
    tag: "Gen-AI",
    body: "Structured-output LLM pass with Pydantic schema, parse/retry loop, cost-aware tiering, model swap by env var.",
  },
  {
    tag: "DSPM domain",
    body: "PII / SECRETS / FINANCIAL / HEALTH categories, sensitivity levels, tenant-scoped findings, pipeline_version for re-classification passes.",
  },
  {
    tag: "Production rigor",
    body: "Idempotency, DLQ, structured logs with bound contextvars, Prometheus metrics mapped to alerts, graceful shutdown respecting the K8s lifecycle.",
  },
];
