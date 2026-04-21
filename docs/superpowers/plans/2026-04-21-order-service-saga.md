# Order Service Saga Orchestrator (Phase 3b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a RabbitMQ-based saga orchestrator to order-service for the checkout flow, with cart reservation semantics and compensation on failure.

**Architecture:** Order-service orchestrates checkout via RabbitMQ commands to cart-service (reserve, release, clear) and gRPC calls to product-service (stock check). Cart-service gains a RabbitMQ consumer and reservation column. Saga state is tracked in the orders table for crash recovery.

**Tech Stack:** Go 1.26, RabbitMQ (amqp091-go), gRPC, protobuf/buf, pgx, Prometheus, OTel, Kubernetes

**Spec:** `docs/superpowers/specs/2026-04-21-order-service-saga-design.md` (Sub-phase B section)

**PREREQUISITE:** Phase 3a (rename) must be complete and deployed. The service is now called `order-service` at `go/order-service/`.

**Base branch:** `qa` (or whatever branch Phase 3a landed on)

**IMPORTANT:** Before starting, read the current state of these key files to understand what exists:
- `go/order-service/internal/service/order.go` — current Checkout method
- `go/order-service/internal/worker/order_processor.go` — current order worker (to be replaced)
- `go/order-service/cmd/server/main.go` — current server setup
- `go/cart-service/internal/service/cart.go` — current cart service (has stubbed Reserve/Release)
- `go/cart-service/internal/grpc/server.go` — current gRPC server (stubs to wire up)
- `go/cart-service/cmd/server/main.go` — current cart server setup

---

### Task 1: Add saga_step column to orders table

**Files:**
- Create: `go/order-service/migrations/NNN_add_saga_step.up.sql`
- Create: `go/order-service/migrations/NNN_add_saga_step.down.sql`

Find the next migration number by listing `go/order-service/migrations/`. It will be after the existing cart_items and orders migrations.

- [ ] **Step 1: Create up migration**

```sql
ALTER TABLE orders ADD COLUMN saga_step TEXT NOT NULL DEFAULT 'CREATED';
```

- [ ] **Step 2: Create down migration**

```sql
ALTER TABLE orders DROP COLUMN IF EXISTS saga_step;
```

- [ ] **Step 3: Commit**

```bash
git add go/order-service/migrations/
git commit -m "feat(order-service): add saga_step column to orders table"
```

---

### Task 2: Add reserved column to cart_items table

**Files:**
- Create: `go/cart-service/migrations/002_add_reserved_column.up.sql`
- Create: `go/cart-service/migrations/002_add_reserved_column.down.sql`

- [ ] **Step 1: Create up migration**

```sql
ALTER TABLE cart_items ADD COLUMN reserved BOOLEAN NOT NULL DEFAULT false;
```

- [ ] **Step 2: Create down migration**

```sql
ALTER TABLE cart_items DROP COLUMN IF EXISTS reserved;
```

- [ ] **Step 3: Commit**

```bash
git add go/cart-service/migrations/
git commit -m "feat(cart-service): add reserved column for saga reservation"
```

---

### Task 3: Add saga model types to order-service

**Files:**
- Create: `go/order-service/internal/saga/types.go`

- [ ] **Step 1: Create saga types**

Define the saga step constants, command/event message types, and the saga state interface:

```go
package saga

import "time"

// Saga step constants — stored in orders.saga_step column.
const (
	StepCreated              = "CREATED"
	StepItemsReserved        = "ITEMS_RESERVED"
	StepStockValidated       = "STOCK_VALIDATED"
	StepCompleted            = "COMPLETED"
	StepCompensating         = "COMPENSATING"
	StepCompensationComplete = "COMPENSATION_COMPLETE"
	StepFailed               = "FAILED"
)

// Command is a saga command published to RabbitMQ.
type Command struct {
	Command   string        `json:"command"`
	OrderID   string        `json:"order_id"`
	UserID    string        `json:"user_id"`
	Items     []CommandItem `json:"items,omitempty"`
	TraceID   string        `json:"trace_id"`
	Timestamp time.Time     `json:"timestamp"`
}

type CommandItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// Event is a saga event reply consumed from RabbitMQ.
type Event struct {
	Event     string    `json:"event"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Commands
const (
	CmdReserveItems = "reserve.items"
	CmdReleaseItems = "release.items"
	CmdClearCart     = "clear.cart"
)

// Events
const (
	EvtItemsReserved = "items.reserved"
	EvtItemsReleased = "items.released"
	EvtCartCleared   = "cart.cleared"
)
```

- [ ] **Step 2: Commit**

```bash
git add go/order-service/internal/saga/
git commit -m "feat(order-service): add saga step constants and message types"
```

---

### Task 4: Implement saga publisher (order-service → RabbitMQ)

**Files:**
- Create: `go/order-service/internal/saga/publisher.go`

- [ ] **Step 1: Create publisher**

The publisher wraps RabbitMQ publishing with trace context injection, retry headers, and structured logging:

```go
package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	SagaExchange    = "ecommerce.saga"
	CartCommandsKey = "saga.cart.commands"
)

type Publisher struct {
	ch *amqp.Channel
}

func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

func (p *Publisher) PublishCommand(ctx context.Context, cmd Command) error {
	cmd.Timestamp = time.Now().UTC()

	body, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal saga command: %w", err)
	}

	headers := make(amqp.Table)
	tracing.InjectAMQP(ctx, headers)
	headers["x-retry-count"] = int32(0)

	slog.InfoContext(ctx, "publishing saga command",
		"command", cmd.Command,
		"orderID", cmd.OrderID,
		"routingKey", CartCommandsKey,
	)

	return p.ch.PublishWithContext(ctx, SagaExchange, CartCommandsKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers:     headers,
		Body:        body,
	})
}
```

- [ ] **Step 2: Commit**

```bash
git add go/order-service/internal/saga/publisher.go
git commit -m "feat(order-service): add saga RabbitMQ command publisher"
```

---

### Task 5: Implement saga orchestrator state machine

**Files:**
- Create: `go/order-service/internal/saga/orchestrator.go`
- Create: `go/order-service/internal/saga/orchestrator_test.go`

- [ ] **Step 1: Write orchestrator tests**

Test the state machine transitions with mocks for the publisher and product client:

```go
package saga_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
)

type mockOrderRepo struct {
	order *model.Order
}

func (m *mockOrderRepo) FindByID(_ context.Context, _ uuid.UUID) (*model.Order, error) {
	return m.order, nil
}

func (m *mockOrderRepo) UpdateSagaStep(_ context.Context, _ uuid.UUID, step string) error {
	m.order.SagaStep = step
	return nil
}

func (m *mockOrderRepo) UpdateStatus(_ context.Context, _ uuid.UUID, status model.OrderStatus) error {
	m.order.Status = status
	return nil
}

type mockPublisher struct {
	commands []saga.Command
}

func (m *mockPublisher) PublishCommand(_ context.Context, cmd saga.Command) error {
	m.commands = append(m.commands, cmd)
	return nil
}

type mockProductClient struct {
	available bool
}

func (m *mockProductClient) CheckAvailability(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
	return m.available, nil
}

func TestOrchestrator_HandleCreated_PublishesReserve(t *testing.T) {
	order := &model.Order{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		SagaStep: saga.StepCreated,
		Items:    []model.OrderItem{{ProductID: uuid.New(), Quantity: 1}},
	}
	repo := &mockOrderRepo{order: order}
	pub := &mockPublisher{}
	orch := saga.NewOrchestrator(repo, pub, nil, nil)

	err := orch.Advance(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pub.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(pub.commands))
	}
	if pub.commands[0].Command != saga.CmdReserveItems {
		t.Errorf("expected reserve.items command, got %s", pub.commands[0].Command)
	}
}

func TestOrchestrator_HandleItemsReserved_ChecksStock(t *testing.T) {
	order := &model.Order{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		SagaStep: saga.StepItemsReserved,
		Items:    []model.OrderItem{{ProductID: uuid.New(), Quantity: 1, PriceAtPurchase: 1000}},
	}
	repo := &mockOrderRepo{order: order}
	pub := &mockPublisher{}
	prodClient := &mockProductClient{available: true}
	orch := saga.NewOrchestrator(repo, pub, prodClient, nil)

	err := orch.Advance(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.SagaStep != saga.StepStockValidated {
		t.Errorf("expected STOCK_VALIDATED, got %s", order.SagaStep)
	}
	// Should publish clear.cart
	if len(pub.commands) != 1 || pub.commands[0].Command != saga.CmdClearCart {
		t.Errorf("expected clear.cart command")
	}
}

func TestOrchestrator_StockUnavailable_Compensates(t *testing.T) {
	order := &model.Order{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		SagaStep: saga.StepItemsReserved,
		Status:   model.OrderStatusPending,
		Items:    []model.OrderItem{{ProductID: uuid.New(), Quantity: 100}},
	}
	repo := &mockOrderRepo{order: order}
	pub := &mockPublisher{}
	prodClient := &mockProductClient{available: false}
	orch := saga.NewOrchestrator(repo, pub, prodClient, nil)

	err := orch.Advance(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Status != model.OrderStatusFailed {
		t.Errorf("expected FAILED status, got %s", order.Status)
	}
	if len(pub.commands) != 1 || pub.commands[0].Command != saga.CmdReleaseItems {
		t.Errorf("expected release.items compensation command")
	}
}
```

- [ ] **Step 2: Implement orchestrator**

Read the current `go/order-service/internal/model/order.go` to understand the Order struct, then add a `SagaStep string` field to it.

Create `go/order-service/internal/saga/orchestrator.go`:

```go
package saga

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
)

type OrderRepository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	UpdateSagaStep(ctx context.Context, orderID uuid.UUID, step string) error
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error
}

type SagaPublisher interface {
	PublishCommand(ctx context.Context, cmd Command) error
}

type StockChecker interface {
	CheckAvailability(ctx context.Context, productID uuid.UUID, quantity int) (bool, error)
}

type KafkaPublisher interface {
	PublishOrderCompleted(ctx context.Context, order *model.Order) error
}

type Orchestrator struct {
	repo    OrderRepository
	pub     SagaPublisher
	stock   StockChecker
	kafkaPub KafkaPublisher
}

func NewOrchestrator(repo OrderRepository, pub SagaPublisher, stock StockChecker, kafkaPub KafkaPublisher) *Orchestrator {
	return &Orchestrator{repo: repo, pub: pub, stock: stock, kafkaPub: kafkaPub}
}

// Advance moves the saga forward from its current step.
func (o *Orchestrator) Advance(ctx context.Context, orderID uuid.UUID) error {
	order, err := o.repo.FindByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find order: %w", err)
	}

	slog.InfoContext(ctx, "advancing saga", "orderID", orderID, "currentStep", order.SagaStep)

	switch order.SagaStep {
	case StepCreated:
		return o.handleCreated(ctx, order)
	case StepItemsReserved:
		return o.handleItemsReserved(ctx, order)
	case StepStockValidated:
		return o.handleStockValidated(ctx, order)
	case StepCompensating:
		// Compensation command already sent, waiting for reply
		return nil
	case StepCompleted, StepCompensationComplete, StepFailed:
		// Terminal states — nothing to do
		return nil
	default:
		return fmt.Errorf("unknown saga step: %s", order.SagaStep)
	}
}

func (o *Orchestrator) handleCreated(ctx context.Context, order *model.Order) error {
	items := make([]CommandItem, len(order.Items))
	for i, item := range order.Items {
		items[i] = CommandItem{
			ProductID: item.ProductID.String(),
			Quantity:  item.Quantity,
		}
	}

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReserveItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
		Items:   items,
	})
}

func (o *Orchestrator) handleItemsReserved(ctx context.Context, order *model.Order) error {
	// Check stock availability via gRPC (sync)
	for _, item := range order.Items {
		available, err := o.stock.CheckAvailability(ctx, item.ProductID, item.Quantity)
		if err != nil {
			return fmt.Errorf("check stock for %s: %w", item.ProductID, err)
		}
		if !available {
			slog.WarnContext(ctx, "stock insufficient, compensating",
				"orderID", order.ID, "productID", item.ProductID)
			return o.compensate(ctx, order)
		}
	}

	// All stock available — advance
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepStockValidated); err != nil {
		return err
	}
	order.SagaStep = StepStockValidated

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdClearCart,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}

func (o *Orchestrator) handleStockValidated(ctx context.Context, order *model.Order) error {
	// Cart cleared — finalize order
	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusCompleted); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompleted); err != nil {
		return err
	}

	slog.InfoContext(ctx, "saga completed", "orderID", order.ID)

	// Publish Kafka analytics event (fire-and-forget)
	if o.kafkaPub != nil {
		if err := o.kafkaPub.PublishOrderCompleted(ctx, order); err != nil {
			slog.WarnContext(ctx, "kafka publish failed", "orderID", order.ID, "error", err)
		}
	}

	return nil
}

func (o *Orchestrator) compensate(ctx context.Context, order *model.Order) error {
	if err := o.repo.UpdateStatus(ctx, order.ID, model.OrderStatusFailed); err != nil {
		return err
	}
	if err := o.repo.UpdateSagaStep(ctx, order.ID, StepCompensating); err != nil {
		return err
	}
	order.Status = model.OrderStatusFailed
	order.SagaStep = StepCompensating

	return o.pub.PublishCommand(ctx, Command{
		Command: CmdReleaseItems,
		OrderID: order.ID.String(),
		UserID:  order.UserID.String(),
	})
}

// HandleEvent processes a saga reply event and advances the saga.
func (o *Orchestrator) HandleEvent(ctx context.Context, evt Event) error {
	orderID, err := uuid.Parse(evt.OrderID)
	if err != nil {
		return fmt.Errorf("parse order ID: %w", err)
	}

	slog.InfoContext(ctx, "handling saga event", "event", evt.Event, "orderID", evt.OrderID)

	switch evt.Event {
	case EvtItemsReserved:
		if err := o.repo.UpdateSagaStep(ctx, orderID, StepItemsReserved); err != nil {
			return err
		}
		return o.Advance(ctx, orderID)

	case EvtCartCleared:
		return o.Advance(ctx, orderID)

	case EvtItemsReleased:
		return o.repo.UpdateSagaStep(ctx, orderID, StepCompensationComplete)

	default:
		return fmt.Errorf("unknown saga event: %s", evt.Event)
	}
}
```

- [ ] **Step 3: Add `SagaStep` field to Order model**

Read `go/order-service/internal/model/order.go` and add `SagaStep string` to the Order struct. Add `json:"sagaStep"` tag.

- [ ] **Step 4: Add `UpdateSagaStep` to order repository**

Read `go/order-service/internal/repository/order.go` and add:

```go
func (r *OrderRepository) UpdateSagaStep(ctx context.Context, orderID uuid.UUID, step string) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			"UPDATE orders SET saga_step = $1, updated_at = NOW() WHERE id = $2",
			step, orderID,
		)
		return err
	})
}
```

Also update `FindByID` and `Create` queries to include `saga_step` in SELECT/INSERT.

- [ ] **Step 5: Run tests**

```bash
cd go/order-service && go test ./internal/saga/... -v -race
```

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/saga/ go/order-service/internal/model/ go/order-service/internal/repository/
git commit -m "feat(order-service): implement saga orchestrator state machine with tests"
```

---

### Task 6: Implement saga event consumer (order-service)

**Files:**
- Create: `go/order-service/internal/saga/consumer.go`

- [ ] **Step 1: Create consumer**

```go
package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	OrderEventsQueue = "saga.order.events"
)

type Consumer struct {
	orch *Orchestrator
}

func NewConsumer(orch *Orchestrator) *Consumer {
	return &Consumer{orch: orch}
}

func (c *Consumer) Start(ctx context.Context, ch *amqp.Channel) error {
	msgs, err := ch.Consume(OrderEventsQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume saga events: %w", err)
	}

	slog.Info("saga event consumer started", "queue", OrderEventsQueue)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			if err := c.handleMessage(ctx, msg); err != nil {
				slog.Error("saga event handling failed", "error", err)
				_ = msg.Nack(false, true) // requeue
			} else {
				_ = msg.Ack(false)
			}
		}
	}
}

func (c *Consumer) handleMessage(parentCtx context.Context, msg amqp.Delivery) error {
	ctx := tracing.ExtractAMQP(parentCtx, msg.Headers)

	var evt Event
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		return fmt.Errorf("unmarshal saga event: %w", err)
	}

	return c.orch.HandleEvent(ctx, evt)
}
```

- [ ] **Step 2: Commit**

```bash
git add go/order-service/internal/saga/consumer.go
git commit -m "feat(order-service): add saga event consumer"
```

---

### Task 7: Implement saga recovery on startup

**Files:**
- Create: `go/order-service/internal/saga/recovery.go`

- [ ] **Step 1: Create recovery**

```go
package saga

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

type IncompleteOrderFinder interface {
	FindIncompleteSagas(ctx context.Context) ([]uuid.UUID, error)
}

func RecoverIncomplete(ctx context.Context, finder IncompleteOrderFinder, orch *Orchestrator) {
	orderIDs, err := finder.FindIncompleteSagas(ctx)
	if err != nil {
		slog.Error("saga recovery: failed to find incomplete orders", "error", err)
		return
	}

	if len(orderIDs) == 0 {
		slog.Info("saga recovery: no incomplete sagas found")
		return
	}

	slog.Info("saga recovery: resuming incomplete sagas", "count", len(orderIDs))
	for _, id := range orderIDs {
		if err := orch.Advance(ctx, id); err != nil {
			slog.Error("saga recovery: failed to resume", "orderID", id, "error", err)
		}
	}
}
```

- [ ] **Step 2: Add `FindIncompleteSagas` to order repository**

```go
func (r *OrderRepository) FindIncompleteSagas(ctx context.Context) ([]uuid.UUID, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]uuid.UUID, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT id FROM orders WHERE saga_step NOT IN ($1, $2, $3)`,
			StepCompleted, StepCompensationComplete, StepFailed,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, nil
	})
}
```

Note: import saga step constants from the saga package, or pass them as strings directly.

- [ ] **Step 3: Commit**

```bash
git add go/order-service/internal/saga/recovery.go go/order-service/internal/repository/
git commit -m "feat(order-service): add saga crash recovery on startup"
```

---

### Task 8: Set up RabbitMQ saga topology

**Files:**
- Create: `go/order-service/internal/saga/topology.go`

- [ ] **Step 1: Create topology setup**

This declares the exchanges and queues on startup:

```go
package saga

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	SagaDLX      = "ecommerce.saga.dlx"
	SagaDLQ      = "ecommerce.saga.dlq"
	CartCommands = "saga.cart.commands"
	OrderEvents  = "saga.order.events"
)

// DeclareTopology sets up the saga exchanges and queues.
func DeclareTopology(ch *amqp.Channel) error {
	// Main saga exchange
	if err := ch.ExchangeDeclare(SagaExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare saga exchange: %w", err)
	}

	// Dead letter exchange and queue
	if err := ch.ExchangeDeclare(SagaDLX, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLX: %w", err)
	}
	if _, err := ch.QueueDeclare(SagaDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare DLQ: %w", err)
	}
	if err := ch.QueueBind(SagaDLQ, "", SagaDLX, false, nil); err != nil {
		return fmt.Errorf("bind DLQ: %w", err)
	}

	// Cart commands queue (consumed by cart-service)
	if _, err := ch.QueueDeclare(CartCommands, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": SagaDLX,
	}); err != nil {
		return fmt.Errorf("declare cart commands queue: %w", err)
	}
	if err := ch.QueueBind(CartCommands, CartCommandsKey, SagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind cart commands: %w", err)
	}

	// Order events queue (consumed by order-service)
	if _, err := ch.QueueDeclare(OrderEvents, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": SagaDLX,
	}); err != nil {
		return fmt.Errorf("declare order events queue: %w", err)
	}
	if err := ch.QueueBind(OrderEvents, "saga.order.events", SagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind order events: %w", err)
	}

	return nil
}
```

- [ ] **Step 2: Commit**

```bash
git add go/order-service/internal/saga/topology.go
git commit -m "feat(order-service): declare RabbitMQ saga topology (exchange, queues, DLQ)"
```

---

### Task 9: Implement cart-service saga command handler

**Files:**
- Modify: `go/cart-service/internal/repository/cart.go` (add Reserve, Release queries)
- Modify: `go/cart-service/internal/service/cart.go` (add ReserveItems, ReleaseItems methods)
- Create: `go/cart-service/internal/worker/saga_handler.go`
- Modify: `go/cart-service/internal/model/cart.go` (add Reserved field)

- [ ] **Step 1: Add Reserved field to CartItem model**

Add `Reserved bool` field to the CartItem struct with `json:"reserved,omitempty"` tag.

- [ ] **Step 2: Add Reserve and Release repository methods**

```go
func (r *CartRepository) Reserve(ctx context.Context, userID uuid.UUID) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			"UPDATE cart_items SET reserved = true WHERE user_id = $1 AND reserved = false",
			userID,
		)
		return err
	})
}

func (r *CartRepository) Release(ctx context.Context, userID uuid.UUID) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			"UPDATE cart_items SET reserved = false WHERE user_id = $1 AND reserved = true",
			userID,
		)
		return err
	})
}
```

- [ ] **Step 3: Update UpdateQuantity and RemoveItem to check reserved**

Change the WHERE clause to include `AND reserved = false`. If 0 rows affected, check if item exists but is reserved, and return a 409-style error.

- [ ] **Step 4: Add ReserveItems and ReleaseItems to cart service**

```go
func (s *CartService) ReserveItems(ctx context.Context, userID uuid.UUID) error {
	return s.repo.Reserve(ctx, userID)
}

func (s *CartService) ReleaseItems(ctx context.Context, userID uuid.UUID) error {
	return s.repo.Release(ctx, userID)
}
```

- [ ] **Step 5: Wire gRPC stubs to real service methods**

Update `go/cart-service/internal/grpc/server.go` — change `ReserveItems` and `ReleaseItems` from returning `codes.Unimplemented` to calling the service methods. Update the `CartServicer` interface to include these methods.

- [ ] **Step 6: Create saga command handler**

Create `go/cart-service/internal/worker/saga_handler.go` that:
- Consumes from `saga.cart.commands` queue
- Parses command messages
- Dispatches to service methods (ReserveItems, ReleaseItems, ClearCart)
- Publishes reply events to `saga.order.events` via `ecommerce.saga` exchange

- [ ] **Step 7: Add RabbitMQ to cart-service config and main.go**

Add `RABBITMQ_URL` to cart-service config. Connect in main.go. Start the saga handler goroutine.

- [ ] **Step 8: Run cart-service tests**

```bash
cd go/cart-service && go test ./... -v -race
```

- [ ] **Step 9: Commit**

```bash
git add go/cart-service/
git commit -m "feat(cart-service): implement saga command handlers (reserve, release, clear)"
```

---

### Task 10: Wire saga into order-service main.go and update Checkout

**Files:**
- Modify: `go/order-service/cmd/server/main.go`
- Modify: `go/order-service/internal/service/order.go`

- [ ] **Step 1: Update Checkout to use saga**

Simplify the `Checkout` method: create order with `saga_step: CREATED`, kick off saga orchestrator (async), return order immediately as PENDING. Remove the direct cart clear and order worker publish.

- [ ] **Step 2: Remove old order worker**

Delete `go/order-service/internal/worker/` directory (order_processor.go and tests). The saga orchestrator replaces this.

- [ ] **Step 3: Wire saga in main.go**

In order-service main.go:
- Declare saga topology via `saga.DeclareTopology(ch)`
- Create saga publisher, stock checker (wraps product gRPC client), orchestrator
- Start saga event consumer goroutine
- Run saga recovery after consumer starts
- Remove old order worker startup

- [ ] **Step 4: Add product gRPC stock checker adapter**

Create a small adapter that wraps the existing product gRPC client's `CheckAvailability` to satisfy the `saga.StockChecker` interface.

- [ ] **Step 5: Run all order-service tests**

```bash
cd go/order-service && go test ./... -v -race
```

- [ ] **Step 6: Commit**

```bash
git add go/order-service/
git commit -m "feat(order-service): wire saga orchestrator into server lifecycle"
```

---

### Task 11: Add saga Prometheus metrics

**Files:**
- Create: `go/order-service/internal/saga/metrics.go`
- Modify: `go/order-service/internal/saga/orchestrator.go` (add metric increments)

- [ ] **Step 1: Create metrics**

```go
package saga

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	SagaStepsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "saga_steps_total",
		Help: "Total saga step transitions.",
	}, []string{"step", "outcome"})

	SagaDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "saga_duration_seconds",
		Help:    "Total saga duration from CREATED to terminal state.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	})

	SagaDLQTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "saga_dlq_messages_total",
		Help: "Messages sent to the saga dead letter queue.",
	})
)
```

- [ ] **Step 2: Add metric increments to orchestrator**

Add `SagaStepsTotal.WithLabelValues(step, "success").Inc()` at each state transition. Track saga start time in the order and observe `SagaDuration` on completion.

- [ ] **Step 3: Commit**

```bash
git add go/order-service/internal/saga/
git commit -m "feat(order-service): add saga Prometheus metrics"
```

---

### Task 12: Define order.proto and add gRPC server

**Files:**
- Create: `go/proto/order/v1/order.proto`
- Create: `go/order-service/internal/grpc/server.go`

- [ ] **Step 1: Create order.proto**

Minimal proto with GetOrder and ListOrders RPCs. Follow the same pattern as product.proto and cart.proto.

- [ ] **Step 2: Run buf generate**

```bash
cd go && buf generate
```

- [ ] **Step 3: Implement gRPC server**

Create `go/order-service/internal/grpc/server.go` implementing the generated interface. Wire into main.go with a gRPC listener on `:9092`.

- [ ] **Step 4: Update K8s deployment**

Add gRPC port 9092 to order-service deployment and service manifests.

- [ ] **Step 5: Commit**

```bash
git add go/proto/order/ go/order-service/ go/k8s/
git commit -m "feat(order-service): add order.proto and gRPC server"
```

---

### Task 13: Update K8s manifests for saga

**Files:**
- Modify: `go/k8s/configmaps/cart-service-config.yml` (add RABBITMQ_URL)
- Modify: `k8s/overlays/qa-go/kustomization.yaml` (add RABBITMQ_URL to cart-service QA patch)

- [ ] **Step 1: Add RABBITMQ_URL to cart-service config**

Add `RABBITMQ_URL: amqp://guest:guest@rabbitmq.java-tasks.svc.cluster.local:5672` to the cart-service ConfigMap.

- [ ] **Step 2: Add QA overlay patch**

Add `RABBITMQ_URL` patch for cart-service in the QA overlay.

- [ ] **Step 3: Commit**

```bash
git add go/k8s/ k8s/overlays/
git commit -m "feat(k8s): add RabbitMQ config for cart-service saga handler"
```

---

### Task 14: Run preflight and push

- [ ] **Step 1: Run full preflight**

```bash
cd go/order-service && go mod tidy && go test ./... -v -race
cd go/cart-service && go mod tidy && go test ./... -v -race
cd frontend && npx tsc --noEmit && npm run lint
```

- [ ] **Step 2: Push and create PR to qa**

```bash
git push -u origin agent/feat-order-service-saga
gh pr create --base qa --title "feat: add saga orchestrator for checkout flow (Phase 3b)" --body "$(cat <<'EOF'
## Summary
- Saga orchestrator in order-service coordinates checkout via RabbitMQ commands
- Cart-service gains reservation semantics (reserved column, saga command handler)
- RabbitMQ topology: saga exchange, command/event queues, DLQ with retry
- Crash recovery: resumes incomplete sagas on startup
- Stock validation via gRPC to product-service
- Compensation flow: releases reserved items on failure
- Saga Prometheus metrics and Jaeger tracing

## Test plan
- [ ] Full checkout produces Jaeger waterfall with saga steps
- [ ] Stock failure triggers compensation (items released)
- [ ] Kill order-service mid-saga, restart, saga resumes
- [ ] Reserved cart items reject update/remove (409)
- [ ] DLQ catches failed messages
- [ ] Saga metrics visible on /metrics
- [ ] All existing smoke tests still pass

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Watch CI and debug failures**
