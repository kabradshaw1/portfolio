package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	sagaExchange    = "ecommerce.saga"
	cartCommandsQ   = "saga.cart.commands"
	orderEventsKey  = "saga.order.events"
)

// CartServiceForSaga is the subset of cart service needed by the saga handler.
type CartServiceForSaga interface {
	ReserveItems(ctx context.Context, userID uuid.UUID) error
	ReleaseItems(ctx context.Context, userID uuid.UUID) error
	ClearCart(ctx context.Context, userID uuid.UUID) error
}

type command struct {
	Command   string `json:"command"`
	OrderID   string `json:"order_id"`
	UserID    string `json:"user_id"`
	TraceID   string `json:"trace_id"`
	Timestamp string `json:"timestamp"`
}

type event struct {
	Event     string    `json:"event"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	TraceID   string    `json:"trace_id"`
	Timestamp time.Time `json:"timestamp"`
}

// SagaHandler consumes saga commands from RabbitMQ and publishes reply events.
type SagaHandler struct {
	svc        CartServiceForSaga
	ch         *amqp.Channel
	processing atomic.Bool
}

// NewSagaHandler creates a saga command handler.
func NewSagaHandler(svc CartServiceForSaga, ch *amqp.Channel) *SagaHandler {
	return &SagaHandler{svc: svc, ch: ch}
}

// IsIdle returns true when the handler is not processing a message.
func (h *SagaHandler) IsIdle() bool {
	return !h.processing.Load()
}

// Start begins consuming saga commands. Blocks until ctx is cancelled.
func (h *SagaHandler) Start(ctx context.Context) error {
	msgs, err := h.ch.Consume(cartCommandsQ, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume saga commands: %w", err)
	}

	slog.Info("saga command handler started", "queue", cartCommandsQ)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			h.processing.Store(true)
			if err := h.handleMessage(ctx, msg); err != nil {
				slog.Error("saga command handling failed", "error", err)
				_ = msg.Nack(false, true)
			} else {
				_ = msg.Ack(false)
			}
			h.processing.Store(false)
		}
	}
}

func (h *SagaHandler) handleMessage(parentCtx context.Context, msg amqp.Delivery) error {
	headers := make(map[string]interface{})
	for k, v := range msg.Headers {
		headers[k] = v
	}
	ctx := tracing.ExtractAMQP(parentCtx, headers)

	var cmd command
	if err := json.Unmarshal(msg.Body, &cmd); err != nil {
		return fmt.Errorf("unmarshal saga command: %w", err)
	}

	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return fmt.Errorf("parse user ID: %w", err)
	}

	slog.InfoContext(ctx, "handling saga command",
		"command", cmd.Command,
		"orderID", cmd.OrderID,
		"userID", cmd.UserID,
	)

	var evtName string
	var svcErr error

	switch cmd.Command {
	case "reserve.items":
		svcErr = h.svc.ReserveItems(ctx, userID)
		evtName = "items.reserved"
	case "release.items":
		svcErr = h.svc.ReleaseItems(ctx, userID)
		evtName = "items.released"
	case "clear.cart":
		svcErr = h.svc.ClearCart(ctx, userID)
		evtName = "cart.cleared"
	default:
		return fmt.Errorf("unknown saga command: %s", cmd.Command)
	}

	reply := event{
		Event:     evtName,
		OrderID:   cmd.OrderID,
		UserID:    cmd.UserID,
		Success:   svcErr == nil,
		Timestamp: time.Now().UTC(),
	}
	if svcErr != nil {
		reply.Error = svcErr.Error()
		slog.WarnContext(ctx, "saga command failed",
			"command", cmd.Command, "orderID", cmd.OrderID, "error", svcErr)
	}

	return h.publishReply(ctx, reply)
}

func (h *SagaHandler) publishReply(ctx context.Context, evt event) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal saga event: %w", err)
	}

	replyHeaders := make(amqp.Table)
	tracing.InjectAMQP(ctx, replyHeaders)

	return h.ch.PublishWithContext(ctx, sagaExchange, orderEventsKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Headers:     replyHeaders,
		Body:        body,
	})
}
