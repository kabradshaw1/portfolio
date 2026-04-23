# Saga Error Observability: Metrics, Logging & Alerting

- **Date:** 2026-04-22
- **Status:** Accepted

## Context

During the checkout failure investigation (saga_step VARCHAR overflow), several observability gaps made debugging harder than necessary:

1. **No proactive alerting.** The `order-postgres` circuit breaker was tripping, 10 messages accumulated in the saga DLQ, and orders were failing — all silently. The Telegram bot token was also invalid (separate issue), but even with working notifications, no alert rules existed for saga-specific failures.

2. **Saga error metrics existed but were unused.** `saga_steps_total` had an `outcome` label with only `"success"` ever recorded — the `"error"` value was never incremented. `saga_dlq_messages_total` was declared in code but never wired up. These metrics were added during initial saga development with the intent to populate them later.

3. **Consumer error logs lacked context.** The saga consumer logged `"saga event handling failed"` with only `error` and `requeue` fields. Correlating errors to specific orders required parsing message bodies from raw log lines across multiple queries.

This work is a companion to the broader observability-gaps spec (which covers payment metrics, cart/product logging, Promtail fixes, and per-service alerts). That spec explicitly excluded DLQ alerting and didn't address saga error metrics.

## Decision

### Saga error metrics (orchestrator.go)

Added `SagaStepsTotal.WithLabelValues(step, "error").Inc()` at all 6 error/compensation paths: stock insufficient, payment creation failed, payment failed event, refund error, unknown saga step, and unknown event type. This populates the existing `outcome` label rather than creating a new metric — the counter was designed for this from the start.

### DLQ counter (consumer.go)

Added `SagaDLQTotal.Inc()` in the consumer's error-handling path when `requeue` is `false` (message goes to DLQ via the dead-letter exchange). This is the only code path that sends messages to the DLQ, so a single increment point is sufficient.

### Enriched consumer logging (consumer.go)

Changed `handleMessage` to return `(Event, error)` instead of `error`. It already parsed the event — the change just returns it so the caller can log context fields. Error logs now include `orderID`, `event` (type), and `routingKey`, enabling single-query correlation in Loki.

### Alerts (grafana-alerting.yml)

- **`saga-dlq-accumulating`**: `increase(saga_dlq_messages_total[10m]) > 0` for 2 minutes. Deliberately sensitive — any sustained DLQ activity means stuck orders. The 10-minute window smooths transient nacks.
- **`saga-step-error-rate`**: `>10%` error rate on saga steps for 5 minutes. Catches systematic failures (bad migration, broken gRPC, etc.) without alerting on occasional stock-out compensations.

### Dashboard (grafana-dashboards.yml)

New "Saga Error Analysis" row with 3 panels: step error rate by step (stacked with success), DLQ activity + replay rate, and saga duration percentiles (p50/p95/p99).

## Consequences

- **Positive:** Saga failures are now self-diagnosing. The debugging workflow is: check dashboard → query Loki with orderID → check DLQ admin API. This replaces the previous workflow of ~10 exploratory Loki queries.
- **Positive:** The DLQ alert would have been the earliest signal in the VARCHAR overflow incident — messages started accumulating in the DLQ before the circuit breaker fully opened.
- **Positive:** No new metrics were needed — all changes populate existing, previously-unused metric infrastructure.
- **Trade-off:** The DLQ alert is intentionally sensitive (any activity fires it). This could be noisy if a transient error causes a single DLQ message. Acceptable because DLQ messages in a saga represent stuck orders that need attention regardless.
- **Trade-off:** The `handleMessage` signature change from `error` to `(Event, error)` means even unmarshal failures return a zero-value Event. The empty fields in logs are clearly distinguishable from real data and still provide the routing key for diagnosis.
