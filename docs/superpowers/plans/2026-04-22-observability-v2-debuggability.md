# Observability V2: Debuggability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Go service failure diagnosable from Grafana and Loki without kubectl exec — gRPC client metrics, saga step timing, build version logging, and Stripe call logging.

**Architecture:** Shared packages in `go/pkg/` (grpcmetrics, buildinfo) consumed by all services. Saga timing in orchestrator. Stripe logging in payment-service. Dashboard panels in existing go-services dashboard.

**Tech Stack:** Go (slog, prometheus/promauto, grpc interceptors), Grafana dashboard JSON, Dockerfiles (-ldflags)

**Spec:** `docs/superpowers/specs/2026-04-22-observability-v2-debuggability-design.md`

---

### Task 1: Create gRPC Client Interceptor Package

**Files:**
- Create: `go/pkg/grpcmetrics/interceptor.go`
- Create: `go/pkg/grpcmetrics/interceptor_test.go`

- [ ] **Step 1: Create the interceptor with metrics and error logging**

Create `go/pkg/grpcmetrics/interceptor.go`:

```go
package grpcmetrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var (
	clientRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_client_requests_total",
		Help: "Total outbound gRPC requests.",
	}, []string{"target", "method", "status"})

	clientRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_client_request_duration_seconds",
		Help:    "Outbound gRPC request duration.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"target", "method"})
)

// UnaryClientInterceptor returns a gRPC unary client interceptor that
// records Prometheus metrics and logs errors for every outbound call.
func UnaryClientInterceptor(target string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		elapsed := time.Since(start)

		st, _ := status.FromError(err)
		code := st.Code().String()

		clientRequestsTotal.WithLabelValues(target, method, code).Inc()
		clientRequestDuration.WithLabelValues(target, method).Observe(elapsed.Seconds())

		if err != nil {
			slog.ErrorContext(ctx, "gRPC client call failed",
				"target", target,
				"method", method,
				"status", code,
				"duration", elapsed,
				"error", err,
			)
		}

		return err
	}
}
```

- [ ] **Step 2: Write the test**

Create `go/pkg/grpcmetrics/interceptor_test.go`:

```go
package grpcmetrics

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryClientInterceptor_RecordsMetrics(t *testing.T) {
	interceptor := UnaryClientInterceptor("test-service")

	// Successful call
	err := interceptor(
		context.Background(),
		"/test.Service/Method",
		nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestUnaryClientInterceptor_LogsErrors(t *testing.T) {
	interceptor := UnaryClientInterceptor("test-service")

	err := interceptor(
		context.Background(),
		"/test.Service/Method",
		nil, nil, nil,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
			return status.Error(codes.Unavailable, "connection refused")
		},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", st.Code())
	}
}
```

- [ ] **Step 3: Add go.mod dependencies and run tests**

Run: `cd go/pkg && go get google.golang.org/grpc && go mod tidy && go test ./grpcmetrics/ -v -race`
Expected: Both tests pass

- [ ] **Step 4: Commit**

```bash
git add go/pkg/grpcmetrics/
git commit -m "feat(pkg): add shared gRPC client interceptor with Prometheus metrics"
```

---

### Task 2: Wire Interceptor into Order-Service gRPC Clients

**Files:**
- Modify: `go/order-service/internal/productclient/client.go`
- Modify: `go/order-service/internal/cartclient/client.go`
- Modify: `go/order-service/internal/paymentclient/client.go`
- Modify: `go/order-service/cmd/server/main.go` (auth client)

- [ ] **Step 1: Add interceptor to productclient**

In `go/order-service/internal/productclient/client.go`, add the import and interceptor. Replace the `New` function:

```go
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("product-service")),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 2: Add interceptor to paymentclient**

In `go/order-service/internal/paymentclient/client.go`, replace the `New` function:

```go
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("payment-service")),
	)
	if err != nil {
		return nil, fmt.Errorf("payment grpc dial: %w", err)
	}
	return &GRPCClient{conn: conn, client: pb.NewPaymentServiceClient(conn)}, nil
}
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 3: Add interceptor to cartclient**

In `go/order-service/internal/cartclient/client.go`, add the interceptor to both `grpc.NewClient` calls in `New`:

```go
cartConn, err := grpc.NewClient(cartAddr,
	grpc.WithTransportCredentials(creds),
	grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("cart-service")),
)
```

And:

```go
productConn, err := grpc.NewClient(productAddr,
	grpc.WithTransportCredentials(creds),
	grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("product-service")),
)
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 4: Add interceptor to auth client in main.go**

In `go/order-service/cmd/server/main.go`, find the auth `grpc.NewClient` call (around line 157) and add the interceptor:

```go
authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
	grpc.WithTransportCredentials(grpcCreds),
	grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("auth-service")),
)
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 5: Build and test**

Run: `cd go/order-service && go mod tidy && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add go/order-service/
git commit -m "feat(order): wire gRPC client interceptor into all outbound clients"
```

---

### Task 3: Wire Interceptor into Cart-Service gRPC Clients

**Files:**
- Modify: `go/cart-service/internal/productclient/client.go`
- Modify: `go/cart-service/cmd/server/main.go` (auth client)

- [ ] **Step 1: Add interceptor to cart-service productclient**

In `go/cart-service/internal/productclient/client.go`, replace the `New` function:

```go
func New(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("product-service")),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 2: Add interceptor to cart-service auth client**

In `go/cart-service/cmd/server/main.go`, find the auth `grpc.NewClient` call (around line 113) and add:

```go
authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
	grpc.WithTransportCredentials(grpcClientCreds),
	grpc.WithUnaryInterceptor(grpcmetrics.UnaryClientInterceptor("auth-service")),
)
```

Add import: `"github.com/kabradshaw1/portfolio/go/pkg/grpcmetrics"`

- [ ] **Step 3: Build and test**

Run: `cd go/cart-service && go mod tidy && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add go/cart-service/
git commit -m "feat(cart): wire gRPC client interceptor into outbound clients"
```

---

### Task 4: Add Saga Step Duration Metric

**Files:**
- Modify: `go/order-service/internal/saga/metrics.go`
- Modify: `go/order-service/internal/saga/orchestrator.go`

- [ ] **Step 1: Add the histogram to metrics.go**

In `go/order-service/internal/saga/metrics.go`, add after `SagaDuration`:

```go
	SagaStepDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "saga_step_duration_seconds",
		Help:    "Duration of each saga step handler.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"step", "outcome"})
```

- [ ] **Step 2: Add timing to the Advance method**

In `go/order-service/internal/saga/orchestrator.go`, replace the `Advance` method (lines 54-81) with:

```go
func (o *Orchestrator) Advance(ctx context.Context, orderID uuid.UUID) error {
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order: %w", err)
	}

	slog.InfoContext(ctx, "advancing saga", "orderID", orderID, "currentStep", order.SagaStep)

	start := time.Now()
	var stepErr error

	switch order.SagaStep {
	case StepCreated:
		stepErr = o.handleCreated(ctx, order)
	case StepItemsReserved:
		stepErr = o.handleItemsReserved(ctx, order)
	case StepStockValidated:
		stepErr = o.handleStockValidated(ctx, order)
	case StepPaymentCreated:
		return nil // Waiting for webhook confirmation via outbox poller
	case StepPaymentConfirmed:
		stepErr = o.handlePaymentConfirmed(ctx, order)
	case StepCompensating:
		return nil // Compensation command already sent, waiting for reply
	case StepCompleted, StepCompensationComplete, StepFailed:
		return nil // Terminal states
	default:
		SagaStepsTotal.WithLabelValues(order.SagaStep, "error").Inc()
		return fmt.Errorf("unknown saga step: %s", order.SagaStep)
	}

	elapsed := time.Since(start)
	outcome := "success"
	if stepErr != nil {
		outcome = "error"
	}
	SagaStepDuration.WithLabelValues(order.SagaStep, outcome).Observe(elapsed.Seconds())

	return stepErr
}
```

Note: `time` is already imported in this file.

- [ ] **Step 3: Build and test**

Run: `cd go/order-service && go build ./... && go test ./internal/saga/ -v -race`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add go/order-service/internal/saga/
git commit -m "feat(order): add saga step duration histogram metric"
```

---

### Task 5: Create Build Info Package

**Files:**
- Create: `go/pkg/buildinfo/buildinfo.go`

- [ ] **Step 1: Create the package**

Create `go/pkg/buildinfo/buildinfo.go`:

```go
package buildinfo

import (
	"log/slog"
	"runtime"
)

// Set via -ldflags at build time.
var (
	Version = "dev"
	GitSHA  = "unknown"
)

// Log emits a structured log line with build metadata. Call once at startup.
func Log() {
	slog.Info("service started",
		"version", Version,
		"gitSHA", GitSHA,
		"goVersion", runtime.Version(),
	)
}
```

- [ ] **Step 2: Build**

Run: `cd go/pkg && go build ./buildinfo/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add go/pkg/buildinfo/
git commit -m "feat(pkg): add shared build info package for startup version logging"
```

---

### Task 6: Wire Build Info into All Go Services

**Files:**
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/payment-service/cmd/server/main.go`
- Modify: `go/cart-service/cmd/server/main.go`
- Modify: `go/product-service/cmd/server/main.go`
- Modify: `go/auth-service/cmd/server/main.go`
- Modify: `go/ai-service/cmd/server/main.go`
- Modify: `go/analytics-service/cmd/server/main.go`

- [ ] **Step 1: Add buildinfo.Log() to each service's main()**

In each service's `cmd/server/main.go`, add the import and call `buildinfo.Log()` immediately after `slog.SetDefault(...)`:

```go
import "github.com/kabradshaw1/portfolio/go/pkg/buildinfo"

// ... inside main(), after slog.SetDefault:
buildinfo.Log()
```

Do this for all 7 services listed above.

- [ ] **Step 2: Build all services**

Run: `cd go/order-service && go mod tidy && go build ./... && cd ../payment-service && go mod tidy && go build ./... && cd ../cart-service && go mod tidy && go build ./... && cd ../product-service && go mod tidy && go build ./... && cd ../auth-service && go mod tidy && go build ./... && cd ../ai-service && go mod tidy && go build ./... && cd ../analytics-service && go mod tidy && go build ./...`
Expected: All build successfully

- [ ] **Step 3: Commit**

```bash
git add go/*/cmd/server/main.go go/*/go.mod go/*/go.sum
git commit -m "feat: add startup build info logging to all Go services"
```

---

### Task 7: Add -ldflags to All Dockerfiles

**Files:**
- Modify: `go/order-service/Dockerfile`
- Modify: `go/payment-service/Dockerfile`
- Modify: `go/cart-service/Dockerfile`
- Modify: `go/product-service/Dockerfile`
- Modify: `go/auth-service/Dockerfile`
- Modify: `go/ai-service/Dockerfile`
- Modify: `go/analytics-service/Dockerfile`

- [ ] **Step 1: Update each Dockerfile's go build line**

In each Dockerfile, replace the `RUN CGO_ENABLED=0 GOOS=linux go build -o /<service> ./cmd/server` line with:

```dockerfile
ARG BUILD_VERSION=dev
ARG BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.Version=${BUILD_VERSION} -X github.com/kabradshaw1/portfolio/go/pkg/buildinfo.GitSHA=${BUILD_COMMIT}" \
    -o /<service-name> ./cmd/server
```

Replace `<service-name>` with the correct binary name for each service (order-service, payment-service, etc.).

The `ARG` lines must come BEFORE the `RUN` line, within the builder stage.

- [ ] **Step 2: Update CI to pass build args**

In `.github/workflows/ci.yml`, find the Docker build step in the `build-images` job. Add build-args to the `docker/build-push-action` step:

```yaml
      build-args: |
        BUILD_VERSION=${{ github.sha }}
        BUILD_COMMIT=${{ github.sha }}
```

This goes in the existing `with:` block of the build-push-action step.

- [ ] **Step 3: Commit**

```bash
git add go/*/Dockerfile .github/workflows/ci.yml
git commit -m "ci: inject git SHA into Go binaries via Docker build args"
```

---

### Task 8: Add Stripe API Call Logging

**Files:**
- Modify: `go/payment-service/internal/service/stripe.go`

- [ ] **Step 1: Add logging to CreatePayment**

In `go/payment-service/internal/service/stripe.go`, add `"log/slog"` and `"time"` to the import block if not present.

Replace the Stripe call section in `CreatePayment` (around lines 84-89) with:

```go
	slog.InfoContext(ctx, "calling Stripe API", "operation", "create_checkout", "orderID", orderID, "amountCents", amountCents)
	stripeStart := time.Now()
	result, err := s.stripe.CreateCheckoutSession(ctx, params)
	stripeElapsed := time.Since(stripeStart)
	if err != nil {
		slog.ErrorContext(ctx, "Stripe API failed", "operation", "create_checkout", "orderID", orderID, "duration", stripeElapsed, "error", err)
		_ = s.repo.UpdateStatus(ctx, orderID, model.PaymentStatusFailed)
		return nil, fmt.Errorf("create stripe checkout session: %w", err)
	}
	slog.InfoContext(ctx, "Stripe API responded", "operation", "create_checkout", "orderID", orderID, "duration", stripeElapsed)
```

- [ ] **Step 2: Add logging to RefundPayment**

Replace the Stripe refund call section in `RefundPayment` (around lines 121-124) with:

```go
	slog.InfoContext(ctx, "calling Stripe API", "operation", "refund", "orderID", orderID, "intentID", payment.StripePaymentIntentID)
	refundStart := time.Now()
	refundID, err := s.stripe.Refund(ctx, payment.StripePaymentIntentID, reason)
	refundElapsed := time.Since(refundStart)
	if err != nil {
		slog.ErrorContext(ctx, "Stripe API failed", "operation", "refund", "orderID", orderID, "duration", refundElapsed, "error", err)
		return nil, "", fmt.Errorf("stripe refund: %w", err)
	}
	slog.InfoContext(ctx, "Stripe API responded", "operation", "refund", "orderID", orderID, "refundID", refundID, "duration", refundElapsed)
```

- [ ] **Step 3: Build and test**

Run: `cd go/payment-service && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add go/payment-service/internal/service/stripe.go
git commit -m "feat(payment): add before/after logging for Stripe API calls"
```

---

### Task 9: Run Go Preflight

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: All lint and tests pass

- [ ] **Step 2: Fix any lint issues and commit**

```bash
git commit -am "style: fix lint issues from observability v2"
```

---

### Task 10: Add Dashboard Panels

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add gRPC Client Health row**

In the `go-services.json` section of `k8s/monitoring/configmaps/grafana-dashboards.yml`, after the last panel from the previous observability work (Outbox Publish, ID 34, y=50), add a new row and 3 panels.

Row header (ID 35):
```json
{"collapsed":false,"gridPos":{"h":1,"w":24,"x":0,"y":56},"id":35,"title":"gRPC Client Health","type":"row"}
```

gRPC request rate by target (ID 36):
```json
{"datasource":{"type":"prometheus","uid":"PBFA97CFB590B2093"},"fieldConfig":{"defaults":{"unit":"reqps","color":{"mode":"palette-classic"}},"overrides":[]},"gridPos":{"h":6,"w":8,"x":0,"y":57},"id":36,"title":"gRPC Request Rate by Target","type":"timeseries","targets":[{"expr":"sum by (target) (rate(grpc_client_requests_total[5m]))","legendFormat":"{{target}}","refId":"A"}]}
```

gRPC error rate by target (ID 37):
```json
{"datasource":{"type":"prometheus","uid":"PBFA97CFB590B2093"},"fieldConfig":{"defaults":{"unit":"percent","color":{"mode":"palette-classic"},"thresholds":{"steps":[{"color":"green","value":null},{"color":"red","value":5}]}},"overrides":[]},"gridPos":{"h":6,"w":8,"x":8,"y":57},"id":37,"title":"gRPC Error Rate by Target","type":"timeseries","targets":[{"expr":"sum by (target) (rate(grpc_client_requests_total{status!=\"OK\"}[5m])) / sum by (target) (rate(grpc_client_requests_total[5m])) * 100","legendFormat":"{{target}}","refId":"A"}]}
```

gRPC p95 latency by target (ID 38):
```json
{"datasource":{"type":"prometheus","uid":"PBFA97CFB590B2093"},"fieldConfig":{"defaults":{"unit":"s","color":{"mode":"palette-classic"},"thresholds":{"steps":[{"color":"green","value":null},{"color":"yellow","value":1},{"color":"red","value":5}]}},"overrides":[]},"gridPos":{"h":6,"w":8,"x":16,"y":57},"id":38,"title":"gRPC p95 Latency by Target","type":"timeseries","targets":[{"expr":"histogram_quantile(0.95, sum by (target, le) (rate(grpc_client_request_duration_seconds_bucket[5m])))","legendFormat":"{{target}}","refId":"A"}]}
```

- [ ] **Step 2: Add saga step duration panel to existing Saga row**

Add a panel to the "Saga & Payment Health" row. Place it after the existing 4 panels (ID 31-34) by adjusting gridPos. Use ID 39:

```json
{"datasource":{"type":"prometheus","uid":"PBFA97CFB590B2093"},"fieldConfig":{"defaults":{"unit":"s","color":{"mode":"palette-classic"}},"overrides":[]},"gridPos":{"h":6,"w":12,"x":0,"y":63},"id":39,"title":"Saga Step Duration (p95)","type":"timeseries","targets":[{"expr":"histogram_quantile(0.95, sum by (step, le) (rate(saga_step_duration_seconds_bucket[5m])))","legendFormat":"{{step}}","refId":"A"}]}
```

- [ ] **Step 3: Add cert-manager certificate expiry panel**

Add a stat panel (ID 40) next to the saga duration:

```json
{"datasource":{"type":"prometheus","uid":"PBFA97CFB590B2093"},"fieldConfig":{"defaults":{"unit":"d","color":{"mode":"thresholds"},"thresholds":{"steps":[{"color":"red","value":null},{"color":"yellow","value":1},{"color":"green","value":7}]}},"overrides":[]},"gridPos":{"h":6,"w":12,"x":12,"y":63},"id":40,"title":"Certificate Expiry","type":"stat","targets":[{"expr":"(certmanager_certificate_expiration_timestamp_seconds - time()) / 86400","legendFormat":"{{name}}","refId":"A"}]}
```

- [ ] **Step 4: Validate YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/grafana-dashboards.yml')); print('valid')"`
Expected: `valid`

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add gRPC client health, saga duration, and cert expiry dashboard panels"
```

---

### Task 11: Verify cert-manager Prometheus Scraping

- [ ] **Step 1: Check if Prometheus scrapes cert-manager**

Run: `ssh debian 'kubectl get pods -n cert-manager -o yaml | grep -A2 prometheus.io'`

If no `prometheus.io/scrape: "true"` annotation exists, add it to the cert-manager deployment. If it does exist, skip this task.

- [ ] **Step 2: If needed, patch cert-manager deployment**

Run:
```bash
ssh debian 'kubectl patch deployment cert-manager -n cert-manager --type merge -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"prometheus.io/scrape\":\"true\",\"prometheus.io/port\":\"9402\"}}}}}"'
```

- [ ] **Step 3: Commit (if Prometheus config changed)**

Only if a ConfigMap change was needed:
```bash
git add k8s/monitoring/
git commit -m "fix(monitoring): enable Prometheus scraping for cert-manager metrics"
```

---

### Task 12: Final Validation and Push

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: All lint and tests pass

- [ ] **Step 2: Validate all K8s YAML files**

Run:
```bash
python3 -c "
import yaml, glob
for f in sorted(glob.glob('k8s/monitoring/configmaps/*.yml')):
    yaml.safe_load(open(f))
    print(f'OK: {f}')
"
```
Expected: All files OK

- [ ] **Step 3: Push to qa**

Run: `git push origin qa`
