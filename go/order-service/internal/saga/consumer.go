package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/google/uuid"
	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

const consumerPrefetch = 1

type FailureAction string

const (
	FailureDeadLetter     FailureAction = "dead_letter"
	FailureRetryRepublish FailureAction = "retry_republish"
)

type FailureDecision struct {
	Action     FailureAction
	Headers    amqp.Table
	RetryCount int
	ErrorClass string
}

// Consumer listens on the saga.order.events queue and dispatches to the orchestrator.
type Consumer struct {
	orch           *Orchestrator
	retryPublisher ConfirmPublisher
	processing     atomic.Bool
}

// NewConsumer creates a saga event consumer.
func NewConsumer(orch *Orchestrator) *Consumer {
	return &Consumer{orch: orch}
}

func NewConsumerWithPublisher(orch *Orchestrator, retryPublisher ConfirmPublisher) *Consumer {
	return &Consumer{orch: orch, retryPublisher: retryPublisher}
}

// IsIdle returns true when the consumer is not processing a message.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}

// Start begins consuming saga events. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context, ch *amqp.Channel) error {
	if err := ch.Qos(consumerPrefetch, 0, false); err != nil {
		return fmt.Errorf("set saga consumer qos: %w", err)
	}
	if c.retryPublisher == nil {
		retryPublisher, err := rabbitmq.NewPublisher(ch)
		if err != nil {
			return fmt.Errorf("create saga retry publisher: %w", err)
		}
		c.retryPublisher = retryPublisher
	}
	msgs, err := ch.Consume(OrderEvents, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume saga events: %w", err)
	}

	slog.Info("saga event consumer started", "queue", OrderEvents)

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return nil
			}
			c.processing.Store(true)
			evt, handleErr := c.handleMessage(ctx, msg)
			if handleErr != nil {
				decision := DecideFailure(msg.Headers, handleErr, rabbitmq.DefaultMaxRetries)
				slog.ErrorContext(ctx, "saga event handling failed",
					"error", handleErr,
					"action", decision.Action,
					"retryCount", decision.RetryCount,
					"errorClass", decision.ErrorClass,
					"orderID", evt.OrderID,
					"event", evt.Event,
					"routingKey", msg.RoutingKey,
				)
				switch decision.Action {
				case FailureRetryRepublish:
					if err := c.retryPublisher.Publish(ctx, SagaExchange, msg.RoutingKey, retryPublishing(msg, decision.Headers)); err != nil {
						SagaConsumerMessages.WithLabelValues("retry_publish_failed", decision.ErrorClass).Inc()
						_ = msg.Nack(false, true)
						break
					}
					SagaConsumerMessages.WithLabelValues("retried", decision.ErrorClass).Inc()
					_ = msg.Ack(false)
				case FailureDeadLetter:
					SagaConsumerMessages.WithLabelValues("dead_lettered", decision.ErrorClass).Inc()
					SagaDLQTotal.Inc()
					_ = msg.Nack(false, false)
				}
			} else {
				SagaConsumerMessages.WithLabelValues("acked", "none").Inc()
				_ = msg.Ack(false)
			}
			c.processing.Store(false)
		}
	}
}

func (c *Consumer) handleMessage(parentCtx context.Context, msg amqp.Delivery) (Event, error) {
	headers := make(map[string]interface{})
	for k, v := range msg.Headers {
		headers[k] = v
	}
	ctx := tracing.ExtractAMQP(parentCtx, headers)

	var evt Event
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		return evt, rabbitmq.PermanentErrorf("unmarshal saga event: %w", err)
	}
	if err := validateEvent(evt); err != nil {
		return evt, err
	}

	return evt, c.orch.HandleEvent(ctx, evt)
}

func DecideFailure(headers amqp.Table, err error, maxRetries int) FailureDecision {
	retryCount := rabbitmq.RetryCount(headers)
	if maxRetries < 0 {
		maxRetries = 0
	}
	if rabbitmq.IsPermanent(err) || retryCount >= maxRetries {
		return FailureDecision{
			Action:     FailureDeadLetter,
			Headers:    copyHeaders(headers),
			RetryCount: retryCount,
			ErrorClass: errorClass(err),
		}
	}

	retryHeaders := rabbitmq.IncrementRetry(copyHeaders(headers))
	return FailureDecision{
		Action:     FailureRetryRepublish,
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

func validateEvent(evt Event) error {
	if _, err := uuid.Parse(evt.OrderID); err != nil {
		return rabbitmq.PermanentErrorf("parse order ID: %w", err)
	}
	switch evt.Event {
	case EvtItemsReserved, EvtPaymentConfirmed, EvtPaymentFailed, EvtCartCleared, EvtItemsReleased:
		return nil
	default:
		return rabbitmq.PermanentErrorf("unknown saga event: %s", evt.Event)
	}
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
