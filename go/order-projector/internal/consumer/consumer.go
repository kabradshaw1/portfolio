package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/event"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/projection"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/kafkaconsumer"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

type Reader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type offsetReader interface {
	SetOffset(offset int64) error
}

type DLQPublisher interface {
	Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error
}

type EventProcessor interface {
	ProcessOrderEvent(ctx context.Context, msg *OrderEventMessage) error
}

type RetryConfig = kafkaconsumer.RetryConfig

type Config struct {
	GroupID      string
	RetryConfig  RetryConfig
	FetchBackoff time.Duration
}

type OrderEventMessage struct {
	Event   *event.OrderEvent
	Message kafka.Message
}

var errMissingOrderIdentity = errors.New("order event missing order identity")

// Consumer reads from the order-events topic, deserializes events,
// and applies all three projections (timeline, summary, stats).
type Consumer struct {
	reader     Reader
	processor  EventProcessor
	dlq        DLQPublisher
	cfg        Config
	connected  atomic.Bool
	processing atomic.Bool
	latestTS   atomic.Value // stores time.Time
}

// New creates a Kafka consumer for the order-events topic.
func New(brokers []string, repo *repository.Repository, cfg Config, dlq DLQPublisher) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  cfg.GroupID,
		Topic:    "ecommerce.order-events",
		MinBytes: 1,
		MaxBytes: 10e6, // 10 MB
	})

	return NewWithDependencies(reader, NewRepositoryProcessor(repo), dlq, cfg)
}

func NewWithDependencies(reader Reader, processor EventProcessor, dlq DLQPublisher, cfg Config) *Consumer {
	if cfg.GroupID == "" {
		cfg.GroupID = "order-projector-group"
	}
	if cfg.RetryConfig.MaxAttempts == 0 {
		cfg.RetryConfig = kafkaconsumer.DefaultRetryConfig()
	}
	if cfg.FetchBackoff == 0 {
		cfg.FetchBackoff = 250 * time.Millisecond
	}
	return &Consumer{reader: reader, processor: processor, dlq: dlq, cfg: cfg}
}

type RepositoryProcessor struct {
	repo     *repository.Repository
	timeline *projection.Timeline
	summary  *projection.Summary
	stats    *projection.Stats
}

func NewRepositoryProcessor(repo *repository.Repository) *RepositoryProcessor {
	return &RepositoryProcessor{
		repo:     repo,
		timeline: projection.NewTimeline(repo),
		summary:  projection.NewSummary(repo),
		stats:    projection.NewStats(repo),
	}
}

func (p *RepositoryProcessor) ProcessOrderEvent(ctx context.Context, msg *OrderEventMessage) error {
	evt := msg.Event
	start := time.Now()
	if err := p.timeline.Apply(ctx, evt); err != nil {
		metrics.ProjectionErrors.WithLabelValues(projection.ProjectionTimeline, evt.Type).Inc()
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionTimeline, evt.Type).Observe(time.Since(start).Seconds())

	start = time.Now()
	if err := p.summary.Apply(ctx, evt); err != nil {
		metrics.ProjectionErrors.WithLabelValues(projection.ProjectionSummary, evt.Type).Inc()
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionSummary, evt.Type).Observe(time.Since(start).Seconds())

	start = time.Now()
	if err := p.stats.Apply(ctx, evt); err != nil {
		metrics.ProjectionErrors.WithLabelValues(projection.ProjectionStats, evt.Type).Inc()
		return err
	}
	metrics.ProjectionDuration.WithLabelValues(projection.ProjectionStats, evt.Type).Observe(time.Since(start).Seconds())
	return nil
}

// Connected returns whether the consumer has successfully fetched at least one message.
func (c *Consumer) Connected() bool {
	return c.connected.Load()
}

// LatestEventTime returns the timestamp of the most recently processed event.
func (c *Consumer) LatestEventTime() time.Time {
	v := c.latestTS.Load()
	if v == nil {
		return time.Time{}
	}
	t, ok := v.(time.Time)
	if !ok {
		return time.Time{}
	}
	return t
}

// IsIdle returns true when the consumer is not actively processing a message.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}

// ResetOffset resets the consumer group offset to the beginning of the topic
// so that a replay re-reads all events.
func (c *Consumer) ResetOffset() error {
	reader, ok := c.reader.(offsetReader)
	if !ok {
		return nil
	}
	return reader.SetOffset(kafka.FirstOffset)
}

// Close shuts down the Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

func (c *Consumer) processOne(ctx context.Context) error {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		metrics.ConsumerErrors.Inc()
		metrics.FetchErrors.WithLabelValues(c.cfg.GroupID, "ecommerce.order-events").Inc()
		return err
	}

	c.connected.Store(true)
	c.processing.Store(true)
	defer c.processing.Store(false)

	msgCtx := tracing.ExtractKafka(ctx, msg.Headers)
	evt, err := Deserialize(msg.Value)
	if err != nil {
		slog.Error("deserialize error", "error", err, "offset", msg.Offset, "partition", msg.Partition)
		metrics.ConsumerErrors.Inc()
		return c.publishDLQAndCommit(ctx, msg, "decode", err)
	}
	if evt.OrderID == "" && len(msg.Key) > 0 {
		evt.OrderID = string(msg.Key)
	}
	if evt.OrderID == "" {
		err := fmt.Errorf("%w: event_id=%s type=%s", errMissingOrderIdentity, evt.ID, evt.Type)
		slog.Error("event validation error", "error", err, "offset", msg.Offset, "partition", msg.Partition)
		metrics.ConsumerErrors.Inc()
		return c.publishDLQAndCommit(ctx, msg, "validate", err)
	}

	err = kafkaconsumer.Retry(msgCtx, c.cfg.RetryConfig, func(ctx context.Context) error {
		return c.processor.ProcessOrderEvent(ctx, &OrderEventMessage{Event: evt, Message: msg})
	})
	if err != nil {
		return err
	}

	c.latestTS.Store(evt.Timestamp)
	metrics.ProjectionLag.Set(time.Since(evt.Timestamp).Seconds())
	metrics.EventsConsumed.WithLabelValues(evt.Type).Inc()
	return c.commit(ctx, msg)
}

func (c *Consumer) commit(ctx context.Context, msg kafka.Message) error {
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		metrics.CommitErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		return err
	}
	return nil
}

func (c *Consumer) publishDLQAndCommit(ctx context.Context, msg kafka.Message, errorClass string, cause error) error {
	if c.dlq == nil {
		return cause
	}
	if dlqErr := c.dlq.Publish(ctx, msg, errorClass, cause); dlqErr != nil {
		metrics.DLQPublishErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		return dlqErr
	}
	metrics.DLQPublished.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
	return c.commit(ctx, msg)
}

// Run reads messages in a loop until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("kafka consumer starting", "topic", "ecommerce.order-events", "group", c.cfg.GroupID)
	for {
		err := c.processOne(ctx)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		slog.Error("kafka consumer iteration failed", "error", err)
		_ = kafkaconsumer.SleepWithContext(ctx, c.cfg.FetchBackoff)
	}
}
