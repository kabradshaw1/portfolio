package consumer

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/projection"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Consumer reads from the order-events topic, deserializes events,
// and applies all three projections (timeline, summary, stats).
type Consumer struct {
	reader     *kafka.Reader
	timeline   *projection.Timeline
	summary    *projection.Summary
	stats      *projection.Stats
	connected  atomic.Bool
	processing atomic.Bool
	latestTS   atomic.Value // stores time.Time
}

// New creates a Kafka consumer for the order-events topic.
func New(brokers []string, repo *repository.Repository) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  "order-projector-group",
		Topic:   "ecommerce.order-events",
		MinBytes: 1,
		MaxBytes: 10e6, // 10 MB
	})

	return &Consumer{
		reader:   reader,
		timeline: projection.NewTimeline(repo),
		summary:  projection.NewSummary(repo),
		stats:    projection.NewStats(repo),
	}
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
	return c.reader.SetOffset(kafka.FirstOffset)
}

// Close shuts down the Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// Run reads messages in a loop until ctx is cancelled. It deserializes each
// message, applies all three projections, and commits the offset.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("kafka consumer starting",
		"topic", "ecommerce.order-events",
		"group", "order-projector-group",
	)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("kafka fetch error", "error", err)
			metrics.ConsumerErrors.Inc()
			continue
		}

		c.connected.Store(true)
		c.processing.Store(true)

		// Extract trace context from Kafka headers.
		msgCtx := tracing.ExtractKafka(ctx, msg.Headers)

		// Deserialize the raw message into an OrderEvent.
		evt, err := Deserialize(msg.Value)
		if err != nil {
			slog.Error("deserialize error",
				"error", err,
				"offset", msg.Offset,
				"partition", msg.Partition,
			)
			metrics.ConsumerErrors.Inc()
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				slog.Error("kafka commit error after deserialize failure", "error", commitErr)
			}
			c.processing.Store(false)
			continue
		}

		// Apply all three projections. Log errors but continue processing.
		if err := c.timeline.Apply(msgCtx, evt); err != nil {
			slog.Error("timeline projection error", "error", err, "orderID", evt.OrderID)
			metrics.ProjectionErrors.WithLabelValues("timeline", evt.Type).Inc()
		}

		if err := c.summary.Apply(msgCtx, evt); err != nil {
			slog.Error("summary projection error", "error", err, "orderID", evt.OrderID)
			metrics.ProjectionErrors.WithLabelValues("summary", evt.Type).Inc()
		}

		if err := c.stats.Apply(msgCtx, evt); err != nil {
			slog.Error("stats projection error", "error", err, "orderID", evt.OrderID)
			metrics.ProjectionErrors.WithLabelValues("stats", evt.Type).Inc()
		}

		// Track latest event timestamp and update lag metric.
		c.latestTS.Store(evt.Timestamp)
		metrics.ProjectionLag.Set(time.Since(evt.Timestamp).Seconds())

		// Increment consumed counter.
		metrics.EventsConsumed.WithLabelValues(evt.Type).Inc()

		// Commit the offset.
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit error", "error", err)
		}

		c.processing.Store(false)
	}
}
