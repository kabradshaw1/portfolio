# Observability Remaining Gaps Roadmap

Date: 2026-05-07

## TL;DR

The original observability ADR listed seven remaining gaps. As of this handoff,
one is fully addressed, three are partially addressed, and three remain open.

Prioritize this order:

1. RabbitMQ queue depth metrics and saga DLQ depth alerting.
2. End-to-end verification of Python `trace_id` parsing in Loki.
3. `summarize_orders` sub-LLM timing.
4. Optional `TruncateHistory()` context propagation.
5. Optional PostgreSQL manual spans, only if trace-level SQL visibility becomes
   more valuable than the current `pg_stat_statements` and `auto_explain`
   coverage.

## Current Status

| Gap | Status | Notes |
| --- | --- | --- |
| PostgreSQL query tracing | Partially addressed | `pg_stat_statements`, `auto_explain`, postgres exporter queries, dashboards, and alerts exist. Manual OTel spans are still absent. |
| RabbitMQ queue depth metrics | Open | Prometheus has Spring RabbitMQ client counters/gauges, but no broker queue depth series such as `rabbitmq_queue_messages`. |
| Saga DLQ depth alerts | Partially addressed | Alert exists for `increase(saga_dlq_messages_total[10m])`; this catches new DLQ events, not accumulated queue depth. |
| Promtail `trace_id` field fix for Python services | Partially addressed | Config maps Python `trace_id` into `traceID`; end-to-end QA verification is still needed. |
| Grafana dashboard for AI agent debugging | Addressed | `AI Pipeline` dashboard includes agent/tool panels. |
| `summarize_orders` sub-LLM call timing | Open | The sub-call is still folded into overall tool duration. |
| `TruncateHistory()` logs without context | Open, low priority | Still logs via `slog.Info` without `ctx`. Existing ADR accepted this as minor. |

## Phase 1: RabbitMQ Broker Metrics

Goal: expose broker-level queue depth metrics for prod and QA RabbitMQ queues.

Recommended approach:

- Add the RabbitMQ Prometheus plugin or a dedicated RabbitMQ exporter to the
  Kubernetes RabbitMQ deployment.
- Scrape broker metrics from Prometheus with labels that include namespace,
  vhost, and queue.
- Confirm metrics include at least ready, unacked, and total messages per queue.

Likely files:

- `java/k8s/deployments/rabbitmq.yml`
- `java/k8s/services/rabbitmq.yml`
- `k8s/monitoring/configmaps/prometheus-config.yml`
- `k8s/overlays/qa-java/kustomization.yaml` if QA needs different service
  wiring or vhost label handling.

Acceptance checks:

- Prometheus query returns queue depth series:
  `rabbitmq_queue_messages` or equivalent exporter metric.
- Series distinguish prod and QA vhosts.
- The saga DLQ queue is visible by queue label.
- No live RabbitMQ mutation is required outside repo-managed manifests.

Verification:

```bash
make preflight
```

If the full sweep is too broad for the change, run the relevant Kubernetes YAML
checks already covered by the repo preflight path and leave cluster verification
to the deploy environment.

## Phase 2: Saga DLQ Depth Alert

Goal: alert on accumulated DLQ depth, not just newly dead-lettered messages.

Recommended approach:

- Keep the existing `saga-dlq-accumulating` alert based on
  `increase(saga_dlq_messages_total[10m])`; it catches fresh failures.
- Add a second alert for current queue depth once broker metrics exist.
- Use a threshold that avoids noise during short replay/testing windows.

Likely files:

- `k8s/monitoring/configmaps/grafana-alerting.yml`
- `monitoring/grafana/dashboards/go-services.json`
- `k8s/monitoring/configmaps/grafana-dashboards.yml`

Candidate PromQL shape:

```promql
max by (namespace, vhost, queue) (
  rabbitmq_queue_messages{queue="ecommerce.saga.dlq"}
) > 0
```

Adjust metric and labels to the exporter chosen in Phase 1.

Acceptance checks:

- Existing event-rate DLQ alert remains.
- New depth alert fires when messages sit in `ecommerce.saga.dlq`.
- New depth alert resolves when DLQ is empty.
- Dashboard shows current DLQ depth for prod and QA.

## Phase 3: Python Trace ID Verification

Goal: prove Python service log records can be queried in Loki by trace ID.

Standalone spec:

- `docs/handoffs/python-trace-id-loki-verification-spec.md`

Current implementation:

- Python logging emits `trace_id` and `span_id` from OpenTelemetry context.
- Promtail parses both Go `traceID` and Python `trace_id`, then normalizes into
  `traceID`.

Known blocker from this handoff:

- `scripts/loki-query --ns ai-services --app chat --hours 24` returned HTTP
  500 from Loki during verification. Treat Loki health/query failure as the
  first diagnostic step, not as evidence the Promtail mapping is broken.

Recommended verification path:

1. Trigger or find a Go ai-service request that calls a Python AI service.
2. Query Go logs for the parent request and copy the `traceID`.
3. Query Python service logs in Loki for the same trace ID.
4. Confirm the same trace can be opened in Jaeger.

Likely files if a fix is needed:

- `k8s/monitoring/configmaps/promtail-config.yml`
- `services/shared/logging.py`
- `services/shared/tracing.py`
- `scripts/loki-query`

Acceptance checks:

- Python logs show a non-empty `traceID` label or field in Loki.
- `scripts/loki-query` prints `traceID` for Python services, not just Go
  services.
- At least one QA request correlates across Go ai-service and a Python service.

## Phase 4: `summarize_orders` Sub-LLM Timing

Goal: split the sub-LLM call duration from total tool duration.

Recommended approach:

- Add a timer around `t.llm.Chat(...)` inside `summarizeOrdersTool.Call`.
- Log `sub_llm_duration_ms` on success and failure.
- If there is an existing AI tool metric abstraction for tool duration only,
  keep it unchanged and add a targeted histogram only if the extra metric has a
  clear dashboard use.

Likely files:

- `go/ai-service/internal/tools/orders.go`
- `go/ai-service/internal/tools/orders_test.go` if present or a nearby tool
  test file.
- `go/ai-service/internal/metrics/metrics.go` only if adding a metric.
- `monitoring/grafana/dashboards/ai-pipeline.json` only if adding a metric
  panel.

Minimal implementation shape:

```go
llmStart := time.Now()
resp, err := t.llm.Chat(ctx, []llm.Message{
    {Role: llm.RoleUser, Content: prompt},
}, nil)
subLLMDuration := time.Since(llmStart)
```

Acceptance checks:

- Success log includes both `duration_ms` and `sub_llm_duration_ms`.
- Failure log for sub-LLM errors includes `sub_llm_duration_ms`.
- Existing overall `ai_tool_duration_seconds` behavior remains unchanged.

Verification:

```bash
make preflight-go
```

## Phase 5: `TruncateHistory()` Context Propagation

Goal: include trace context on the history truncation log if the function can
accept `ctx` without awkward call-site churn.

Current state:

- `TruncateHistory(msgs []llm.Message, n int)` logs with `slog.Info`.
- The function does not receive `ctx`.
- The original ADR accepted this because it is called once per turn and the
  turn-start log provides correlation.

Recommended approach:

- Only do this if touching the guardrails or agent loop already.
- Change the signature to `TruncateHistory(ctx context.Context, msgs []llm.Message, n int)`.
- Replace `slog.Info` with `slog.InfoContext`.
- Update all call sites and tests in one small patch.

Likely files:

- `go/ai-service/internal/guardrails/history.go`
- `go/ai-service/internal/agent/agent.go`
- `go/ai-service/internal/guardrails/history_test.go`

Acceptance checks:

- History truncation logs include trace context under normal agent requests.
- Tests cover behavior with and without a system message.
- No unrelated agent-loop behavior changes.

Verification:

```bash
make preflight-go
```

## Phase 6: PostgreSQL Manual Spans Decision

Goal: decide whether manual SQL spans are still worth implementing after the
new PostgreSQL dashboards and alerts.

Current coverage:

- `pg_stat_statements` provides aggregated query timing and call counts.
- `auto_explain` emits slow plans.
- Postgres exporter custom queries expose query performance metrics.
- Grafana dashboards and alerts exist for query performance.

Recommendation:

- Do not add manual spans immediately.
- Revisit only if debugging requires per-request trace waterfalls that show
  specific SQL operations inside the Go service traces.
- If implemented, start with one high-value service path, probably
  order-service checkout/reporting, instead of wrapping every repository.

Possible implementation options:

- Use pgx query tracing if compatible with the current pgxpool setup.
- Add narrow manual spans around selected repository methods.
- Avoid recording raw SQL with user data; prefer operation names and stable
  query identifiers.

Acceptance checks if pursued:

- Spans appear under the originating HTTP/gRPC trace.
- Span attributes do not include sensitive values.
- Tests use `tracetest.NewInMemoryExporter()` for span assertions.
- The implementation does not require replacing the established pgxpool wiring.

Verification:

```bash
make preflight-go
```

## Suggested PR Breakdown

1. RabbitMQ broker metrics and Prometheus scrape.
2. Saga DLQ depth dashboard and alert.
3. Python trace ID verification/fix, including `scripts/loki-query` adjustment
   if needed.
4. `summarize_orders` sub-LLM timing.
5. Optional `TruncateHistory()` context propagation.
6. Optional PostgreSQL manual spans proof of concept.

Keep each PR small enough to verify locally and in QA. Avoid combining
RabbitMQ broker instrumentation with Go AI-service code changes.
