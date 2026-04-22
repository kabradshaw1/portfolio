# DLQ Replay & Async Integration Tests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin REST endpoints to inspect/replay DLQ messages in order-service, and add integration tests for both RabbitMQ saga and Kafka consumer flows.

**Architecture:** DLQ client wraps RabbitMQ channel operations (list via basic.get with requeue, replay by republishing to original exchange/routing key). Admin handler exposes two endpoints on `/admin` group. Integration tests use testcontainers for real broker instances, gated behind `//go:build integration`.

**Tech Stack:** Go, RabbitMQ (amqp091-go), Kafka (segmentio/kafka-go), testcontainers-go, Gin, Prometheus

---

## File Structure

| File | Action | Purpose |
|------|--------|---------|
| `go/order-service/internal/saga/dlq.go` | Create | DLQ client: List() and Replay() |
| `go/order-service/internal/saga/dlq_test.go` | Create | Unit tests for DLQ client |
| `go/order-service/internal/handler/admin.go` | Create | Admin HTTP handlers |
| `go/order-service/internal/handler/admin_test.go` | Create | Unit tests for admin handlers |
| `go/order-service/internal/saga/metrics.go` | Modify | Add `saga_dlq_replayed_total` counter |
| `go/order-service/cmd/server/routes.go` | Modify | Register `/admin` route group |
| `go/order-service/cmd/server/main.go` | Modify | Create DLQClient, pass to admin handler |
| `go/order-service/internal/integration/saga_test.go` | Create | RabbitMQ saga + DLQ integration tests |
| `go/analytics-service/internal/integration/testutil/containers.go` | Create | Kafka testcontainer setup |
| `go/analytics-service/internal/integration/consumer_test.go` | Create | Kafka consumer integration tests |
| `go/analytics-service/go.mod` | Modify | Add testcontainers dependencies |
| `Makefile` | Modify | Add analytics-service to `preflight-go-integration` |

---

### Task 1: DLQ Client

**Files:**
- Create: `go/order-service/internal/saga/dlq.go`
- Create: `go/order-service/internal/saga/dlq_test.go`

- [ ] **Step 1: Write the DLQ message type and client interface**

Create `go/order-service/internal/saga/dlq.go`:

```go
package saga

import (
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DLQMessage represents a message sitting in the dead-letter queue.
type DLQMessage struct {
	Index      int                    `json:"index"`
	RoutingKey string                 `json:"routing_key"`
	Exchange   string                 `json:"exchange"`
	Timestamp  time.Time              `json:"timestamp"`
	RetryCount int32                  `json:"retry_count"`
	Headers    map[string]interface{} `json:"headers"`
	Body       json.RawMessage        `json:"body"`
}

// maxDLQList is the upper bound on messages returned by List.
const maxDLQList = 200

// DLQClient provides operations on the saga dead-letter queue.
type DLQClient struct {
	ch *amqp.Channel
}

// NewDLQClient creates a DLQ client wrapping the given channel.
func NewDLQClient(ch *amqp.Channel) *DLQClient {
	return &DLQClient{ch: ch}
}
```

- [ ] **Step 2: Implement List method**

Append to `go/order-service/internal/saga/dlq.go`:

```go
// List peeks at up to limit messages from the DLQ without removing them.
// Messages are fetched via basic.get and immediately nacked with requeue.
func (d *DLQClient) List(limit int) ([]DLQMessage, error) {
	if limit <= 0 || limit > maxDLQList {
		limit = 50
	}

	var messages []DLQMessage

	for i := 0; i < limit; i++ {
		msg, ok, err := d.ch.Get(SagaDLQ, false) // autoAck=false
		if err != nil {
			return nil, fmt.Errorf("get from DLQ: %w", err)
		}
		if !ok {
			break // queue is empty
		}

		var retryCount int32
		if rc, exists := msg.Headers["x-retry-count"]; exists {
			if v, ok := rc.(int32); ok {
				retryCount = v
			}
		}

		// Extract original routing key and exchange from x-death headers.
		routingKey, exchange := extractXDeath(msg.Headers)
		if routingKey == "" {
			routingKey = msg.RoutingKey
		}
		if exchange == "" {
			exchange = msg.Exchange
		}

		messages = append(messages, DLQMessage{
			Index:      i,
			RoutingKey: routingKey,
			Exchange:   exchange,
			Timestamp:  msg.Timestamp,
			RetryCount: retryCount,
			Headers:    msg.Headers,
			Body:       json.RawMessage(msg.Body),
		})

		// Requeue the message so it stays in DLQ.
		if err := msg.Nack(false, true); err != nil {
			return nil, fmt.Errorf("nack DLQ message: %w", err)
		}
	}

	return messages, nil
}

// extractXDeath reads the original routing key and exchange from RabbitMQ's
// x-death header, which is automatically added when a message is dead-lettered.
func extractXDeath(headers amqp.Table) (routingKey, exchange string) {
	xdeath, ok := headers["x-death"]
	if !ok {
		return "", ""
	}

	deaths, ok := xdeath.([]interface{})
	if !ok || len(deaths) == 0 {
		return "", ""
	}

	first, ok := deaths[0].(amqp.Table)
	if !ok {
		return "", ""
	}

	if rks, ok := first["routing-keys"].([]interface{}); ok && len(rks) > 0 {
		if rk, ok := rks[0].(string); ok {
			routingKey = rk
		}
	}
	if ex, ok := first["exchange"].(string); ok {
		exchange = ex
	}

	return routingKey, exchange
}
```

- [ ] **Step 3: Implement Replay method**

Append to `go/order-service/internal/saga/dlq.go`:

```go
// Replay removes the message at the given index from the DLQ and republishes
// it to its original exchange with the original routing key.
func (d *DLQClient) Replay(index int) (*DLQMessage, error) {
	if index < 0 {
		return nil, fmt.Errorf("invalid index: %d", index)
	}

	// Consume messages up to the target index. Non-target messages are
	// nacked with requeue so they remain in the DLQ.
	for i := 0; i <= index; i++ {
		msg, ok, err := d.ch.Get(SagaDLQ, false)
		if err != nil {
			return nil, fmt.Errorf("get from DLQ at position %d: %w", i, err)
		}
		if !ok {
			return nil, fmt.Errorf("DLQ exhausted at position %d, target index %d not found", i, index)
		}

		if i < index {
			// Not the target — put it back.
			if err := msg.Nack(false, true); err != nil {
				return nil, fmt.Errorf("nack non-target message at %d: %w", i, err)
			}
			continue
		}

		// This is the target message. Ack to remove from DLQ.
		if err := msg.Ack(false); err != nil {
			return nil, fmt.Errorf("ack target message: %w", err)
		}

		// Extract original destination.
		routingKey, exchange := extractXDeath(msg.Headers)
		if routingKey == "" {
			routingKey = msg.RoutingKey
		}
		if exchange == "" {
			exchange = SagaExchange
		}

		// Increment retry count.
		if msg.Headers == nil {
			msg.Headers = make(amqp.Table)
		}
		var retryCount int32
		if rc, ok := msg.Headers["x-retry-count"].(int32); ok {
			retryCount = rc
		}
		retryCount++
		msg.Headers["x-retry-count"] = retryCount

		// Republish to original destination.
		err = d.ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
			ContentType: msg.ContentType,
			Headers:     msg.Headers,
			Body:        msg.Body,
			Timestamp:   msg.Timestamp,
		})
		if err != nil {
			return nil, fmt.Errorf("republish to %s/%s: %w", exchange, routingKey, err)
		}

		SagaDLQReplayed.WithLabelValues(routingKey, "success").Inc()

		return &DLQMessage{
			Index:      index,
			RoutingKey: routingKey,
			Exchange:   exchange,
			Timestamp:  msg.Timestamp,
			RetryCount: retryCount,
			Headers:    msg.Headers,
			Body:       json.RawMessage(msg.Body),
		}, nil
	}

	return nil, fmt.Errorf("index %d not reached", index)
}
```

- [ ] **Step 4: Write unit test for extractXDeath**

Create `go/order-service/internal/saga/dlq_test.go`:

```go
package saga

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestExtractXDeath_ValidHeaders(t *testing.T) {
	headers := amqp.Table{
		"x-death": []interface{}{
			amqp.Table{
				"exchange":     "ecommerce.saga",
				"routing-keys": []interface{}{"saga.cart.commands"},
				"queue":        "saga.cart.commands",
				"reason":       "rejected",
			},
		},
	}

	rk, ex := extractXDeath(headers)
	if rk != "saga.cart.commands" {
		t.Errorf("expected routing key saga.cart.commands, got %s", rk)
	}
	if ex != "ecommerce.saga" {
		t.Errorf("expected exchange ecommerce.saga, got %s", ex)
	}
}

func TestExtractXDeath_MissingHeader(t *testing.T) {
	rk, ex := extractXDeath(amqp.Table{})
	if rk != "" || ex != "" {
		t.Errorf("expected empty strings, got rk=%q ex=%q", rk, ex)
	}
}

func TestExtractXDeath_EmptyDeathList(t *testing.T) {
	headers := amqp.Table{
		"x-death": []interface{}{},
	}
	rk, ex := extractXDeath(headers)
	if rk != "" || ex != "" {
		t.Errorf("expected empty strings, got rk=%q ex=%q", rk, ex)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go/order-service && go test ./internal/saga/... -v -run TestExtractXDeath`
Expected: 3 tests PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/saga/dlq.go go/order-service/internal/saga/dlq_test.go
git commit -m "feat(order): add DLQ client with list and replay operations

Closes #97 (partial)"
```

---

### Task 2: DLQ Replayed Prometheus Metric

**Files:**
- Modify: `go/order-service/internal/saga/metrics.go:8-24`

- [ ] **Step 1: Add the replayed counter to metrics.go**

Add after the existing `SagaDLQTotal` var in `go/order-service/internal/saga/metrics.go`:

```go
	SagaDLQReplayed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "saga_dlq_replayed_total",
		Help: "Messages replayed from the saga dead letter queue.",
	}, []string{"routing_key", "outcome"})
```

- [ ] **Step 2: Verify compilation**

Run: `cd go/order-service && go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add go/order-service/internal/saga/metrics.go
git commit -m "feat(order): add saga_dlq_replayed_total Prometheus metric"
```

---

### Task 3: Admin HTTP Handlers

**Files:**
- Create: `go/order-service/internal/handler/admin.go`
- Create: `go/order-service/internal/handler/admin_test.go`

- [ ] **Step 1: Write the admin handler**

Create `go/order-service/internal/handler/admin.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// DLQLister abstracts the DLQ read operations for testability.
type DLQLister interface {
	List(limit int) ([]saga.DLQMessage, error)
	Replay(index int) (*saga.DLQMessage, error)
}

// AdminHandler exposes DLQ inspection and replay endpoints.
type AdminHandler struct {
	dlq DLQLister
}

// NewAdminHandler creates an admin handler.
func NewAdminHandler(dlq DLQLister) *AdminHandler {
	return &AdminHandler{dlq: dlq}
}

// ListDLQ returns messages currently in the dead-letter queue.
func (h *AdminHandler) ListDLQ(c *gin.Context) {
	limit := 50
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && l > 0 {
		limit = l
	}

	messages, err := h.dlq.List(limit)
	if err != nil {
		_ = c.Error(apperror.Internal("DLQ_LIST_FAILED", err.Error()))
		return
	}

	if messages == nil {
		messages = []saga.DLQMessage{}
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages, "count": len(messages)})
}

// ReplayDLQ replays a single message from the dead-letter queue.
func (h *AdminHandler) ReplayDLQ(c *gin.Context) {
	var req struct {
		Index int `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_BODY", "request body must contain index"))
		return
	}

	msg, err := h.dlq.Replay(req.Index)
	if err != nil {
		_ = c.Error(apperror.NotFound("DLQ_MESSAGE_NOT_FOUND", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"replayed": msg})
}
```

- [ ] **Step 2: Write admin handler unit tests**

Create `go/order-service/internal/handler/admin_test.go`:

```go
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type mockDLQ struct {
	messages []saga.DLQMessage
	replayErr error
}

func (m *mockDLQ) List(limit int) ([]saga.DLQMessage, error) {
	if limit > len(m.messages) {
		return m.messages, nil
	}
	return m.messages[:limit], nil
}

func (m *mockDLQ) Replay(index int) (*saga.DLQMessage, error) {
	if m.replayErr != nil {
		return nil, m.replayErr
	}
	if index >= len(m.messages) {
		return nil, fmt.Errorf("index %d out of range", index)
	}
	return &m.messages[index], nil
}

func setupAdminRouter(dlq DLQLister) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	h := NewAdminHandler(dlq)
	admin := r.Group("/admin")
	{
		admin.GET("/dlq/messages", h.ListDLQ)
		admin.POST("/dlq/replay", h.ReplayDLQ)
	}
	return r
}

func TestListDLQ_Empty(t *testing.T) {
	router := setupAdminRouter(&mockDLQ{})

	req, _ := http.NewRequest("GET", "/admin/dlq/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Messages []saga.DLQMessage `json:"messages"`
		Count    int               `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("expected count=0, got %d", resp.Count)
	}
}

func TestListDLQ_WithMessages(t *testing.T) {
	dlq := &mockDLQ{
		messages: []saga.DLQMessage{
			{Index: 0, RoutingKey: "saga.cart.commands", Exchange: "ecommerce.saga", Timestamp: time.Now(), Body: json.RawMessage(`{"command":"reserve.items"}`)},
		},
	}
	router := setupAdminRouter(dlq)

	req, _ := http.NewRequest("GET", "/admin/dlq/messages?limit=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("expected count=1, got %d", resp.Count)
	}
}

func TestReplayDLQ_Success(t *testing.T) {
	dlq := &mockDLQ{
		messages: []saga.DLQMessage{
			{Index: 0, RoutingKey: "saga.cart.commands", Exchange: "ecommerce.saga", Body: json.RawMessage(`{}`)},
		},
	}
	router := setupAdminRouter(dlq)

	body := `{"index": 0}`
	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplayDLQ_NotFound(t *testing.T) {
	dlq := &mockDLQ{replayErr: fmt.Errorf("index 5 not found")}
	router := setupAdminRouter(dlq)

	body := `{"index": 5}`
	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplayDLQ_InvalidBody(t *testing.T) {
	router := setupAdminRouter(&mockDLQ{})

	req, _ := http.NewRequest("POST", "/admin/dlq/replay", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 3: Run tests to verify they pass**

Run: `cd go/order-service && go test ./internal/handler/... -v -run TestListDLQ -run TestReplayDLQ`
Expected: All 5 tests PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-service/internal/handler/admin.go go/order-service/internal/handler/admin_test.go
git commit -m "feat(order): add admin DLQ list and replay HTTP handlers"
```

---

### Task 4: Wire Admin Endpoints into Server

**Files:**
- Modify: `go/order-service/cmd/server/routes.go:17-52`
- Modify: `go/order-service/cmd/server/main.go:80-108`

- [ ] **Step 1: Create DLQClient and AdminHandler in main.go**

In `go/order-service/cmd/server/main.go`, after the saga topology declaration (line 83) and before `orderSvc` creation (line 100), add:

```go
	// Create DLQ client for admin endpoints.
	dlqClient := saga.NewDLQClient(ch)
```

Then update the `setupRouter` call (line 103) to pass the admin handler:

```go
	router := setupRouter(cfg,
		handler.NewOrderHandler(orderSvc),
		handler.NewReturnHandler(returnSvc),
		handler.NewHealthHandler(pool, redisClient),
		handler.NewAdminHandler(dlqClient),
		redisClient,
	)
```

- [ ] **Step 2: Update setupRouter signature and add admin routes**

In `go/order-service/cmd/server/routes.go`, update the function signature to accept `*handler.AdminHandler` and add the admin route group after the authenticated routes block:

```go
func setupRouter(
	cfg Config,
	orderHandler *handler.OrderHandler,
	returnHandler *handler.ReturnHandler,
	healthHandler *handler.HealthHandler,
	adminHandler *handler.AdminHandler,
	redisClient *redis.Client,
) *gin.Engine {
```

Add after the `auth` group's closing brace (after line 49):

```go
	// Admin routes — no auth, protected by network boundary.
	admin := router.Group("/admin")
	{
		admin.GET("/dlq/messages", adminHandler.ListDLQ)
		admin.POST("/dlq/replay", adminHandler.ReplayDLQ)
	}
```

- [ ] **Step 3: Verify compilation**

Run: `cd go/order-service && go build ./cmd/server/...`
Expected: No errors

- [ ] **Step 4: Run all unit tests**

Run: `cd go/order-service && go test ./... -v -race`
Expected: All tests PASS

- [ ] **Step 5: Commit**

```bash
git add go/order-service/cmd/server/main.go go/order-service/cmd/server/routes.go
git commit -m "feat(order): wire admin DLQ endpoints into server router"
```

---

### Task 5: RabbitMQ Saga Integration Tests

**Files:**
- Create: `go/order-service/internal/integration/saga_test.go`

- [ ] **Step 1: Write the saga happy-path integration test**

Create `go/order-service/internal/integration/saga_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
)

// TestSaga_HappyPath verifies the full saga round-trip through live RabbitMQ:
// checkout → reserve.items command → simulated items.reserved reply → order completes.
func TestSaga_HappyPath(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	// Declare saga topology on the test RabbitMQ.
	if err := saga.DeclareTopology(infra.RabbitCh); err != nil {
		t.Fatalf("declare topology: %v", err)
	}

	// Seed products.
	ids := testutil.SeedProducts(ctx, t, infra.Pool, 2)
	productID1, _ := uuid.Parse(ids[0])
	productID2, _ := uuid.Parse(ids[1])

	breaker := newBreaker()
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)

	// Real saga publisher over test RabbitMQ.
	sagaPub := saga.NewPublisher(infra.RabbitCh)

	// Stock checker that always returns available.
	stock := &alwaysAvailableStock{}

	orch := saga.NewOrchestrator(orderRepo, sagaPub, stock, kafka.NopProducer{})

	// Stub cart client that returns seeded items and clears on demand.
	cartClient := &testCartClient{
		items: []model.CartItem{
			{ProductID: productID1, Quantity: 2, ProductPrice: 1000},
			{ProductID: productID2, Quantity: 1, ProductPrice: 2000},
		},
	}
	orderSvc := service.NewOrderService(orderRepo, cartClient, orch)

	userID := uuid.New()

	// Checkout — creates order and saga publishes reserve.items command.
	order, err := orderSvc.Checkout(ctx, userID)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Consume the reserve.items command from the cart commands queue.
	cmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	var sagaCmd saga.Command
	if err := json.Unmarshal(cmd.Body, &sagaCmd); err != nil {
		t.Fatalf("unmarshal command: %v", err)
	}
	if sagaCmd.Command != saga.CmdReserveItems {
		t.Errorf("expected command %s, got %s", saga.CmdReserveItems, sagaCmd.Command)
	}
	if sagaCmd.OrderID != order.ID.String() {
		t.Errorf("expected order ID %s, got %s", order.ID, sagaCmd.OrderID)
	}
	_ = cmd.Ack(false)

	// Simulate cart-service reply: publish items.reserved event.
	replyEvt := saga.Event{
		Event:     saga.EvtItemsReserved,
		OrderID:   order.ID.String(),
		UserID:    userID.String(),
		Success:   true,
		Timestamp: time.Now().UTC(),
	}
	replyBody, _ := json.Marshal(replyEvt)
	err = infra.RabbitCh.Publish(saga.SagaExchange, "saga.order.events", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        replyBody,
	})
	if err != nil {
		t.Fatalf("publish items.reserved: %v", err)
	}

	// Start saga consumer to process the reply.
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()

	// Open a new channel for the consumer (avoid channel reuse conflicts).
	consumerCh, err := infra.RabbitConn.Channel()
	if err != nil {
		t.Fatalf("open consumer channel: %v", err)
	}
	defer consumerCh.Close()

	consumer := saga.NewConsumer(orch)
	go func() {
		_ = consumer.Start(consumerCtx, consumerCh)
	}()

	// The orchestrator should advance through ITEMS_RESERVED → STOCK_VALIDATED
	// and publish clear.cart, then once cart.cleared is received → COMPLETED.

	// Consume the clear.cart command.
	clearCmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	var clearSagaCmd saga.Command
	if err := json.Unmarshal(clearCmd.Body, &clearSagaCmd); err != nil {
		t.Fatalf("unmarshal clear command: %v", err)
	}
	if clearSagaCmd.Command != saga.CmdClearCart {
		t.Errorf("expected command %s, got %s", saga.CmdClearCart, clearSagaCmd.Command)
	}
	_ = clearCmd.Ack(false)

	// Simulate cart.cleared reply.
	clearedEvt := saga.Event{
		Event:     saga.EvtCartCleared,
		OrderID:   order.ID.String(),
		UserID:    userID.String(),
		Success:   true,
		Timestamp: time.Now().UTC(),
	}
	clearedBody, _ := json.Marshal(clearedEvt)
	err = infra.RabbitCh.Publish(saga.SagaExchange, "saga.order.events", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        clearedBody,
	})
	if err != nil {
		t.Fatalf("publish cart.cleared: %v", err)
	}

	// Poll DB until order reaches COMPLETED.
	pollUntilSagaStep(t, ctx, orderRepo, order.ID, saga.StepCompleted, 10*time.Second)

	consumerCancel()
}

// testCartClient is a stub CartClient for integration tests.
type testCartClient struct {
	items []model.CartItem
}

func (c *testCartClient) GetByUser(_ context.Context, _ uuid.UUID) ([]model.CartItem, error) {
	return c.items, nil
}

func (c *testCartClient) ClearCart(_ context.Context, _ uuid.UUID) error {
	c.items = nil
	return nil
}

// alwaysAvailableStock is a StockChecker that always returns true.
type alwaysAvailableStock struct{}

func (s *alwaysAvailableStock) CheckAvailability(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
	return true, nil
}

// consumeOne fetches a single message from the named queue within the timeout.
func consumeOne(t *testing.T, ch *amqp.Channel, queue string, timeout time.Duration) amqp.Delivery {
	t.Helper()
	deadline := time.After(timeout)
	for {
		msg, ok, err := ch.Get(queue, false)
		if err != nil {
			t.Fatalf("get from %s: %v", queue, err)
		}
		if ok {
			return msg
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for message on %s", queue)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// pollUntilSagaStep polls the DB until the order's saga step matches the expected value.
func pollUntilSagaStep(t *testing.T, ctx context.Context, repo *repository.OrderRepository, orderID uuid.UUID, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		order, err := repo.FindByID(ctx, orderID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if order.SagaStep == expected {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for saga step %s, current: %s", expected, order.SagaStep)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `cd go/order-service && DOCKER_HOST=unix://${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/... -run TestSaga_HappyPath -v`
Expected: PASS

- [ ] **Step 3: Add the DLQ replay integration test**

Append to `go/order-service/internal/integration/saga_test.go`:

```go
// TestSaga_FailureToDLQ_Replay verifies that a rejected message lands in the
// DLQ and can be replayed back to its original queue via the admin endpoint.
func TestSaga_FailureToDLQ_Replay(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()

	if err := saga.DeclareTopology(infra.RabbitCh); err != nil {
		t.Fatalf("declare topology: %v", err)
	}

	// Drain any leftover messages from previous tests.
	_, _ = infra.RabbitCh.QueuePurge(saga.CartCommands, false)
	_, _ = infra.RabbitCh.QueuePurge(saga.SagaDLQ, false)

	// Publish a message directly to the cart commands queue.
	body := []byte(`{"command":"reserve.items","order_id":"test-order"}`)
	err := infra.RabbitCh.Publish(saga.SagaExchange, saga.CartCommandsKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers:     amqp.Table{"x-retry-count": int32(0)},
		Body:        body,
	})
	if err != nil {
		t.Fatalf("publish to cart commands: %v", err)
	}

	// Consume and nack without requeue — this triggers dead-lettering.
	msg := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = msg.Nack(false, false) // nack, no requeue → goes to DLX → DLQ

	// Poll DLQ until the message appears.
	dlqClient := saga.NewDLQClient(infra.RabbitCh)
	var dlqMessages []saga.DLQMessage
	deadline := time.After(5 * time.Second)
	for {
		dlqMessages, err = dlqClient.List(10)
		if err != nil {
			t.Fatalf("list DLQ: %v", err)
		}
		if len(dlqMessages) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for message in DLQ")
		case <-time.After(100 * time.Millisecond):
		}
	}

	if len(dlqMessages) != 1 {
		t.Fatalf("expected 1 DLQ message, got %d", len(dlqMessages))
	}

	// Verify DLQ message metadata.
	dlqMsg := dlqMessages[0]
	if dlqMsg.Index != 0 {
		t.Errorf("expected index=0, got %d", dlqMsg.Index)
	}

	// Replay the message.
	_ = ctx
	replayed, err := dlqClient.Replay(0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.RetryCount != 1 {
		t.Errorf("expected retry count=1, got %d", replayed.RetryCount)
	}

	// Verify the message reappears on the original queue.
	replayedMsg := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = replayedMsg.Ack(false)

	var cmd saga.Command
	if err := json.Unmarshal(replayedMsg.Body, &cmd); err != nil {
		t.Fatalf("unmarshal replayed message: %v", err)
	}
	if cmd.Command != "reserve.items" {
		t.Errorf("expected command reserve.items, got %s", cmd.Command)
	}

	// Verify DLQ is now empty.
	remaining, err := dlqClient.List(10)
	if err != nil {
		t.Fatalf("list DLQ after replay: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty DLQ after replay, got %d messages", len(remaining))
	}
}
```

- [ ] **Step 4: Add the compensation integration test**

Append to `go/order-service/internal/integration/saga_test.go`:

```go
// TestSaga_Compensation verifies that when stock validation fails, the
// orchestrator publishes a release.items command and the order reaches
// COMPENSATION_COMPLETE.
func TestSaga_Compensation(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	if err := saga.DeclareTopology(infra.RabbitCh); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
	_, _ = infra.RabbitCh.QueuePurge(saga.CartCommands, false)

	ids := testutil.SeedProducts(ctx, t, infra.Pool, 1)
	productID, _ := uuid.Parse(ids[0])

	breaker := newBreaker()
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)

	sagaPub := saga.NewPublisher(infra.RabbitCh)
	stock := &neverAvailableStock{}
	orch := saga.NewOrchestrator(orderRepo, sagaPub, stock, kafka.NopProducer{})

	cartClient := &testCartClient{
		items: []model.CartItem{
			{ProductID: productID, Quantity: 1, ProductPrice: 1000},
		},
	}
	orderSvc := service.NewOrderService(orderRepo, cartClient, orch)

	userID := uuid.New()

	order, err := orderSvc.Checkout(ctx, userID)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Consume reserve.items command.
	reserveCmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = reserveCmd.Ack(false)

	// Simulate items.reserved reply — stock check will fail in orchestrator.
	replyEvt := saga.Event{
		Event:     saga.EvtItemsReserved,
		OrderID:   order.ID.String(),
		UserID:    userID.String(),
		Success:   true,
		Timestamp: time.Now().UTC(),
	}
	replyBody, _ := json.Marshal(replyEvt)
	err = infra.RabbitCh.Publish(saga.SagaExchange, "saga.order.events", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        replyBody,
	})
	if err != nil {
		t.Fatalf("publish items.reserved: %v", err)
	}

	// Start consumer.
	consumerCtx, consumerCancel := context.WithCancel(ctx)
	defer consumerCancel()

	consumerCh, err := infra.RabbitConn.Channel()
	if err != nil {
		t.Fatalf("open consumer channel: %v", err)
	}
	defer consumerCh.Close()

	consumer := saga.NewConsumer(orch)
	go func() {
		_ = consumer.Start(consumerCtx, consumerCh)
	}()

	// Stock check fails → orchestrator should publish release.items.
	releaseCmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	var releaseSagaCmd saga.Command
	if err := json.Unmarshal(releaseCmd.Body, &releaseSagaCmd); err != nil {
		t.Fatalf("unmarshal release command: %v", err)
	}
	if releaseSagaCmd.Command != saga.CmdReleaseItems {
		t.Errorf("expected command %s, got %s", saga.CmdReleaseItems, releaseSagaCmd.Command)
	}
	_ = releaseCmd.Ack(false)

	// Simulate items.released reply.
	releasedEvt := saga.Event{
		Event:     saga.EvtItemsReleased,
		OrderID:   order.ID.String(),
		UserID:    userID.String(),
		Success:   true,
		Timestamp: time.Now().UTC(),
	}
	releasedBody, _ := json.Marshal(releasedEvt)
	err = infra.RabbitCh.Publish(saga.SagaExchange, "saga.order.events", false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        releasedBody,
	})
	if err != nil {
		t.Fatalf("publish items.released: %v", err)
	}

	// Poll until COMPENSATION_COMPLETE.
	pollUntilSagaStep(t, ctx, orderRepo, order.ID, saga.StepCompensationComplete, 10*time.Second)

	consumerCancel()
}

// neverAvailableStock is a StockChecker that always returns false.
type neverAvailableStock struct{}

func (s *neverAvailableStock) CheckAvailability(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
	return false, nil
}
```

- [ ] **Step 5: Run all saga integration tests**

Run: `cd go/order-service && DOCKER_HOST=unix://${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/... -run TestSaga -v`
Expected: All 3 saga tests PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/integration/saga_test.go
git commit -m "test(order): add RabbitMQ saga integration tests

Tests happy path, DLQ replay, and compensation flows
through live RabbitMQ via testcontainers.

Closes #97, closes #99 (partial)"
```

---

### Task 6: Kafka Integration Test Infrastructure

**Files:**
- Create: `go/analytics-service/internal/integration/testutil/containers.go`
- Modify: `go/analytics-service/go.mod`

- [ ] **Step 1: Add testcontainers dependency to analytics-service**

Run:
```bash
cd go/analytics-service && go get github.com/testcontainers/testcontainers-go github.com/testcontainers/testcontainers-go/modules/kafka && go mod tidy
```

- [ ] **Step 2: Create Kafka testcontainer setup**

Create `go/analytics-service/internal/integration/testutil/containers.go`:

```go
//go:build integration

package testutil

import (
	"context"
	"testing"

	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

// Infra holds live connections to Kafka spun up by testcontainers.
type Infra struct {
	KafkaBrokers []string

	kafkaContainer *tckafka.KafkaContainer
}

// SetupInfra starts a Kafka container and returns connection details.
func SetupInfra(ctx context.Context, t testing.TB) *Infra {
	t.Helper()

	kafkaC, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.6.0")
	if err != nil {
		t.Fatalf("start kafka container: %v", err)
	}

	brokers, err := kafkaC.Brokers(ctx)
	if err != nil {
		t.Fatalf("kafka brokers: %v", err)
	}

	infra := &Infra{
		KafkaBrokers:   brokers,
		kafkaContainer: kafkaC,
	}

	t.Cleanup(func() {
		cleanCtx := context.Background()
		if err := kafkaC.Terminate(cleanCtx); err != nil {
			t.Logf("terminate kafka container: %v", err)
		}
	})

	return infra
}

// Teardown terminates the Kafka container. For use in TestMain.
func (i *Infra) Teardown() {
	ctx := context.Background()
	_ = i.kafkaContainer.Terminate(ctx)
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd go/analytics-service && go build ./...`
Expected: No errors (integration files skipped without build tag, but go.mod is valid)

- [ ] **Step 4: Commit**

```bash
git add go/analytics-service/internal/integration/testutil/containers.go go/analytics-service/go.mod go/analytics-service/go.sum
git commit -m "test(analytics): add Kafka testcontainer infrastructure"
```

---

### Task 7: Kafka Consumer Integration Tests

**Files:**
- Create: `go/analytics-service/internal/integration/consumer_test.go`

- [ ] **Step 1: Write Kafka consumer integration tests**

Create `go/analytics-service/internal/integration/consumer_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

var sharedInfra *testutil.Infra

type mainTB struct{ testing.TB }

func (*mainTB) Helper()                       {}
func (*mainTB) Log(args ...any)               { fmt.Println(args...) }
func (*mainTB) Logf(f string, args ...any)    { fmt.Printf(f+"\n", args...) }
func (*mainTB) Fatal(args ...any)             { panic(fmt.Sprint(args...)) }
func (*mainTB) Fatalf(f string, args ...any)  { panic(fmt.Sprintf(f, args...)) }
func (*mainTB) Cleanup(_ func())              {}
func (*mainTB) Setenv(_ string, _ string)     {}
func (*mainTB) TempDir() string               { return os.TempDir() }
func (*mainTB) Parallel()                     {}
func (*mainTB) Skip(args ...any)              {}
func (*mainTB) Skipf(f string, args ...any)   {}
func (*mainTB) SkipNow()                      {}
func (*mainTB) Skipped() bool                 { return false }
func (*mainTB) Failed() bool                  { return false }
func (*mainTB) FailNow()                      { panic("FailNow") }
func (*mainTB) Fail()                         {}
func (*mainTB) Name() string                  { return "TestMain" }
func (*mainTB) Error(args ...any)             { fmt.Println(args...) }
func (*mainTB) Errorf(f string, args ...any)  { fmt.Printf(f+"\n", args...) }
func (*mainTB) Run(_ string, _ func(t *testing.T)) bool { return true }

func TestMain(m *testing.M) {
	ctx := context.Background()
	tb := &mainTB{}

	sharedInfra = testutil.SetupInfra(ctx, tb)

	// Create topics.
	createTopics(ctx, sharedInfra.KafkaBrokers, consumer.TopicOrders, consumer.TopicCart, consumer.TopicViews)

	code := m.Run()
	sharedInfra.Teardown()
	os.Exit(code)
}

func createTopics(ctx context.Context, brokers []string, topics ...string) {
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		panic(fmt.Sprintf("dial kafka: %v", err))
	}
	defer conn.Close()

	configs := make([]kafka.TopicConfig, len(topics))
	for i, t := range topics {
		configs[i] = kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     1,
			ReplicationFactor: 1,
		}
	}
	if err := conn.CreateTopics(configs...); err != nil {
		panic(fmt.Sprintf("create topics: %v", err))
	}
}

func getInfra(t *testing.T) *testutil.Infra {
	t.Helper()
	if sharedInfra == nil {
		t.Fatal("sharedInfra is nil — TestMain setup must have failed")
	}
	return sharedInfra
}

// publishEvent writes a JSON event to the given topic.
func publishEvent(t *testing.T, brokers []string, topic string, eventType string, data any, headers ...kafka.Header) {
	t.Helper()

	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	env := struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}{
		Type: eventType,
		Data: json.RawMessage(dataBytes),
	}
	value, _ := json.Marshal(env)

	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer w.Close()

	err = w.WriteMessages(context.Background(), kafka.Message{
		Value:   value,
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("write message to %s: %v", topic, err)
	}
}

func TestConsumer_OrderEvent(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.created", map[string]any{
		"orderID":    "ord-1",
		"userID":     "user-1",
		"totalCents": 5000,
	})

	// Poll until the aggregator records the event.
	deadline := time.After(15 * time.Second)
	for {
		stats := orders.Stats()
		if stats.StatusBreakdown.Created >= 1 {
			if stats.StatusBreakdown.Created != 1 {
				t.Errorf("expected 1 created, got %d", stats.StatusBreakdown.Created)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for order event to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_CartEvent(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicCart, "cart.item_added", map[string]any{
		"productID": "prod-1",
	})

	deadline := time.After(15 * time.Second)
	for {
		stats := carts.Stats()
		if stats.ActiveCarts >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for cart event to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_ProductViewed(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicViews, "product.viewed", map[string]any{
		"productID":   "prod-view-1",
		"productName": "Test Widget",
	})

	deadline := time.After(15 * time.Second)
	for {
		top := trending.TopProducts()
		if len(top) >= 1 {
			if top[0].Name != "Test Widget" {
				t.Errorf("expected name 'Test Widget', got %s", top[0].Name)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for product view to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_TracePropagation(t *testing.T) {
	infra := getInfra(t)

	// Inject a trace context header into the Kafka message.
	traceID := "0af7651916cd43dd8448eb211c80319c"
	spanID := "b7ad6b7169203331"
	traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)

	headers := []kafka.Header{
		{Key: "traceparent", Value: []byte(traceparent)},
	}

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-trace",
		"userID":     "user-trace",
		"totalCents": 1000,
	}, headers...)

	// Verify the event was consumed (trace extraction doesn't panic).
	deadline := time.After(15 * time.Second)
	for {
		stats := orders.Stats()
		if stats.StatusBreakdown.Completed >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for traced order event")
		case <-time.After(200 * time.Millisecond):
		}
	}

	// Note: Full trace ID verification would require an in-memory exporter.
	// This test verifies that trace extraction doesn't panic and the message
	// is consumed correctly with trace headers present.
	_ = tracing.ExtractKafka // confirm import is valid
}
```

- [ ] **Step 2: Run Kafka integration tests**

Run: `cd go/analytics-service && DOCKER_HOST=unix://${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/... -v`
Expected: All 4 tests PASS

- [ ] **Step 3: Commit**

```bash
git add go/analytics-service/internal/integration/consumer_test.go
git commit -m "test(analytics): add Kafka consumer integration tests

Tests order, cart, product view events and trace propagation
through live Kafka via testcontainers.

Closes #99"
```

---

### Task 8: Update Makefile and Final Verification

**Files:**
- Modify: `Makefile:61-64`

- [ ] **Step 1: Add analytics-service to preflight-go-integration**

In `Makefile`, update the `preflight-go-integration` target to also run analytics-service integration tests:

```makefile
preflight-go-integration:
	@echo "\n=== Go: integration tests (order-service) ==="
	cd go/order-service && DOCKER_HOST=unix://$${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...
	@echo "\n=== Go: integration tests (analytics-service) ==="
	cd go/analytics-service && DOCKER_HOST=unix://$${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...
```

- [ ] **Step 2: Run preflight-go to verify lint + unit tests pass**

Run: `make preflight-go`
Expected: All linting and unit tests PASS across all services

- [ ] **Step 3: Run full integration test suite**

Run: `make preflight-go-integration`
Expected: All integration tests PASS for both order-service and analytics-service

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "ci: add analytics-service to preflight-go-integration target"
```

---

## Verification Checklist

After all tasks are complete:

1. `make preflight-go` — lint + unit tests green
2. `make preflight-go-integration` — integration tests green (both services)
3. Manual DLQ test (optional): `docker compose up`, create order, nack message, `curl localhost:8092/admin/dlq/messages`, `curl -X POST localhost:8092/admin/dlq/replay -d '{"index":0}'`
4. `curl localhost:8092/metrics | grep saga_dlq_replayed` — metric exists after replay
