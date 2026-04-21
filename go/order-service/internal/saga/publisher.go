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

// Publisher wraps RabbitMQ publishing for saga commands.
type Publisher struct {
	ch *amqp.Channel
}

// NewPublisher creates a saga command publisher.
func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

// PublishCommand sends a saga command to the cart-service via RabbitMQ.
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
