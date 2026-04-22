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
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Topics consumed by the analytics service.
const (
	TopicOrders   = "ecommerce.orders"
	TopicCart     = "ecommerce.cart"
	TopicViews    = "ecommerce.views"
	TopicPayments = "ecommerce.payments"
)

// Consumer reads messages from Kafka topics and routes them to aggregators.
type Consumer struct {
	reader    *kafka.Reader
	orders    *aggregator.OrderAggregator
	trending  *aggregator.TrendingAggregator
	carts     *aggregator.CartAggregator
	connected  atomic.Bool
	processing atomic.Bool
}

// New creates a Kafka consumer for the given brokers.
func New(brokers []string, orders *aggregator.OrderAggregator, trending *aggregator.TrendingAggregator, carts *aggregator.CartAggregator) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		GroupID:  "analytics-group",
		Topic:    "", // set below via GroupTopics
		MinBytes: 1,
		MaxBytes: 10e6, // 10MB
		GroupTopics: []string{TopicOrders, TopicCart, TopicViews, TopicPayments},
	})

	return &Consumer{
		reader:   reader,
		orders:   orders,
		trending: trending,
		carts:    carts,
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

// Run reads messages until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	slog.Info("kafka consumer starting", "topics", []string{TopicOrders, TopicCart, TopicViews, TopicPayments})

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // shutting down
			}
			slog.Error("kafka fetch error", "error", err)
			metrics.ConsumerErrors.Inc()
			continue
		}

		c.connected.Store(true)

		// Record consumer lag from reader stats.
		stats := c.reader.Stats()
		metrics.ConsumerLag.Set(float64(stats.Lag))

		// Extract trace context from Kafka headers.
		msgCtx := tracing.ExtractKafka(ctx, msg.Headers)
		_ = msgCtx // available for span creation if tracing is enabled

		c.processing.Store(true)
		c.route(msg)
		c.processing.Store(false)

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit error", "error", err)
		}
	}
}

// Close shuts down the Kafka reader.
func (c *Consumer) Close() error {
	return c.reader.Close()
}

// event is the envelope all Kafka messages use.
type event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func (c *Consumer) route(msg kafka.Message) {
	var env event
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		slog.Error("unmarshal event", "topic", msg.Topic, "error", err)
		return
	}

	start := time.Now()

	switch msg.Topic {
	case TopicOrders:
		c.handleOrder(env)
	case TopicCart:
		c.handleCart(env)
	case TopicViews:
		c.handleView(env)
	case TopicPayments:
		c.handlePayment(env)
	default:
		slog.Warn("unknown topic", "topic", msg.Topic)
		return
	}

	metrics.EventsConsumed.WithLabelValues(msg.Topic).Inc()
	metrics.AggregationLatency.WithLabelValues(msg.Topic).Observe(time.Since(start).Seconds())
}

type orderData struct {
	OrderID    string `json:"orderID"`
	UserID     string `json:"userID"`
	TotalCents int    `json:"totalCents"`
}

func (c *Consumer) handleOrder(env event) {
	var data orderData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal order data", "error", err)
		return
	}

	switch env.Type {
	case "order.created":
		c.orders.RecordCreated(data.TotalCents)
	case "order.completed":
		c.orders.RecordCompleted(data.TotalCents)
	case "order.failed":
		c.orders.RecordFailed()
	}
}

type cartData struct {
	ProductID string `json:"productID"`
}

func (c *Consumer) handleCart(env event) {
	var data cartData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal cart data", "error", err)
		return
	}

	switch env.Type {
	case "cart.item_added":
		c.carts.RecordItemAdded(data.ProductID)
		c.trending.RecordPurchase(data.ProductID, "")
	case "cart.item_removed":
		c.carts.RecordItemRemoved()
	}
}

type viewData struct {
	ProductID   string `json:"productID"`
	ProductName string `json:"productName"`
}

func (c *Consumer) handleView(env event) {
	var data viewData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		slog.Error("unmarshal view data", "error", err)
		return
	}

	c.trending.RecordView(data.ProductID, data.ProductName)
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
