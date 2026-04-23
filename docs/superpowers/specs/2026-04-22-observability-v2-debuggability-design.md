# Observability V2: Debuggability Gaps

## Context

During a multi-hour debugging session for the payment-service checkout failure, every critical diagnosis required manual `kubectl exec`, `kubectl logs | grep`, and `openssl` commands. The observability stack (Prometheus, Loki, Grafana) couldn't answer basic questions:

- **"Is my fix deployed?"** — No build version in logs. Had to check image digests and grep for feature-specific startup lines.
- **"Why is the saga stuck?"** — gRPC call to payment-service hung silently forever. No timeout, no error, no metric. Loki showed the saga entering a step and then nothing.
- **"Is the gRPC connection healthy?"** — No client-side gRPC metrics. Had to `wget` from inside pods to test TLS handshakes.
- **"What's Stripe doing?"** — Payment-service calls Stripe with zero logging around the call. No "calling Stripe" or "Stripe responded in Xms".
- **"Are the TLS certs valid?"** — cert-manager Certificate status not visible in any dashboard.

**Goal:** Make every failure diagnosable from Grafana and Loki without SSH or kubectl exec. Turn silent failures into loud, queryable signals.

**Scope:** Go services only. Builds on the first observability spec (`2026-04-22-observability-gaps-design.md`).

## Design

### 1. gRPC Client Interceptor Metrics

**Files:**
- Create: `go/pkg/grpcmetrics/interceptor.go`
- Modify: `go/order-service/cmd/server/main.go` (add interceptor to gRPC client dials)
- Modify: Any other service that creates outbound gRPC clients

**New shared package** `go/pkg/grpcmetrics/` provides a unary client interceptor that wraps every outbound gRPC call with:

**Prometheus metrics:**
- `grpc_client_requests_total` — counter, labels: `target`, `method`, `status` (OK, Unavailable, DeadlineExceeded, Internal, etc.)
- `grpc_client_request_duration_seconds` — histogram, labels: `target`, `method`

**Structured logging:** On every non-OK result, log with `slog.ErrorContext` including target, method, gRPC status code, and duration.

**Usage pattern:**
```go
conn, err := grpc.NewClient(addr,
    grpc.WithTransportCredentials(creds),
    grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor()),
)
```

Services that create outbound gRPC clients: order-service (payment, cart, product, auth), cart-service (product, auth), ecommerce-service (product).

### 2. Saga Step Duration Metric

**Files:**
- Modify: `go/order-service/internal/saga/metrics.go` (add histogram)
- Modify: `go/order-service/internal/saga/orchestrator.go` (record timing in `Advance`)

**New metric:**
- `saga_step_duration_seconds` — histogram, labels: `step`, `outcome` (success/error)

**Implementation:** Time the step handler call inside `Advance()`, wrapping the switch-case. Single measurement point — not per-handler. Records duration and outcome after each step handler returns.

### 3. Startup Version Logging

**Files:**
- Create: `go/pkg/buildinfo/buildinfo.go`
- Modify: Each Go service's `cmd/server/main.go` (call `buildinfo.Log()` at startup)
- Modify: Each Go service's `Dockerfile` (add `-ldflags` to `go build`)

**Shared package** `go/pkg/buildinfo/` with:
- Package-level `var` for `Version` and `GitSHA` (set via ldflags)
- `Log()` function that calls `slog.Info("service started", "version", Version, "gitSHA", GitSHA, "goVersion", runtime.Version())`

**Build change:** Add to each Dockerfile's `go build` line:
```
-ldflags "-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=$(git describe --tags --always) -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=$(git rev-parse --short HEAD)"
```

**Loki query:** `{app="go-payment-service"} |= "service started" | json | gitSHA="abc1234"`

### 4. Stripe API Call Logging

**Files:**
- Modify: `go/payment-service/internal/service/stripe.go`

Add before/after logging around the two Stripe API calls in `CreatePayment` and `RefundPayment`:

- Before: `slog.InfoContext(ctx, "calling Stripe API", "operation", "<op>", "orderID", orderID)`
- After success: `slog.InfoContext(ctx, "Stripe API responded", "operation", "<op>", "orderID", orderID, "duration", elapsed)`
- After error: `slog.ErrorContext(ctx, "Stripe API failed", "operation", "<op>", "orderID", orderID, "duration", elapsed, "error", err)`

No new Prometheus metrics — the gRPC client interceptor captures overall call duration from the order-service side. This logging captures Stripe's specific contribution to that duration.

### 5. Dashboard Panels

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml` (go-services.json section)

**New row: gRPC Client Health** (3 panels, 8-width each):
- Request rate by target service (`sum by (target) (rate(grpc_client_requests_total[5m]))`)
- Error rate by target service (`sum by (target) (rate(grpc_client_requests_total{status!="OK"}[5m])) / sum by (target) (rate(grpc_client_requests_total[5m])) * 100`)
- p95 latency by target service (`histogram_quantile(0.95, sum by (target, le) (rate(grpc_client_request_duration_seconds_bucket[5m])))`)

**Addition to Saga & Payment Health row** (1 panel):
- Saga step duration timeseries (`histogram_quantile(0.95, sum by (step, le) (rate(saga_step_duration_seconds_bucket[5m])))`)

**New panel: Certificate Expiry** (1 stat panel):
- Query: `(certmanager_certificate_expiration_timestamp_seconds - time()) / 86400`
- Unit: days
- Thresholds: green (default), yellow < 7, red < 1
- Note: cert-manager exports this metric automatically; Prometheus just needs to scrape the cert-manager pod (check that `prometheus.io/scrape` annotation exists)

## Out of Scope

- gRPC streaming interceptor (no streaming gRPC calls currently)
- Alerting on the new metrics (can be added once we validate the dashboard panels work)
- Server-side gRPC metrics (already handled by `otelgrpc.NewServerHandler()`)
- Stripe webhook latency metrics (already covered in observability v1 spec)

## Verification

1. **gRPC interceptor:** Trigger checkout on QA. Grafana gRPC Client Health panel should show requests to `go-payment-service` with status and latency. Loki should show errors with target/method/status on failure.
2. **Saga duration:** Trigger checkout. Saga step duration panel should show time per step. STOCK_VALIDATED should show ~30s if the payment call is slow (matching the timeout).
3. **Build version:** After deploy, query Loki: `{app="go-payment-service"} |= "service started" | json` — should show gitSHA and version.
4. **Stripe logging:** Trigger checkout. Loki query `{app="go-payment-service"} |= "Stripe API"` should show the call with duration.
5. **Cert expiry:** Grafana panel should show days until expiry for all gRPC certificates.
