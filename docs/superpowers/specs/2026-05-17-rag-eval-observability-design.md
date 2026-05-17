# Local RAG Eval Observability Design

## Goal

Improve local, agent-facing observability for RAG eval experiments so a stuck,
failed, or successful eval run can be explained with bounded evidence:
metadata, lifecycle events, upstream failures, and performance signals.

This design targets local development and MCP-driven investigation only. It
does not add production dashboards, production alerting, rate-limit policy, or
rerank reliability changes beyond shared fields such as `eval_id`, rerank
request state, and status transitions.

## Current State

`services/eval` has a FastAPI eval API, SQLite-backed eval/experiment storage,
basic Prometheus metrics, and plain stdlib log lines for run completion and
failure. The run loop calls chat `/search`, chat `/chat`, and a judge model but
does not emit per-item lifecycle events, upstream timings, or structured
correlation fields.

`services/chat` already has stronger observability patterns: shared structured
logging, request IDs, tracing hooks, RAG pipeline metrics, and rerank metrics.
The eval design should align with those patterns instead of inventing a second
style.

`go/eval-mcp-service` can start, wait for, fetch, compare, and summarize eval
runs through the eval API, but it does not summarize observability evidence for
a run.

`go/observability-mcp-service` is read-only and intentionally service
allowlisted. It currently includes `chat`, `ingestion`, and `debug`, but not
`eval`, so local agents cannot inspect eval logs or service evidence through
that MCP. Its direct defaults point at localhost Prometheus/Loki, which can be
unavailable unless local monitoring is running; Grafana gateway mode already
exists and should be the preferred local path when available.

## Recommended Approach

Use structured eval telemetry plus MCP summaries:

1. Add structured lifecycle logs and bounded Prometheus metrics in
   `services/eval`.
2. Extend `go/eval-mcp-service` with a run evidence summary assembled from the
   eval API.
3. Extend `go/observability-mcp-service` so local agents can safely inspect
   `eval` and investigate one eval run by `eval_id`.
4. Make local Prometheus scrape eval/chat/debug metrics correctly for local
   runs, while keeping Grafana gateway mode as the preferred fallback when
   direct local endpoints are down.

This keeps the first slice local, auditable, and useful without requiring
production dashboards or unsafe environment access.

## Eval Logs

`services/eval` should use the shared structured logging setup already used by
`services/chat`: JSON logs with service name, timestamp, level, request context
when available, and trace fields when tracing is configured.

The eval run should emit these lifecycle events:

- `eval_run_started`: emitted before the background run begins upstream work.
- `eval_run_config_captured`: emitted after chat/ingestion config capture
  returns, including whether capture was complete or partial.
- `eval_item_started`: emitted before processing one dataset item.
- `eval_upstream_call_failed`: emitted when chat `/search`, chat `/chat`,
  config capture, or judge calls fail.
- `eval_item_completed`: emitted after retrieval, answer generation, judging,
  and local scoring complete for one item.
- `eval_item_failed`: emitted when a specific item fails before the whole run
  is marked failed.
- `eval_run_completed`: emitted after the DB status is updated to completed and
  run metrics are recorded.
- `eval_run_failed`: emitted after the DB status is updated to failed.
- `eval_run_stale_detected`: emitted by a local inspection path when an
  existing running eval exceeds the stale threshold.

Failures must log bounded error information: error type, upstream service,
operation, and a small failure reason enum. Raw exception strings can remain in
the eval API `error` field for explicit run inspection, but metric labels and
high-cardinality logs should avoid raw error text.

## Correlation Fields

Every eval lifecycle log should include the relevant subset of these fields:

- `eval_id`
- `experiment_id`
- `experiment_label`
- `dataset_id`
- `collection`
- `requested_rerank`
- `baseline_eval_id`
- `item_index`
- `item_count`
- `query_hash`
- `upstream_service`: `chat`, `ingestion`, or `judge`
- `upstream_operation`: `search`, `chat`, `config`, or `judge`
- `status`: `running`, `completed`, or `failed`
- `error_type`
- `failure_reason`

`query_hash` must be a stable short hash of the query. Logs and metrics should
not include raw query text. The eval API may continue returning raw queries in
explicit result payloads because those are already part of run review.

## Metrics

Add bounded eval metrics in `services/eval/app/metrics.py`:

- `eval_run_duration_seconds{status,requested_rerank}`: full background run
  duration.
- `eval_item_duration_seconds{stage,requested_rerank}`: per-item stage
  duration. Stages are `search`, `chat`, `judge`, `score`, and `total`.
- `eval_upstream_request_duration_seconds{service,operation,outcome,requested_rerank}`:
  chat/config/judge upstream request duration.
- `eval_upstream_failures_total{service,operation,reason,requested_rerank}`:
  upstream failures by bounded reason.
- `eval_runs_total{status,requested_rerank}`: completed and failed run count.
- `eval_items_total{status,requested_rerank}`: item success/failure count.
- `eval_stale_running_runs{threshold}`: count of running evals older than the
  threshold used by local inspection.

Keep the existing `eval_ragas_score{metric}` gauge for latest score reporting.
Do not add `eval_id`, raw query, or raw error strings as metric labels.
`dataset_id` and `collection` should not be metric labels in the first slice to
avoid normalizing high-cardinality patterns into the service.

Chat already exposes rerank and RAG metrics:

- `rag_rerank_duration_seconds{model,outcome}`
- `rag_rerank_fallbacks_total{reason}`
- `rag_pipeline_duration_seconds{stage}`
- Qdrant and Ollama latency metrics

Eval metrics should measure eval orchestration and upstream call boundaries;
chat metrics should remain the source for chat internals and reranking cost.

## Eval API And Persistence

Prefer using existing eval API endpoints for evidence assembly. Do not add a
new public endpoint unless MCP cannot produce a useful summary from existing
run and experiment responses.

The existing API already exposes run status, created/completed timestamps,
config, aggregate scores, error, results, and experiment run attachments. The
first implementation should add only the minimal metadata required for
correlation, such as making `requested_rerank` reliably available in the stored
config and passing experiment identifiers into the background task context.

Do not persist every lifecycle event in SQLite in this slice. Structured logs
are the event stream. SQLite remains the source for durable run status, config,
scores, and errors.

## Eval MCP Evidence

Add a `get_eval_run_evidence` tool to `go/eval-mcp-service`.

Input:

- `eval_id`
- optional `stale_after`, defaulting to a conservative local threshold such as
  `30m`

Output:

- `eval_id`
- `status`
- `dataset_id`
- `collection`
- `experiment_id` and `experiment_label` when discoverable
- `baseline_eval_id`
- `requested_rerank`
- `created_at`
- `completed_at`
- `age_seconds`
- `stale_running`
- `stale_after_seconds`
- `error`
- `aggregate_scores`
- `config_captured`
- compact result counts, including total results and scored results
- a short `next_steps` list with safe local actions, such as calling
  observability MCP `investigate_eval_run` when logs/metrics are needed

Extend `wait_for_eval_run` timeout results so timeouts include the latest run
status, age, `eval_id`, and a pointer to `get_eval_run_evidence`. A timeout
means the wait operation timed out; it should not claim the eval run is stuck
unless the run age exceeds the stale threshold.

## Observability MCP Evidence

Add `eval` to the observability MCP service allowlist.

Add an `investigate_eval_run` tool to `go/observability-mcp-service`.

Input:

- `eval_id`
- optional `window`

Behavior:

- Query Loki logs for service `eval` filtered by `eval_id`.
- Query Prometheus eval run, item, upstream, and stale-running metrics.
- Query chat rerank latency and fallback metrics over the same window.
- Include service evidence for `eval` and `chat` when available.
- Return a bounded evidence bundle with source statuses, findings, signals,
  logs, and source errors.

If Prometheus or Loki is unavailable, the tool must return partial evidence
with explicit source errors rather than failing the whole investigation. This
is important because the original debugging failure involved local endpoints
returning connection refused.

Update `investigate_ai_pipeline` so local AI/RAG evidence includes eval logs
and eval metrics alongside chat, ingestion, debug, and Go AI service evidence.

## Local Observability Access

Local agents should have a clear, safe fallback order:

1. Prefer observability MCP Grafana gateway mode for Prometheus and Loki when
   `OBS_GRAFANA_URL` and Cloudflare Access credentials are configured.
2. Use direct localhost Prometheus/Loki only when the local monitoring stack is
   running.
3. If metrics/logs are unavailable, return partial MCP evidence from the eval
   API and explicit source errors.

Update local `monitoring/prometheus.yml` so local Prometheus scrapes `/metrics`
for `chat`, `ingestion`, `debug`, and `eval`. The current local config scrapes
some `/health` endpoints, which does not expose custom metrics.

No production dashboard, Grafana alert, Kubernetes rollout, or shared-cluster
mutation is part of this local-only implementation slice.

## Tests And Smoke Checks

Eval service tests should prove:

- `/metrics` contains the new eval metric families.
- a successful run emits structured start, per-item, and completed log events.
- upstream chat failure emits `eval_upstream_call_failed` and increments the
  bounded failure counter.
- failed runs increment `eval_runs_total{status="failed"}`.
- logs include `query_hash` and do not include raw query text.
- stale-running calculation works against an old running DB row.

Chat tests should prove:

- existing rerank metrics remain present.
- rerank fallback logs use bounded fields such as error type and collection.

Eval MCP tests should prove:

- `get_eval_run_evidence` returns metadata, config, score/error state, result
  counts, age, and stale-running state.
- `wait_for_eval_run` timeout output includes latest status and evidence-tool
  guidance.

Observability MCP tests should prove:

- `eval` is allowlisted.
- `investigate_eval_run` filters logs by `eval_id`.
- Prometheus/Loki connection failures return partial evidence with errors.
- `investigate_ai_pipeline` includes eval evidence.

Local smoke should prove:

- start a local eval run.
- call `wait_for_eval_run`.
- call `get_eval_run_evidence`.
- call observability MCP `investigate_eval_run`.
- verify `/metrics` exposes eval run, item, and upstream metrics after one
  smoke run.

## Non-Goals

- No production Grafana dashboard or alerting.
- No changes to rate-limit policy.
- No rerank reliability fixes beyond observability fields already flowing
  through eval and chat.
- No raw query, `eval_id`, or raw error string labels in Prometheus metrics.
- No Kubernetes mutation as part of the local-only design.
