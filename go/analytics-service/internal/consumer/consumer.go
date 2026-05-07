package consumer

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/pkg/kafkaconsumer"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Topics consumed by the analytics service.
const (
	TopicOrders   = "ecommerce.orders"
	TopicCart     = "ecommerce.cart"
	TopicViews    = "ecommerce.views"
	TopicPayments = "ecommerce.payments"
)

type DLQPublisher interface {
	Publish(ctx context.Context, msg kafka.Message, errorClass string, cause error) error
}

type Config struct {
	GroupID      string
	RetryConfig  kafkaconsumer.RetryConfig
	FetchBackoff time.Duration
}

// Consumer reads messages from Kafka topics and routes them to windowed aggregators.
type Consumer struct {
	reader        *kafka.Reader
	revenue       *aggregator.RevenueAggregator
	trending      *aggregator.TrendingAggregator
	abandonment   *aggregator.AbandonmentAggregator
	dlq           DLQPublisher
	cfg           Config
	connected     atomic.Bool
	processing    atomic.Bool
	flushInterval time.Duration
}

// New creates a Kafka consumer for the given brokers with windowed aggregators.
func New(
	brokers []string,
	revenue *aggregator.RevenueAggregator,
	trending *aggregator.TrendingAggregator,
	abandonment *aggregator.AbandonmentAggregator,
	flushInterval time.Duration,
	cfg Config,
	dlq DLQPublisher,
) *Consumer {
	if cfg.GroupID == "" {
		cfg.GroupID = "analytics-group"
	}
	if cfg.RetryConfig.MaxAttempts == 0 {
		cfg.RetryConfig = kafkaconsumer.DefaultRetryConfig()
	}
	if cfg.FetchBackoff == 0 {
		cfg.FetchBackoff = 250 * time.Millisecond
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     cfg.GroupID,
		Topic:       "", // set below via GroupTopics
		MinBytes:    1,
		MaxBytes:    10e6, // 10MB
		GroupTopics: []string{TopicOrders, TopicCart, TopicViews, TopicPayments},
	})

	return &Consumer{
		reader:        reader,
		revenue:       revenue,
		trending:      trending,
		abandonment:   abandonment,
		dlq:           dlq,
		cfg:           cfg,
		flushInterval: flushInterval,
	}
}

// Connected returns whether the consumer has successfully connected to Kafka.
func (c *Consumer) Connected() bool {
	return c.connected.Load()
}

// IsIdle returns true when the consumer is not processing a message.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}

// Run reads messages until ctx is cancelled, flushing aggregators periodically.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("kafka consumer starting",
		"topics", []string{TopicOrders, TopicCart, TopicViews, TopicPayments},
		"flushInterval", c.flushInterval,
	)

	// Start the periodic flush ticker in a background goroutine.
	go c.flushLoop(ctx)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Shutting down — perform a final flush before returning.
				c.finalFlush()
				return nil
			}
			slog.Error("kafka fetch error", "error", err)
			metrics.ConsumerErrors.Inc()
			_ = kafkaconsumer.SleepWithContext(ctx, c.cfg.FetchBackoff)
			continue
		}

		c.connected.Store(true)

		// Record consumer lag from reader stats.
		stats := c.reader.Stats()
		metrics.ConsumerLag.Set(float64(stats.Lag))

		// Extract trace context from Kafka headers.
		msgCtx := tracing.ExtractKafka(ctx, msg.Headers)

		c.processing.Store(true)
		shouldCommit := c.route(msgCtx, msg)
		c.processing.Store(false)

		if shouldCommit {
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				slog.Error("kafka commit error", "error", err)
				metrics.CommitErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
			}
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit error", "error", err)
			metrics.CommitErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		}
	}
}

// flushLoop periodically flushes all aggregators until the context is cancelled.
func (c *Consumer) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.flushAll(ctx)
		}
	}
}

// flushAll calls Flush on every aggregator, logging errors but continuing.
func (c *Consumer) flushAll(ctx context.Context) {
	if err := c.revenue.Flush(ctx); err != nil {
		slog.Error("revenue flush error", "error", err)
	} else {
		metrics.LastSuccessfulFlushTimestamp.WithLabelValues("revenue").Set(float64(time.Now().Unix()))
	}
	if err := c.trending.Flush(ctx); err != nil {
		slog.Error("trending flush error", "error", err)
	} else {
		metrics.LastSuccessfulFlushTimestamp.WithLabelValues("trending").Set(float64(time.Now().Unix()))
	}
	if err := c.abandonment.Flush(ctx); err != nil {
		slog.Error("abandonment flush error", "error", err)
	} else {
		metrics.LastSuccessfulFlushTimestamp.WithLabelValues("abandonment").Set(float64(time.Now().Unix()))
	}
}

// finalFlush performs one last flush with a fresh context on shutdown.
func (c *Consumer) finalFlush() {
	slog.Info("performing final flush on shutdown")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:mnd // shutdown timeout
	defer cancel()
	c.flushAll(ctx)
}

// Close shuts down the Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// event is the envelope all Kafka messages use.
type event struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

func (c *Consumer) route(ctx context.Context, msg kafka.Message) bool {
	var env event
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		slog.Error("unmarshal event", "topic", msg.Topic, "error", err)
		metrics.InvalidEvents.WithLabelValues(msg.Topic).Inc()
		if c.dlq != nil {
			if dlqErr := c.dlq.Publish(ctx, msg, "decode", err); dlqErr != nil {
				metrics.DLQPublishErrors.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
				return false
			}
			metrics.DLQPublished.WithLabelValues(c.cfg.GroupID, msg.Topic).Inc()
		}
		return false
	}

	eventTime := env.Timestamp
	if eventTime.IsZero() {
		eventTime = time.Now()
	}

	start := time.Now()

	switch msg.Topic {
	case TopicOrders:
		c.handleOrder(env, eventTime)
	case TopicCart:
		c.handleCart(env, eventTime)
	case TopicViews:
		c.handleView(env, eventTime)
	case TopicPayments:
		c.handlePayment(env)
	default:
		slog.Warn("unknown topic", "topic", msg.Topic)
		return false
	}

	metrics.EventsConsumed.WithLabelValues(msg.Topic).Inc()
	metrics.AggregationLatency.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
	return true
}

type orderData struct {
	OrderID    string `json:"orderID"`
	UserID     string `json:"userID"`
	TotalCents int    `json:"totalCents"`
}

func (c *Consumer) handleOrder(env event, eventTime time.Time) {
	var data orderData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal order data", "error", err)
		return
	}

	switch env.Type {
	case "order.completed":
		if !c.revenue.HandleOrderCompleted(eventTime, int64(data.TotalCents)) {
			slog.Debug("late order event dropped", "orderID", data.OrderID)
		}
		if !c.abandonment.HandleOrderCompleted(eventTime, data.UserID) {
			slog.Debug("late abandonment order event dropped", "orderID", data.OrderID)
		}
	}
}

type cartData struct {
	ProductID string `json:"productID"`
	UserID    string `json:"userID"`
}

func (c *Consumer) handleCart(env event, eventTime time.Time) {
	var data cartData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal cart data", "error", err)
		return
	}

	switch env.Type {
	case "cart.item_added":
		c.trending.HandleCartAdd(eventTime, data.ProductID)
		if data.UserID != "" {
			c.abandonment.HandleCartItemAdded(eventTime, data.UserID)
		}
	}
}

type viewData struct {
	ProductID   string `json:"productID"`
	ProductName string `json:"productName"`
}

func (c *Consumer) handleView(env event, eventTime time.Time) {
	var data viewData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal view data", "error", err)
		return
	}

	c.trending.HandleView(eventTime, data.ProductID, data.ProductName)
}

func (c *Consumer) handlePayment(env event) {
	switch env.Type {
	case "payment.succeeded":
		slog.Info("payment succeeded", "data", env.Data)
	case "payment.failed":
		slog.Info("payment failed", "data", env.Data)
	case "payment.refunded":
		slog.Info("payment refunded", "data", env.Data)
	}
}
