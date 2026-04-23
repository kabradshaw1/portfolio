# Fix Observability Debugging Gaps

**Date:** 2026-04-23
**Status:** Draft
**Scope:** Error logging, alerting, OTEL config, dashboard query

## Problem

During the Stripe webhook payment lookup bug, four observability gaps caused significant debugging friction:

1. **Silent 5xx errors**: `apperror.ErrorHandler()` middleware only logs unknown errors. AppError instances with status >= 500 are converted to JSON responses with no log entry. Server errors vanish from Loki.
2. **No stuck order alert**: Orders sat at `PAYMENT_CREATED` for hours with no alert. Existing saga alerts only fire on DLQ accumulation and step error rates.
3. **OTEL endpoint typo**: Payment-service points to `jaeger-collector` while all other services use `jaeger`. Causes "traces export timeout" log noise.
4. **No webhook event type breakdown**: Dashboard shows aggregate webhook success/error but no per-event-type breakdown.

## Solution

### 1. Log 5xx AppErrors in middleware

**File:** `go/pkg/apperror/middleware.go`

Add `slog.ErrorContext` for AppErrors with `HTTPStatus >= 500`, before sending the JSON response. Include error code, message, and request ID. No change for 4xx errors.

### 2. Fix payment-service OTEL endpoint

**File:** `go/k8s/configmaps/payment-service-config.yml`

Change `jaeger-collector.monitoring.svc.cluster.local:4317` to `jaeger.monitoring.svc.cluster.local:4317`.

### 3. Add stuck order alert

**File:** `k8s/monitoring/configmaps/grafana-alerting.yml`

New alert rule `saga-order-stalled` using existing `saga_steps_total` counters. Fires when orders reach `PAYMENT_CREATED` but none reach a terminal state (`COMPLETED` or `COMPENSATION_COMPLETE`) within 30 minutes. Routes to Telegram.

### 4. Webhook event type breakdown on dashboard

**File:** `k8s/monitoring/configmaps/grafana-dashboards.yml`

Update the "Payment Webhooks" panel PromQL to group `by (event_type, outcome)` instead of just `by (outcome)`.

## Files Changed

| File | Change |
|------|--------|
| `go/pkg/apperror/middleware.go` | Add slog.Error for AppErrors with HTTPStatus >= 500 |
| `go/k8s/configmaps/payment-service-config.yml` | Fix OTEL endpoint |
| `k8s/monitoring/configmaps/grafana-alerting.yml` | Add saga-order-stalled alert |
| `k8s/monitoring/configmaps/grafana-dashboards.yml` | Add event_type breakdown to webhook panel |

## Verification

1. `make preflight-go` — middleware change compiles and tests pass
2. Deploy to QA, trigger a 5xx error (e.g., stop postgres, make a request) — verify error appears in Loki
3. Payment-service logs: verify "traces export timeout" noise stops
4. Grafana: verify new alert rule appears in alert list
5. Grafana: verify webhook panel shows per-event-type breakdown
