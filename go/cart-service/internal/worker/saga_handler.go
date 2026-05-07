package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/metrics"
	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	sagaExchange     = "ecommerce.saga"
	sagaDLX          = "ecommerce.saga.dlx"
	sagaDLQ          = "ecommerce.saga.dlq"
	cartCommandsQ    = "saga.cart.commands"
	orderEventsKey   = "saga.order.events"
	consumerPrefetch = 1
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

type confirmPublisher = rabbitmq.PublishingClient

// SagaHandler consumes saga commands from RabbitMQ and publishes reply events.
type SagaHandler struct {
	svc        CartServiceForSaga
	ch         *amqp.Channel
	publisher  confirmPublisher
	processing atomic.Bool
}

// NewSagaHandler creates a saga command handler.
func NewSagaHandler(svc CartServiceForSaga, ch *amqp.Channel) *SagaHandler {
	return &SagaHandler{svc: svc, ch: ch}
}

func NewSagaHandlerWithPublisher(svc CartServiceForSaga, ch *amqp.Channel, publisher rabbitmq.PublishingClient) *SagaHandler {
	return &SagaHandler{svc: svc, ch: ch, publisher: publisher}
}

func newSagaHandlerWithPublisher(svc CartServiceForSaga, publisher confirmPublisher) *SagaHandler {
	return NewSagaHandlerWithPublisher(svc, nil, publisher)
}

func DeclareSagaTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(sagaExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare saga exchange: %w", err)
	}
	if err := ch.ExchangeDeclare(sagaDLX, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare saga dlx: %w", err)
	}
	if _, err := ch.QueueDeclare(sagaDLQ, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare saga dlq: %w", err)
	}
	if err := ch.QueueBind(sagaDLQ, "", sagaDLX, false, nil); err != nil {
		return fmt.Errorf("bind saga dlq: %w", err)
	}
	if _, err := ch.QueueDeclare(cartCommandsQ, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": sagaDLX,
	}); err != nil {
		return fmt.Errorf("declare cart commands queue: %w", err)
	}
	if err := ch.QueueBind(cartCommandsQ, cartCommandsQ, sagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind cart commands queue: %w", err)
	}
	if _, err := ch.QueueDeclare(orderEventsKey, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange": sagaDLX,
	}); err != nil {
		return fmt.Errorf("declare order events queue: %w", err)
	}
	if err := ch.QueueBind(orderEventsKey, orderEventsKey, sagaExchange, false, nil); err != nil {
		return fmt.Errorf("bind order events queue: %w", err)
	}
	return nil
}

// IsIdle returns true when the handler is not processing a message.
func (h *SagaHandler) IsIdle() bool {
	return !h.processing.Load()
}

// Start begins consuming saga commands. Blocks until ctx is cancelled.
func (h *SagaHandler) Start(ctx context.Context) error {
	if err := h.ch.Qos(consumerPrefetch, 0, false); err != nil {
		return fmt.Errorf("set saga command qos: %w", err)
	}
	if h.publisher == nil {
		publisher, err := rabbitmq.NewPublisher(h.ch)
		if err != nil {
			return fmt.Errorf("create saga command publisher: %w", err)
		}
		h.publisher = publisher
	}
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
				decision := decideFailure(msg.Headers, err, rabbitmq.DefaultMaxRetries)
				slog.Error("saga command handling failed",
					"error", err,
					"action", decision.Action,
					"retryCount", decision.RetryCount,
					"errorClass", decision.ErrorClass,
					"routingKey", msg.RoutingKey,
				)
				switch decision.Action {
				case failureRetryRepublish:
					if err := h.publisher.Publish(ctx, sagaExchange, retryRoutingKey(msg), retryPublishing(msg, decision.Headers)); err != nil {
						metrics.SagaMessages.WithLabelValues("retry_publish_failed", decision.ErrorClass).Inc()
						_ = msg.Nack(false, true)
						break
					}
					metrics.SagaMessages.WithLabelValues("retried", decision.ErrorClass).Inc()
					_ = msg.Ack(false)
				case failureDeadLetter:
					metrics.SagaMessages.WithLabelValues("dead_lettered", decision.ErrorClass).Inc()
					_ = msg.Nack(false, false)
				}
			} else {
				metrics.SagaMessages.WithLabelValues("acked", "none").Inc()
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
		return rabbitmq.PermanentErrorf("unmarshal saga command: %w", err)
	}

	if _, err := uuid.Parse(cmd.OrderID); err != nil {
		return rabbitmq.PermanentErrorf("parse order ID: %w", err)
	}

	userID, err := uuid.Parse(cmd.UserID)
	if err != nil {
		return rabbitmq.PermanentErrorf("parse user ID: %w", err)
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
		return rabbitmq.PermanentErrorf("unknown saga command: %s", cmd.Command)
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
	if h.publisher == nil {
		return fmt.Errorf("saga publisher not configured")
	}

	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal saga event: %w", err)
	}

	replyHeaders := make(amqp.Table)
	tracing.InjectAMQP(ctx, replyHeaders)

	return h.publisher.Publish(ctx, sagaExchange, orderEventsKey, amqp.Publishing{
		ContentType:  "application/json",
		Headers:      replyHeaders,
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}

// HandleMessageForTest exposes message handling to package-external tests.
func (h *SagaHandler) HandleMessageForTest(ctx context.Context, msg amqp.Delivery) error {
	return h.handleMessage(ctx, msg)
}

type failureAction string

const (
	failureDeadLetter     failureAction = "dead_letter"
	failureRetryRepublish failureAction = "retry_republish"
)

type failureDecision struct {
	Action     failureAction
	Headers    amqp.Table
	RetryCount int
	ErrorClass string
}

func decideFailure(headers amqp.Table, err error, maxRetries int) failureDecision {
	retryCount := rabbitmq.RetryCount(headers)
	if rabbitmq.IsPermanent(err) || retryCount >= maxRetries {
		return failureDecision{
			Action:     failureDeadLetter,
			Headers:    copyHeaders(headers),
			RetryCount: retryCount,
			ErrorClass: errorClass(err),
		}
	}

	retryHeaders := rabbitmq.IncrementRetry(copyHeaders(headers))
	return failureDecision{
		Action:     failureRetryRepublish,
		Headers:    retryHeaders,
		RetryCount: rabbitmq.RetryCount(retryHeaders),
		ErrorClass: errorClass(err),
	}
}

func retryPublishing(msg amqp.Delivery, headers amqp.Table) amqp.Publishing {
	return amqp.Publishing{
		Headers:         headers,
		ContentType:     msg.ContentType,
		ContentEncoding: msg.ContentEncoding,
		DeliveryMode:    amqp.Persistent,
		Priority:        msg.Priority,
		CorrelationId:   msg.CorrelationId,
		ReplyTo:         msg.ReplyTo,
		Expiration:      msg.Expiration,
		MessageId:       msg.MessageId,
		Timestamp:       msg.Timestamp,
		Type:            msg.Type,
		UserId:          msg.UserId,
		AppId:           msg.AppId,
		Body:            msg.Body,
	}
}

func retryRoutingKey(msg amqp.Delivery) string {
	if msg.RoutingKey != "" {
		return msg.RoutingKey
	}
	return cartCommandsQ
}

func copyHeaders(headers amqp.Table) amqp.Table {
	copied := amqp.Table{}
	for key, value := range headers {
		copied[key] = value
	}
	return copied
}

func errorClass(err error) string {
	if rabbitmq.IsPermanent(err) {
		return "permanent"
	}
	return "retryable"
}
