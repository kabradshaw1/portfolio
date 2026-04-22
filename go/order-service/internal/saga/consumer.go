package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Consumer listens on the saga.order.events queue and dispatches to the orchestrator.
type Consumer struct {
	orch       *Orchestrator
	processing atomic.Bool
}

// NewConsumer creates a saga event consumer.
func NewConsumer(orch *Orchestrator) *Consumer {
	return &Consumer{orch: orch}
}

// IsIdle returns true when the consumer is not processing a message.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}

// Start begins consuming saga events. Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context, ch *amqp.Channel) error {
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
			if err := c.handleMessage(ctx, msg); err != nil {
				// If the circuit breaker is open, nack WITHOUT requeue so the
				// message goes to the DLQ instead of looping and keeping the
				// breaker permanently tripped.
				requeue := !strings.Contains(err.Error(), "CIRCUIT_OPEN") &&
					!strings.Contains(err.Error(), "circuit breaker is open") &&
					!strings.Contains(err.Error(), "temporarily unavailable")
				slog.Error("saga event handling failed", "error", err, "requeue", requeue)
				_ = msg.Nack(false, requeue)
			} else {
				_ = msg.Ack(false)
			}
			c.processing.Store(false)
		}
	}
}

func (c *Consumer) handleMessage(parentCtx context.Context, msg amqp.Delivery) error {
	headers := make(map[string]interface{})
	for k, v := range msg.Headers {
		headers[k] = v
	}
	ctx := tracing.ExtractAMQP(parentCtx, headers)

	var evt Event
	if err := json.Unmarshal(msg.Body, &evt); err != nil {
		return fmt.Errorf("unmarshal saga event: %w", err)
	}

	return c.orch.HandleEvent(ctx, evt)
}
