package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Consumer listens on the saga.order.events queue and dispatches to the orchestrator.
type Consumer struct {
	orch *Orchestrator
}

// NewConsumer creates a saga event consumer.
func NewConsumer(orch *Orchestrator) *Consumer {
	return &Consumer{orch: orch}
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
			if err := c.handleMessage(ctx, msg); err != nil {
				slog.Error("saga event handling failed", "error", err)
				_ = msg.Nack(false, true)
			} else {
				_ = msg.Ack(false)
			}
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
