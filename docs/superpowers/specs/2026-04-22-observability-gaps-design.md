# Observability Gaps: Go & Python Service Instrumentation

## Context

The ecommerce monolith was decomposed into order-service, cart-service, payment-service, and product-service. The monitoring stack (Prometheus, Loki, Jaeger, Grafana) was deployed alongside. However, instrumentation hasn't caught up with the decomposition:

- Cart and product service layers log nothing — operations succeed or fail silently
- Payment-service has zero custom Prometheus metrics
- Promtail can't extract Python trace IDs due to a field name mismatch
- Dashboards and alerts still reference the monolithic service, missing the decomposed services entirely

This was exposed during a checkout debugging session where the payment-service TLS handshake failure was traceable via Loki, but several adjacent gaps made the investigation harder than necessary.

**Goal:** Close the instrumentation gaps so that any failure in the Go ecommerce saga or Python RAG pipeline can be diagnosed from Grafana/Loki/Jaeger without SSH.

**Scope:** Go services (primary) and Python Promtail integration. Java services are excluded (being deprecated to non-featured portfolio status).

## Design

### 1. Promtail Field Name Fix

**File:** `k8s/monitoring/configmaps/promtail-config.yml`

Add a second JSON extraction stage for Python's `trace_id` field and a template stage that merges it into the `traceID` label used by Loki's Jaeger derived field.

```yaml
pipeline_stages:
  - cri: {}
  - json:
      expressions:
        level: level
        msg: msg
        traceID: traceID        # Go services (camelCase)
  - json:
      expressions:
        trace_id: trace_id      # Python services (snake_case)
  - template:
      source: traceID
      template: '{{ if .trace_id }}{{ .trace_id }}{{ else }}{{ .traceID }}{{ end }}'
  - labels:
      level:
  # ... rest unchanged
```

No service code changes. Promtail DaemonSet rolls out automatically on ConfigMap update.

### 2. Cart-Service Logging

**File:** `go/cart-service/internal/service/cart.go`

Add `slog.InfoContext` / `slog.ErrorContext` at decision points:

| Method | What to log | Level |
|--------|------------|-------|
| `AddItem` | userID, productID, quantity on success | INFO |
| `AddItem` | userID, productID, error on validation failure | WARN |
| `RemoveItem` | userID, itemID | INFO |
| `UpdateQuantity` | userID, itemID, new quantity | INFO |
| `GetCart` | No logging (high-frequency read, covered by request middleware) | — |

All log calls must accept `context.Context` to carry traceID from the request span.

### 3. Product-Service Logging

**File:** `go/product-service/internal/service/product.go`

| Method | What to log | Level |
|--------|------------|-------|
| `DecrementStock` | productID, quantity, remaining stock on success | INFO |
| `DecrementStock` | productID, requested vs available on `ErrInsufficientStock` | WARN |
| Cache miss | productID when falling through to DB | INFO |
| Standard CRUD reads | No logging (covered by middleware) | — |

### 4. Payment-Service Metrics

**Files:** New `go/payment-service/internal/metrics/metrics.go` (or add to existing middleware)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `payment_webhook_events_total` | Counter | `event_type`, `outcome` (processed/duplicate/error) | Webhook throughput and error rate |
| `payment_created_total` | Counter | `status` (succeeded/failed) | Payment outcome tracking |
| `payment_outbox_publish_total` | Counter | `outcome` (success/error) | Outbox reliability |
| `payment_outbox_lag_seconds` | Histogram | — | Time from outbox insert to publish |

### 5. Payment-Service Logging & Trace Fix

**Logging additions:**
- `go/payment-service/internal/handler/webhook.go`: Log eventType, eventID, intentID after successful signature verification (currently only logs on failure)
- `go/payment-service/internal/service/webhook.go`: Include orderID in error logs when DB status update fails

**Trace injection:**
- `go/payment-service/internal/outbox/poller.go`: Add `tracing.InjectAMQP()` when publishing to RabbitMQ. This is the only saga producer that doesn't propagate trace context, breaking the trace chain for payment events.

### 6. Dashboard Updates

**File:** `k8s/monitoring/configmaps/grafana-dashboards.yml` (go-services.json section)

**New row: Decomposed Services RED**
- Per-service request rate panel (order, cart, payment, product — individual, not aggregated)
- Per-service error rate panel
- Per-service p95 latency panel
- Uses existing `http_requests_total` and `http_request_duration_seconds` metrics

**New row: Saga & Payment Health**
- Saga completion rate (CONFIRMED vs FAILED/COMPENSATING from `ecommerce_orders_placed_total`)
- Circuit breaker state panel (`circuit_breaker_state` gauge, per-breaker)
- Payment webhook throughput (`payment_webhook_events_total`)
- Outbox publish success/error rate (`payment_outbox_publish_total`)

No changes to the other 4 dashboards (system-overview, kubernetes, ai-pipeline, observability-overview).

### 7. Alert Rules

**File:** `k8s/monitoring/configmaps/grafana-alerting.yml`

**New Application SLO alerts** (same thresholds as existing order-service alerts):

| Alert | Condition | Duration |
|-------|-----------|----------|
| `go-cart-error-rate` | > 2% 5xx | 5 min |
| `go-payment-error-rate` | > 2% 5xx | 5 min |
| `go-product-error-rate` | > 2% 5xx | 5 min |
| `go-auth-error-rate` | > 2% 5xx | 5 min |
| `go-cart-latency-high` | p95 > 2s | 5 min |
| `go-payment-latency-high` | p95 > 2s | 5 min |
| `go-product-latency-high` | p95 > 2s | 5 min |

**New operational alert:**

| Alert | Condition | Duration |
|-------|-----------|----------|
| `circuit-breaker-open` | `circuit_breaker_state` == 2 (open) | 2 min |

All alerts route to the existing Telegram contact point. Must reference Prometheus datasource UID `PBFA97CFB590B2093`.

## Out of Scope

- Java service tracing (Java stack being deprecated)
- PostgreSQL query tracing (manual spans) — useful but significant effort for incremental value
- RabbitMQ management API metrics / DLQ depth alerts — requires new Prometheus scrape target
- Saga DLQ or Kafka per-aggregator alerts — depends on metrics not currently scraped
- Order-service saga logs adding userID — minor gap, orderID is sufficient for correlation

## Verification

1. **Promtail:** Deploy updated ConfigMap, query Loki for Python service logs — verify `traceID` label is populated and Jaeger links work
2. **Cart/product logging:** Trigger cart add, stock decrement via QA frontend — verify log lines appear in Loki with correct fields (userID, productID, traceID)
3. **Payment metrics:** Hit `/metrics` endpoint on payment-service — verify new counters/histograms are registered
4. **Payment trace:** Trigger checkout, find payment outbox message in Jaeger — verify trace chain is unbroken through payment saga events
5. **Dashboards:** Open Grafana go-services dashboard — verify new panels render with data for each decomposed service
6. **Alerts:** Verify alert rules appear in Grafana Alerting UI and reference correct Prometheus queries
7. **End-to-end:** Complete a checkout flow on QA — verify the entire saga is traceable from order creation through payment confirmation in Jaeger, with correlated logs in Loki
