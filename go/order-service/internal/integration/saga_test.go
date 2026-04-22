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

// neverAvailableStock is a StockChecker that always returns false.
type neverAvailableStock struct{}

func (s *neverAvailableStock) CheckAvailability(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
	return false, nil
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

// TestSaga_HappyPath verifies the full saga round-trip through live RabbitMQ.
func TestSaga_HappyPath(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()
	testutil.TruncateAll(ctx, t, infra.Pool)

	if err := saga.DeclareTopology(infra.RabbitCh); err != nil {
		t.Fatalf("declare topology: %v", err)
	}

	ids := testutil.SeedProducts(ctx, t, infra.Pool, 2)
	productID1, _ := uuid.Parse(ids[0])
	productID2, _ := uuid.Parse(ids[1])

	breaker := newBreaker()
	orderRepo := repository.NewOrderRepository(infra.Pool, breaker)

	sagaPub := saga.NewPublisher(infra.RabbitCh)
	stock := &alwaysAvailableStock{}
	orch := saga.NewOrchestrator(orderRepo, sagaPub, stock, nil, kafka.NopProducer{})

	cartClient := &testCartClient{
		items: []model.CartItem{
			{ProductID: productID1, Quantity: 2, ProductPrice: 1000},
			{ProductID: productID2, Quantity: 1, ProductPrice: 2000},
		},
	}
	orderSvc := service.NewOrderService(orderRepo, cartClient, orch)

	userID := uuid.New()

	order, err := orderSvc.Checkout(ctx, userID)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	// Consume reserve.items command.
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

	// Simulate items.reserved reply.
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

	// Start saga consumer.
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

	// Consume clear.cart command.
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

	pollUntilSagaStep(t, ctx, orderRepo, order.ID, saga.StepCompleted, 10*time.Second)
	consumerCancel()
}

// TestSaga_FailureToDLQ_Replay verifies DLQ → replay flow.
func TestSaga_FailureToDLQ_Replay(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()

	if err := saga.DeclareTopology(infra.RabbitCh); err != nil {
		t.Fatalf("declare topology: %v", err)
	}

	_, _ = infra.RabbitCh.QueuePurge(saga.CartCommands, false)
	_, _ = infra.RabbitCh.QueuePurge(saga.SagaDLQ, false)

	body := []byte(`{"command":"reserve.items","order_id":"test-order"}`)
	err := infra.RabbitCh.Publish(saga.SagaExchange, saga.CartCommandsKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers:     amqp.Table{"x-retry-count": int32(0)},
		Body:        body,
	})
	if err != nil {
		t.Fatalf("publish to cart commands: %v", err)
	}

	msg := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = msg.Nack(false, false) // nack, no requeue → DLX → DLQ

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

	dlqMsg := dlqMessages[0]
	if dlqMsg.Index != 0 {
		t.Errorf("expected index=0, got %d", dlqMsg.Index)
	}

	_ = ctx
	replayed, err := dlqClient.Replay(0)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.RetryCount != 1 {
		t.Errorf("expected retry count=1, got %d", replayed.RetryCount)
	}

	replayedMsg := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = replayedMsg.Ack(false)

	var cmd saga.Command
	if err := json.Unmarshal(replayedMsg.Body, &cmd); err != nil {
		t.Fatalf("unmarshal replayed message: %v", err)
	}
	if cmd.Command != "reserve.items" {
		t.Errorf("expected command reserve.items, got %s", cmd.Command)
	}

	remaining, err := dlqClient.List(10)
	if err != nil {
		t.Fatalf("list DLQ after replay: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty DLQ after replay, got %d messages", len(remaining))
	}
}

// TestSaga_Compensation verifies stock failure → compensation flow.
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
	orch := saga.NewOrchestrator(orderRepo, sagaPub, stock, nil, kafka.NopProducer{})

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

	reserveCmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	_ = reserveCmd.Ack(false)

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

	releaseCmd := consumeOne(t, infra.RabbitCh, saga.CartCommands, 5*time.Second)
	var releaseSagaCmd saga.Command
	if err := json.Unmarshal(releaseCmd.Body, &releaseSagaCmd); err != nil {
		t.Fatalf("unmarshal release command: %v", err)
	}
	if releaseSagaCmd.Command != saga.CmdReleaseItems {
		t.Errorf("expected command %s, got %s", saga.CmdReleaseItems, releaseSagaCmd.Command)
	}
	_ = releaseCmd.Ack(false)

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

	pollUntilSagaStep(t, ctx, orderRepo, order.ID, saga.StepCompensationComplete, 10*time.Second)
	consumerCancel()
}
