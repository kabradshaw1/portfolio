package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/pkg/kafkametrics"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const serviceName = "ai-service"

var metricsRecorder kafkametrics.Recorder = kafkametrics.PrometheusRecorder{}

// Event is the envelope for all Kafka analytics events.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
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

// writerProducer implements Producer using a best-effort kafka-go async Writer.
type writerProducer struct {
	writer *kafkago.Writer
}

const (
	batchSize    = 100
	batchTimeout = 1 * time.Second
)

// NewBestEffortProducer creates an async analytics producer. Publish failures
// are observable but must not fail the primary business operation.
func NewBestEffortProducer(brokers []string) Producer {
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Balancer:     &kafkago.LeastBytes{},
		Async:        true,
		BatchSize:    batchSize,
		BatchTimeout: batchTimeout,
		RequiredAcks: kafkago.RequireOne,
	}
	metricsRecorder.SetAsyncMode(serviceName, true)
	return &writerProducer{writer: w}
}

// NewProducer preserves the historical constructor name for existing callers.
func NewProducer(brokers []string) Producer {
	return NewBestEffortProducer(brokers)
}

func (p *writerProducer) Publish(ctx context.Context, topic string, key string, event Event) error {
	event.ID = uuid.New().String()
	event.Source = "ai-service"
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
func (NopProducer) Close() error                                         { return nil }

// SafePublish publishes a best-effort analytics event, logging and swallowing
// errors so analytics visibility never breaks the primary request path.
func SafePublish(ctx context.Context, p Producer, topic, key string, event Event) {
	start := time.Now()
	outcome := "success"
	if err := p.Publish(ctx, topic, key, event); err != nil {
		outcome = "error"
		slog.Warn("kafka publish failed", "topic", topic, "error", err)
	}
	metricsRecorder.ObserveMessage(serviceName, topic, outcome)
	metricsRecorder.ObserveWriteDuration(serviceName, topic, time.Since(start))
}
