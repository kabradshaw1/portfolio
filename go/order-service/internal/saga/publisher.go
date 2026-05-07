package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
)

type ConfirmPublisher interface {
	Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error
}

// Publisher wraps RabbitMQ publishing for saga commands.
type Publisher struct {
	pub     ConfirmPublisher
	initErr error
}

// NewPublisher creates a saga command publisher.
func NewPublisher(ch *amqp.Channel) *Publisher {
	pub, err := rabbitmq.NewPublisher(ch)
	return &Publisher{pub: pub, initErr: err}
}

func NewPublisherWithConfirmPublisher(pub ConfirmPublisher) *Publisher {
	return &Publisher{pub: pub}
}

// PublishCommand sends a saga command to the cart-service via RabbitMQ.
func (p *Publisher) PublishCommand(ctx context.Context, cmd Command) error {
	if p.initErr != nil {
		return fmt.Errorf("init saga publisher: %w", p.initErr)
	}
	if p.pub == nil {
		return fmt.Errorf("saga publisher not configured")
	}

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

	return p.pub.Publish(ctx, SagaExchange, CartCommandsKey, amqp.Publishing{
		ContentType:  "application/json",
		Headers:      headers,
		DeliveryMode: amqp.Persistent,
		Body:         body,
	})
}
