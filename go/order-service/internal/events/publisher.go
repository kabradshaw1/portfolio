package events

import (
	"context"
	"log/slog"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

// Publisher emits domain events to the order-events Kafka topic.
type Publisher struct {
	kafkaPub kafka.Producer
}

// NewPublisher creates a Publisher backed by the given Kafka producer.
// A nil producer is safe — Publish becomes a no-op.
func NewPublisher(kafkaPub kafka.Producer) *Publisher {
	return &Publisher{kafkaPub: kafkaPub}
}

// Publish sends a domain event for the given order. It is fire-and-forget:
// failures are logged but never returned, so event publishing cannot break
// the saga flow.
func (p *Publisher) Publish(ctx context.Context, orderID string, eventType string, data any) {
	if p.kafkaPub == nil {
		return
	}
	event := kafka.Event{
		Type:    eventType,
		Version: CurrentVersion,
		Data:    data,
	}
	kafka.SafePublish(ctx, p.kafkaPub, TopicOrderEvents, orderID, event)
	slog.DebugContext(ctx, "published order event", "type", eventType, "orderID", orderID)
}
