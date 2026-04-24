package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Event is the envelope for all Kafka analytics events.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Version   int       `json:"version,omitempty"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	TraceID   string    `json:"traceID"`
	Data      any       `json:"data"`
}

// Producer publishes events to Kafka topics.
type Producer interface {
	Publish(ctx context.Context, topic string, key string, event Event) error
	Close() error
}

// writerProducer implements Producer using kafka-go Writer.
type writerProducer struct {
	writer *kafkago.Writer
}

const (
	batchSize    = 100
	batchTimeout = 1 * time.Second
)

// NewProducer creates a Kafka producer for the given broker addresses.
func NewProducer(brokers []string) Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Balancer:     &kafkago.LeastBytes{},
		Async:        true,
		BatchSize:    batchSize,
		BatchTimeout: batchTimeout,
		RequiredAcks: kafkago.RequireOne,
	}
	return &writerProducer{writer: w}
}

func (p *writerProducer) Publish(ctx context.Context, topic string, key string, event Event) error {
	event.ID = uuid.New().String()
	event.Source = "order-service"
	event.Timestamp = time.Now().UTC()

	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	var headers []kafkago.Header
	tracing.InjectKafka(ctx, &headers)

	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic:   topic,
		Key:     []byte(key),
		Value:   body,
		Headers: headers,
	})
}

func (p *writerProducer) Close() error {
	return p.writer.Close()
}

// NopProducer is a no-op producer used when Kafka is unavailable.
type NopProducer struct{}

func (NopProducer) Publish(context.Context, string, string, Event) error { return nil }
func (NopProducer) Close() error                                        { return nil }

// SafePublish publishes an event, logging and swallowing errors.
// Use this for fire-and-forget analytics events that must not affect the primary flow.
func SafePublish(ctx context.Context, p Producer, topic, key string, event Event) {
	if err := p.Publish(ctx, topic, key, event); err != nil {
		slog.Warn("kafka publish failed", "topic", topic, "error", err)
	}
}
