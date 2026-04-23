# Observability Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close instrumentation gaps across Go and Python services so failures are diagnosable from Grafana/Loki/Jaeger without SSH.

**Architecture:** Bottom-up — fix Promtail field mapping, add service-layer logging to cart/product, add payment metrics and trace injection, then update dashboards and alerts to consume the new data.

**Tech Stack:** Go (slog, prometheus/promauto, OTel tracing), Promtail pipeline stages, Grafana provisioned dashboards/alerts (YAML ConfigMaps)

**Spec:** `docs/superpowers/specs/2026-04-22-observability-gaps-design.md`

---

### Task 1: Fix Promtail trace_id Field Mismatch

**Files:**
- Modify: `k8s/monitoring/configmaps/promtail-config.yml:27-49`

- [ ] **Step 1: Update the pipeline_stages to extract Python's trace_id**

In `k8s/monitoring/configmaps/promtail-config.yml`, replace the `pipeline_stages` block (lines 27-49) with:

```yaml
        pipeline_stages:
          - cri: {}
          - json:
              expressions:
                level: level
                msg: msg
                traceID: traceID
          - json:
              expressions:
                trace_id: trace_id
          - template:
              source: traceID
              template: '{{ if .trace_id }}{{ .trace_id }}{{ else }}{{ .traceID }}{{ end }}'
          - labels:
              level:
          - output:
              source: msg
          - regex:
              expression: '/var/log/pods/(?P<namespace>[^_]+)_(?P<pod>[^_]+)_[^/]+/(?P<container>[^/]+)/.*'
              source: filename
          - labels:
              namespace:
              pod:
              container:
          - template:
              source: app
              template: '{{ .container }}'
          - labels:
              app:
```

- [ ] **Step 2: Validate YAML is well-formed**

Run: `python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/promtail-config.yml')); print('valid')"`
Expected: `valid`

- [ ] **Step 3: Commit**

```bash
git add k8s/monitoring/configmaps/promtail-config.yml
git commit -m "fix(monitoring): add Python trace_id extraction to Promtail pipeline"
```

---

### Task 2: Add Service-Layer Logging to Cart-Service

**Files:**
- Modify: `go/cart-service/internal/service/cart.go`

- [ ] **Step 1: Add slog import and logging to AddItem**

In `go/cart-service/internal/service/cart.go`, add `"log/slog"` to the import block, then replace the `AddItem` method (lines 52-74):

```go
func (s *CartService) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	if err := s.productClient.ValidateProduct(ctx, productID); err != nil {
		slog.WarnContext(ctx, "product validation failed", "userID", userID, "productID", productID, "error", err)
		return nil, err
	}

	item, err := s.repo.AddItem(ctx, userID, productID, quantity)
	if err != nil {
		slog.ErrorContext(ctx, "failed to add item to cart", "userID", userID, "productID", productID, "quantity", quantity, "error", err)
		return nil, err
	}

	metrics.CartItemsAdded.Inc()
	metrics.ProductValidation.WithLabelValues("success").Inc()
	slog.InfoContext(ctx, "item added to cart", "userID", userID, "productID", productID, "quantity", quantity)

	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.cart", userID.String(), kafka.Event{
		Type: "cart.item_added",
		Data: map[string]any{
			"userID":    userID.String(),
			"productID": productID.String(),
			"quantity":  quantity,
		},
	})
	return item, nil
}
```

- [ ] **Step 2: Add logging to UpdateQuantity**

Replace the `UpdateQuantity` method (lines 76-78):

```go
func (s *CartService) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	if err := s.repo.UpdateQuantity(ctx, itemID, userID, quantity); err != nil {
		slog.ErrorContext(ctx, "failed to update cart item quantity", "userID", userID, "itemID", itemID, "quantity", quantity, "error", err)
		return err
	}
	slog.InfoContext(ctx, "cart item quantity updated", "userID", userID, "itemID", itemID, "quantity", quantity)
	return nil
}
```

- [ ] **Step 3: Add logging to RemoveItem**

Replace the `RemoveItem` method (lines 80-95):

```go
func (s *CartService) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
	if err := s.repo.RemoveItem(ctx, itemID, userID); err != nil {
		slog.ErrorContext(ctx, "failed to remove cart item", "userID", userID, "itemID", itemID, "error", err)
		return err
	}

	metrics.CartItemsRemoved.Inc()
	slog.InfoContext(ctx, "cart item removed", "userID", userID, "itemID", itemID)

	kafka.SafePublish(ctx, s.kafkaPublisher, "ecommerce.cart", userID.String(), kafka.Event{
		Type: "cart.item_removed",
		Data: map[string]any{
			"userID": userID.String(),
			"itemID": itemID.String(),
		},
	})
	return nil
}
```

- [ ] **Step 4: Build and run tests**

Run: `cd go/cart-service && go build ./... && go test ./internal/service/ -v -race`
Expected: All tests pass, no compilation errors

- [ ] **Step 5: Commit**

```bash
git add go/cart-service/internal/service/cart.go
git commit -m "feat(cart): add service-layer logging for cart operations"
```

---

### Task 3: Add Service-Layer Logging to Product-Service

**Files:**
- Modify: `go/product-service/internal/service/product.go`

- [ ] **Step 1: Add slog import and logging to List (cache miss path)**

In `go/product-service/internal/service/product.go`, add `"log/slog"` to the import block, then replace the `List` method (lines 64-86):

```go
func (s *ProductService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	if params.Cursor != "" {
		return s.repo.List(ctx, params)
	}

	cacheKey := fmt.Sprintf("product:list:%s:%s:%d:%d", params.Category, params.Sort, params.Page, params.Limit)

	if resp, ok := getFromCache[model.ProductListResponse](ctx, s.redis, cacheKey); ok {
		return resp.Products, resp.Total, nil
	}

	slog.InfoContext(ctx, "product list cache miss", "cacheKey", cacheKey)

	products, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	setInCache(ctx, s.redis, cacheKey, model.ProductListResponse{
		Products: products, Total: total, Page: params.Page, Limit: params.Limit,
	}, 5*time.Minute)

	return products, total, nil
}
```

- [ ] **Step 2: Add logging to GetByID (cache miss path)**

Replace the `GetByID` method (lines 88-102):

```go
func (s *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	cacheKey := fmt.Sprintf("product:%s", id.String())

	if p, ok := getFromCache[model.Product](ctx, s.redis, cacheKey); ok {
		return &p, nil
	}

	slog.InfoContext(ctx, "product cache miss", "productID", id, "cacheKey", cacheKey)

	product, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	setInCache(ctx, s.redis, cacheKey, product, 5*time.Minute)
	return product, nil
}
```

- [ ] **Step 3: Add logging to InvalidateCache**

Replace the `InvalidateCache` method (lines 120-142):

```go
func (s *ProductService) InvalidateCache(ctx context.Context) error {
	if s.redis == nil {
		return nil
	}

	var cursor uint64
	var deleted int
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "product:*", 100).Result()
		if err != nil {
			return fmt.Errorf("scan product keys: %w", err)
		}
		if len(keys) > 0 {
			s.redis.Del(ctx, keys...)
			deleted += len(keys)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	s.redis.Del(ctx, "product:categories")
	slog.InfoContext(ctx, "product cache invalidated", "keysDeleted", deleted+1)
	return nil
}
```

- [ ] **Step 4: Build and run tests**

Run: `cd go/product-service && go build ./... && go test ./internal/service/ -v -race`
Expected: All tests pass, no compilation errors

- [ ] **Step 5: Commit**

```bash
git add go/product-service/internal/service/product.go
git commit -m "feat(product): add service-layer logging for cache misses and invalidation"
```

---

### Task 4: Create Payment-Service Metrics Package

**Files:**
- Create: `go/payment-service/internal/metrics/metrics.go`

- [ ] **Step 1: Create the metrics package**

Create `go/payment-service/internal/metrics/metrics.go`:

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WebhookEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_webhook_events_total",
		Help: "Stripe webhook events received.",
	}, []string{"event_type", "outcome"}) // outcome: processed, duplicate, error

	PaymentsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_created_total",
		Help: "Payments created via gRPC.",
	}, []string{"status"}) // status: succeeded, failed

	OutboxPublish = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_outbox_publish_total",
		Help: "Outbox message publish attempts.",
	}, []string{"outcome"}) // outcome: success, error

	OutboxLag = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "payment_outbox_lag_seconds",
		Help:    "Time from outbox insert to publish.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
	})
)
```

- [ ] **Step 2: Build**

Run: `cd go/payment-service && go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add go/payment-service/internal/metrics/metrics.go
git commit -m "feat(payment): add Prometheus business metrics package"
```

---

### Task 5: Add Webhook Logging, Metrics, and Error Context to Payment-Service

**Files:**
- Modify: `go/payment-service/internal/handler/webhook.go`
- Modify: `go/payment-service/internal/service/webhook.go`

- [ ] **Step 1: Add webhook success logging and metrics to handler**

In `go/payment-service/internal/handler/webhook.go`, add the metrics import and logging after signature verification. Replace lines 3-11 (imports) with:

```go
import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

Then replace the `HandleWebhook` method (lines 38-75) with:

```go
func (h *WebhookHandler) HandleWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_READ_FAILED", "failed to read webhook body"))
		return
	}

	sigHeader := c.GetHeader("Stripe-Signature")
	eventType, eventID, intentID, err := h.verifier.VerifyAndParse(payload, sigHeader)
	if err != nil {
		_ = c.Error(apperror.BadRequest("WEBHOOK_INVALID_SIGNATURE", "webhook signature verification failed"))
		return
	}

	ctx := c.Request.Context()
	slog.InfoContext(ctx, "webhook event received", "eventType", eventType, "eventID", eventID, "intentID", intentID)

	switch eventType {
	case "payment_intent.succeeded":
		if err := h.svc.HandlePaymentSucceeded(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "payment_intent.payment_failed":
		if err := h.svc.HandlePaymentFailed(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	case "charge.refunded":
		if err := h.svc.HandleRefund(ctx, eventID, intentID); err != nil {
			metrics.WebhookEvents.WithLabelValues(eventType, "error").Inc()
			_ = c.Error(err)
			return
		}
		metrics.WebhookEvents.WithLabelValues(eventType, "processed").Inc()
	default:
		slog.InfoContext(ctx, "received unknown webhook event type, ignoring", "eventType", eventType, "eventID", eventID)
	}

	c.JSON(http.StatusOK, gin.H{"received": true})
}
```

Note: remove the unused `"context"` import since it's not directly used (context comes from `c.Request.Context()`).

- [ ] **Step 2: Add orderID context and duplicate metrics to webhook service**

In `go/payment-service/internal/service/webhook.go`, add the metrics import. Replace the import block (lines 3-11):

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)
```

Then replace the `HandlePaymentSucceeded` method (lines 53-85):

```go
func (s *WebhookService) HandlePaymentSucceeded(ctx context.Context, eventID, intentID string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.succeeded")
	if err != nil {
		return fmt.Errorf("dedup payment succeeded: %w", err)
	}
	if !inserted {
		slog.InfoContext(ctx, "duplicate payment_intent.succeeded event, skipping", "eventID", eventID)
		metrics.WebhookEvents.WithLabelValues("payment_intent.succeeded", "duplicate").Inc()
		return nil
	}

	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment for succeeded event: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusSucceeded); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "succeeded", "error", err)
		return fmt.Errorf("update payment status to succeeded: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.confirmed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment confirmed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment confirmed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment succeeded", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}
```

Replace the `HandlePaymentFailed` method (lines 89-121):

```go
func (s *WebhookService) HandlePaymentFailed(ctx context.Context, eventID, intentID string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.payment_failed")
	if err != nil {
		return fmt.Errorf("dedup payment failed: %w", err)
	}
	if !inserted {
		slog.InfoContext(ctx, "duplicate payment_intent.payment_failed event, skipping", "eventID", eventID)
		metrics.WebhookEvents.WithLabelValues("payment_intent.payment_failed", "duplicate").Inc()
		return nil
	}

	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment for failed event: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusFailed); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "failed", "error", err)
		return fmt.Errorf("update payment status to failed: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.failed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment failed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment failed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment failed", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}
```

Replace the `HandleRefund` method (lines 125-145):

```go
func (s *WebhookService) HandleRefund(ctx context.Context, eventID, intentID string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "charge.refunded")
	if err != nil {
		return fmt.Errorf("dedup refund: %w", err)
	}
	if !inserted {
		slog.InfoContext(ctx, "duplicate charge.refunded event, skipping", "eventID", eventID)
		metrics.WebhookEvents.WithLabelValues("charge.refunded", "duplicate").Inc()
		return nil
	}

	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment for refund event: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusRefunded); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "refunded", "error", err)
		return fmt.Errorf("update payment status to refunded: %w", err)
	}

	slog.InfoContext(ctx, "payment refunded", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}
```

- [ ] **Step 3: Build and run tests**

Run: `cd go/payment-service && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add go/payment-service/internal/handler/webhook.go go/payment-service/internal/service/webhook.go
git commit -m "feat(payment): add webhook logging, metrics, and orderID error context"
```

---

### Task 6: Add AMQP Trace Injection to Payment Outbox Poller

**Files:**
- Modify: `go/payment-service/internal/outbox/poller.go`

- [ ] **Step 1: Add tracing import and inject headers**

In `go/payment-service/internal/outbox/poller.go`, add the tracing import. Replace the import block (lines 3-13):

```go
import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)
```

Then replace the publish loop in the `poll` method (lines 75-105):

```go
	for _, msg := range messages {
		headers := make(amqp.Table)
		tracing.InjectAMQP(ctx, headers)

		err := p.ch.PublishWithContext(
			ctx,
			msg.Exchange,
			msg.RoutingKey,
			false, // mandatory
			false, // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp.Persistent,
				MessageId:    msg.ID.String(),
				Headers:      headers,
				Body:         msg.Payload,
			},
		)
		if err != nil {
			slog.ErrorContext(ctx, "outbox poller: failed to publish message",
				"messageID", msg.ID,
				"exchange", msg.Exchange,
				"routingKey", msg.RoutingKey,
				"error", err,
			)
			metrics.OutboxPublish.WithLabelValues("error").Inc()
			continue
		}

		metrics.OutboxPublish.WithLabelValues("success").Inc()

		if markErr := p.fetcher.MarkPublished(ctx, msg.ID); markErr != nil {
			slog.ErrorContext(ctx, "outbox poller: failed to mark message published",
				"messageID", msg.ID,
				"error", markErr,
			)
		}
	}
```

- [ ] **Step 2: Build and run tests**

Run: `cd go/payment-service && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add go/payment-service/internal/outbox/poller.go
git commit -m "fix(payment): inject AMQP trace context in outbox poller"
```

---

### Task 7: Add gRPC Payment Created Metric

**Files:**
- Modify: `go/payment-service/internal/grpcserver/server.go`

- [ ] **Step 1: Find the gRPC CreatePayment handler and add the metric**

Read `go/payment-service/internal/grpcserver/server.go`. Add a metrics import and increment `metrics.PaymentsCreated` after a successful or failed payment creation.

At the successful return path:
```go
metrics.PaymentsCreated.WithLabelValues("succeeded").Inc()
```

At error return paths:
```go
metrics.PaymentsCreated.WithLabelValues("failed").Inc()
```

- [ ] **Step 2: Build and run tests**

Run: `cd go/payment-service && go build ./... && go test ./... -v -race`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add go/payment-service/internal/grpcserver/server.go
git commit -m "feat(payment): add gRPC payment creation metrics"
```

---

### Task 8: Run Go Preflight Checks

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: All lint and tests pass across all Go services

- [ ] **Step 2: Fix any lint issues**

If golangci-lint reports issues with the new code, fix them.

- [ ] **Step 3: Commit any lint fixes**

```bash
git commit -am "style: fix lint issues from observability changes"
```

---

### Task 9: Add Decomposed Services Dashboard Panels

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-dashboards.yml`

- [ ] **Step 1: Add Decomposed Services RED row to go-services dashboard**

In the `go-services.json` section of `k8s/monitoring/configmaps/grafana-dashboards.yml`, after the last panel (ID 25, Consumer Errors at y=36), add a new row and three panels. The next available panel ID is 26, next y position is 42.

Add a row header panel:
```json
{
  "collapsed": false,
  "gridPos": {"h": 1, "w": 24, "x": 0, "y": 42},
  "id": 26,
  "title": "Decomposed Services",
  "type": "row"
}
```

Add per-service request rate panel (ID 27):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "reqps", "color": {"mode": "palette-classic"}}, "overrides": []},
  "gridPos": {"h": 6, "w": 8, "x": 0, "y": 43},
  "id": 27,
  "title": "Request Rate by Service",
  "type": "timeseries",
  "targets": [
    {"expr": "sum by (service) (rate(http_requests_total{service=~\"go-(order|cart|payment|product)-service\"}[5m]))", "legendFormat": "{{service}}", "refId": "A"}
  ]
}
```

Add per-service error rate panel (ID 28):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "percent", "color": {"mode": "palette-classic"}, "thresholds": {"steps": [{"color": "green", "value": null}, {"color": "red", "value": 2}]}}, "overrides": []},
  "gridPos": {"h": 6, "w": 8, "x": 8, "y": 43},
  "id": 28,
  "title": "Error Rate by Service",
  "type": "timeseries",
  "targets": [
    {"expr": "sum by (service) (rate(http_requests_total{service=~\"go-(order|cart|payment|product)-service\",status=~\"5..\"}[5m])) / sum by (service) (rate(http_requests_total{service=~\"go-(order|cart|payment|product)-service\"}[5m])) * 100", "legendFormat": "{{service}}", "refId": "A"}
  ]
}
```

Add per-service p95 latency panel (ID 29):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "s", "color": {"mode": "palette-classic"}, "thresholds": {"steps": [{"color": "green", "value": null}, {"color": "yellow", "value": 1}, {"color": "red", "value": 2}]}}, "overrides": []},
  "gridPos": {"h": 6, "w": 8, "x": 16, "y": 43},
  "id": 29,
  "title": "p95 Latency by Service",
  "type": "timeseries",
  "targets": [
    {"expr": "histogram_quantile(0.95, sum by (service, le) (rate(http_request_duration_seconds_bucket{service=~\"go-(order|cart|payment|product)-service\"}[5m])))", "legendFormat": "{{service}}", "refId": "A"}
  ]
}
```

- [ ] **Step 2: Add Saga & Payment Health row**

Add a row header (ID 30) and four panels after the decomposed services row:

Row header:
```json
{
  "collapsed": false,
  "gridPos": {"h": 1, "w": 24, "x": 0, "y": 49},
  "id": 30,
  "title": "Saga & Payment Health",
  "type": "row"
}
```

Saga completion rate (ID 31):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "short", "color": {"mode": "palette-classic"}}, "overrides": []},
  "gridPos": {"h": 6, "w": 6, "x": 0, "y": 50},
  "id": 31,
  "title": "Orders by Status",
  "type": "timeseries",
  "targets": [
    {"expr": "sum by (status) (rate(ecommerce_orders_placed_total[5m]))", "legendFormat": "{{status}}", "refId": "A"}
  ]
}
```

Circuit breaker state (ID 32):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "short", "color": {"mode": "palette-classic"}, "mappings": [{"options": {"0": {"text": "CLOSED"}, "1": {"text": "HALF-OPEN"}, "2": {"text": "OPEN"}}, "type": "value"}]}, "overrides": []},
  "gridPos": {"h": 6, "w": 6, "x": 6, "y": 50},
  "id": 32,
  "title": "Circuit Breaker State",
  "type": "stat",
  "targets": [
    {"expr": "circuit_breaker_state", "legendFormat": "{{name}}", "refId": "A"}
  ]
}
```

Webhook throughput (ID 33):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "ops", "color": {"mode": "palette-classic"}}, "overrides": []},
  "gridPos": {"h": 6, "w": 6, "x": 12, "y": 50},
  "id": 33,
  "title": "Payment Webhooks",
  "type": "timeseries",
  "targets": [
    {"expr": "sum by (outcome) (rate(payment_webhook_events_total[5m]))", "legendFormat": "{{outcome}}", "refId": "A"}
  ]
}
```

Outbox publish rate (ID 34):
```json
{
  "datasource": {"type": "prometheus", "uid": "PBFA97CFB590B2093"},
  "fieldConfig": {"defaults": {"unit": "ops", "color": {"mode": "palette-classic"}}, "overrides": []},
  "gridPos": {"h": 6, "w": 6, "x": 18, "y": 50},
  "id": 34,
  "title": "Outbox Publish",
  "type": "timeseries",
  "targets": [
    {"expr": "sum by (outcome) (rate(payment_outbox_publish_total[5m]))", "legendFormat": "{{outcome}}", "refId": "A"}
  ]
}
```

- [ ] **Step 3: Validate YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/grafana-dashboards.yml')); print('valid')"`
Expected: `valid`

- [ ] **Step 4: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-dashboards.yml
git commit -m "feat(monitoring): add decomposed services and saga health dashboard panels"
```

---

### Task 10: Add Alert Rules for Decomposed Services

**Files:**
- Modify: `k8s/monitoring/configmaps/grafana-alerting.yml`

- [ ] **Step 1: Add error rate alerts for cart, payment, product, auth**

In `k8s/monitoring/configmaps/grafana-alerting.yml`, in the `Application SLOs` group (after the last existing rule), add these alert rules. Follow the exact structure of `go-order-error-rate` (lines 530-574) — change uid, title, service name in expr, and summary:

```yaml
          - uid: go-cart-error-rate
            title: Go Cart Service Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-cart-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-cart-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go cart-service error rate is above 2%"

          - uid: go-payment-error-rate
            title: Go Payment Service Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-payment-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-payment-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go payment-service error rate is above 2%"

          - uid: go-product-error-rate
            title: Go Product Service Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-product-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-product-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go product-service error rate is above 2%"

          - uid: go-auth-error-rate
            title: Go Auth Service Error Rate High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    sum(rate(http_requests_total{service="go-auth-service",status=~"5.."}[5m]))
                    / sum(rate(http_requests_total{service="go-auth-service"}[5m]))
                    * 100
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go auth-service error rate is above 2%"
```

- [ ] **Step 2: Add latency alerts for cart, payment, product**

After the error rate alerts, add latency alerts. Use the same pattern as `go-order-latency-high` but with `service` label changed:

```yaml
          - uid: go-cart-latency-high
            title: Go Cart Service Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum by (le) (rate(http_request_duration_seconds_bucket{service="go-cart-service"}[5m]))
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go cart-service p95 latency is above 2s"

          - uid: go-payment-latency-high
            title: Go Payment Service Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum by (le) (rate(http_request_duration_seconds_bucket{service="go-payment-service"}[5m]))
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go payment-service p95 latency is above 2s"

          - uid: go-product-latency-high
            title: Go Product Service Latency High
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: >-
                    histogram_quantile(0.95,
                      sum by (le) (rate(http_request_duration_seconds_bucket{service="go-product-service"}[5m]))
                    )
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 2
                  refId: C
            for: 5m
            labels:
              severity: warning
            annotations:
              summary: "Go product-service p95 latency is above 2s"
```

- [ ] **Step 3: Add circuit breaker operational alert**

Add a new group after the `Streaming Analytics` group (at the end of the groups list):

```yaml
      - orgId: 1
        name: Operational
        folder: Infrastructure Alerts
        interval: 1m
        rules:
          - uid: circuit-breaker-open
            title: Circuit Breaker Open
            condition: C
            data:
              - refId: A
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: PBFA97CFB590B2093
                model:
                  expr: circuit_breaker_state == 2
                  instant: true
                  refId: A
              - refId: B
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: reduce
                  expression: A
                  reducer: last
                  refId: B
              - refId: C
                relativeTimeRange:
                  from: 300
                  to: 0
                datasourceUid: __expr__
                model:
                  type: threshold
                  expression: B
                  conditions:
                    - evaluator:
                        type: gt
                        params:
                          - 0
                  refId: C
            for: 2m
            labels:
              severity: critical
            annotations:
              summary: "Circuit breaker is OPEN — a dependency is unavailable"
```

- [ ] **Step 4: Validate YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('k8s/monitoring/configmaps/grafana-alerting.yml')); print('valid')"`
Expected: `valid`

- [ ] **Step 5: Commit**

```bash
git add k8s/monitoring/configmaps/grafana-alerting.yml
git commit -m "feat(monitoring): add error rate, latency, and circuit breaker alerts for decomposed services"
```

---

### Task 11: Final Validation and Push

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: All lint and tests pass

- [ ] **Step 2: Validate all K8s YAML files**

Run:
```bash
python3 -c "
import yaml, glob
for f in glob.glob('k8s/monitoring/configmaps/*.yml'):
    yaml.safe_load(open(f))
    print(f'OK: {f}')
"
```
Expected: All files OK

- [ ] **Step 3: Push to qa**

Run: `git push origin qa`
