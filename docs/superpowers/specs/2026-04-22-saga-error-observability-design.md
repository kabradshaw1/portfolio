# Saga Error Observability: Metrics, Logging & Alerting

## Context

During a checkout failure investigation on QA, the order-service saga was silently failing due to a `VARCHAR(20)` overflow on `saga_step`. The debugging session required ~10 Loki queries and explored TLS certs, SANs, ingress config, and service configs before identifying the root cause. Three observability gaps made this harder than necessary:

1. **No proactive alerting** — the `order-postgres` circuit breaker was tripping repeatedly, 10 messages accumulated in the saga DLQ, and orders were failing — all with zero alerts fired.
2. **Saga error metrics exist but are unused** — `saga_steps_total` has an `outcome` label but only records `"success"`. `saga_dlq_messages_total` is declared in code but never incremented.
3. **Consumer error logs lack context** — the saga consumer logs `"saga event handling failed"` with only `error` and `requeue` fields — no orderID, event type, or routing key — making correlation across log lines manual and slow.

This is a **companion spec** to `2026-04-22-observability-gaps-design.md`, which covers payment metrics, cart/product logging, Promtail fixes, per-service alerts, and decomposed service dashboards. That spec explicitly excludes DLQ alerting and does not address saga error metrics or consumer logging.

**Goal:** Make saga failures self-diagnosing — any saga error should fire an alert, appear on a dashboard, and produce a log line with enough context to identify the failing order and step without follow-up queries.

**Scope:** Order-service saga code (orchestrator, consumer) and Grafana alerting/dashboards. No changes to other Go services, Python services, or Promtail.

**Execution order:** This spec should execute **before** the observability-gaps spec. The Go code changes are entirely in `go/order-service/internal/saga/` (untouched by the other spec). Dashboard panels use IDs 26–29 and alert rules use unique UIDs — the other spec starts numbering after these.

## Design

### 1. Populate `outcome="error"` on Saga Step Metrics

**File:** `go/order-service/internal/saga/orchestrator.go`

The `SagaStepsTotal` counter already has `step` and `outcome` labels, but only `"success"` is ever recorded. Add `SagaStepsTotal.WithLabelValues(step, "error").Inc()` at each error/compensation path:

| Location | Step label | Trigger |
|----------|-----------|---------|
| `handleItemsReserved` before `compensate()` | `ITEMS_RESERVED` | Stock insufficient |
| `handleStockValidated` before `compensate()` | `STOCK_VALIDATED` | CreatePayment failed |
| `HandleEvent` case `EvtPaymentFailed` | `PAYMENT_CREATED` | Payment failed event |
| `compensate` when `RefundPayment` fails | `refund` | Refund error during compensation |
| `Advance` default case | value of `order.SagaStep` | Unknown saga step |
| `HandleEvent` default case | `unknown_event` | Unknown event type |

No changes to `metrics.go` — the metric definition already supports this.

### 2. Increment `SagaDLQTotal` on DLQ Nack

**File:** `go/order-service/internal/saga/consumer.go`

In `Start()`, when `requeue` is `false` the message goes to the DLQ via the dead-letter exchange. Call `SagaDLQTotal.Inc()` before `msg.Nack(false, false)`. This is the only code path that sends messages to the DLQ.

### 3. Enrich Consumer Error Logging

**File:** `go/order-service/internal/saga/consumer.go`

Two changes:

**a) Change `handleMessage` signature** from `error` to `(Event, error)`. It already parses the event at line 73 — return it alongside the error. On unmarshal failure, return the zero-value `Event` (safe to log with empty fields).

**b) Enrich the error log** in `Start()` with the parsed event context:

```go
slog.ErrorContext(ctx, "saga event handling failed",
    "error", err,
    "requeue", requeue,
    "orderID", evt.OrderID,
    "event", evt.Event,
    "routingKey", msg.RoutingKey,
)
```

This enables Loki queries like `{app="order-service"} | json | orderID="<uuid>"` to surface all errors for a specific order in a single query.

### 4. DLQ Accumulation Alert

**File:** `k8s/monitoring/configmaps/grafana-alerting.yml`

New rule in the "Application SLOs" group:

| Field | Value |
|-------|-------|
| UID | `saga-dlq-accumulating` |
| Title | Saga DLQ Messages Accumulating |
| Condition | `increase(saga_dlq_messages_total[10m]) > 0` |
| Duration | 2 min |
| Severity | warning |
| Datasource UID | `PBFA97CFB590B2093` |

**Rationale:** Any sustained DLQ activity means stuck orders requiring attention. Deliberately sensitive — the 10-minute increase window smooths transient nacks while the 2-minute `for` duration prevents single-event false positives. The existing spec's `circuit-breaker-open` alert covers the upstream cause; this covers the downstream symptom.

### 5. Saga Step Error Rate Alert

**File:** `k8s/monitoring/configmaps/grafana-alerting.yml`

New rule in the "Application SLOs" group:

| Field | Value |
|-------|-------|
| UID | `saga-step-error-rate` |
| Title | Saga Step Error Rate High |
| Condition | `sum(rate(saga_steps_total{outcome="error"}[5m])) / sum(rate(saga_steps_total[5m])) * 100 > 10` |
| Duration | 5 min |
| Severity | warning |
| Datasource UID | `PBFA97CFB590B2093` |

### 6. Saga Error Analysis Dashboard Row

**File:** `k8s/monitoring/configmaps/grafana-dashboards.yml` (go-services.json section)

New row "Saga Error Analysis" appended after the existing "Streaming Analytics" row. Uses panel IDs 26–29.

| Panel (ID) | Type | Query | Purpose |
|------------|------|-------|---------|
| Row header (26) | row | — | Collapsible section header |
| Saga Step Error Rate (27) | timeseries | `rate(saga_steps_total{outcome="error"}[5m])` by step, stacked with success rate | Identify which steps fail and at what rate |
| DLQ Accumulation (28) | timeseries | `increase(saga_dlq_messages_total[5m])` + `rate(saga_dlq_replayed_total[5m])` by outcome | Track DLQ depth and replay activity |
| Saga Duration (29) | timeseries | `histogram_quantile(0.5/0.95/0.99, rate(saga_duration_seconds_bucket[5m]))` | Detect slow sagas, spot latency regressions |

## Out of Scope

- RabbitMQ management API exporter — not needed, `saga_dlq_messages_total` is sufficient
- Saga recovery/replay automation — the DLQ admin API already exists (`GET/POST /admin/dlq/`)
- PostgreSQL query tracing — high effort, incremental value (same rationale as parent spec)
- Changes to payment, cart, or product services — covered by the parent spec

## Verification

1. **Metrics:** `curl order-service:8092/metrics | grep saga_steps_total` — confirm both `outcome="success"` and `outcome="error"` labels present
2. **DLQ metric:** `curl order-service:8092/metrics | grep saga_dlq_messages_total` — confirm counter increments when a message is nacked to DLQ
3. **Logging:** Trigger a saga failure → query Loki `{app="order-service"} |= "saga event handling failed" | json` → confirm `orderID`, `event`, and `routingKey` fields are present in the log line
4. **Alerts:** Open Grafana Alerting UI → verify `saga-dlq-accumulating` and `saga-step-error-rate` rules appear in "Application SLOs" group with correct Prometheus queries
5. **Dashboard:** Open Go Services dashboard → verify "Saga Error Analysis" row renders with 3 panels (may show "No data" without traffic, which is expected)
6. **End-to-end:** Trigger a checkout on QA with insufficient stock → verify the saga compensates, the step error metric increments, the DLQ metric increments if the breaker trips, and the enriched error log appears in Loki with full context
