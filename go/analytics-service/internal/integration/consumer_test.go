//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
)

var sharedInfra *testutil.Infra

type mainTB struct{ testing.TB }

func (*mainTB) Helper()                                 {}
func (*mainTB) Log(args ...any)                         { fmt.Println(args...) }
func (*mainTB) Logf(f string, args ...any)              { fmt.Printf(f+"\n", args...) }
func (*mainTB) Fatal(args ...any)                       { panic(fmt.Sprint(args...)) }
func (*mainTB) Fatalf(f string, args ...any)            { panic(fmt.Sprintf(f, args...)) }
func (*mainTB) Cleanup(_ func())                        {}
func (*mainTB) Setenv(_ string, _ string)               {}
func (*mainTB) TempDir() string                         { return os.TempDir() }
func (*mainTB) Parallel()                               {}
func (*mainTB) Skip(args ...any)                        {}
func (*mainTB) Skipf(f string, args ...any)             {}
func (*mainTB) SkipNow()                                {}
func (*mainTB) Skipped() bool                           { return false }
func (*mainTB) Failed() bool                            { return false }
func (*mainTB) FailNow()                                { panic("FailNow") }
func (*mainTB) Fail()                                   {}
func (*mainTB) Name() string                            { return "TestMain" }
func (*mainTB) Error(args ...any)                       { fmt.Println(args...) }
func (*mainTB) Errorf(f string, args ...any)            { fmt.Printf(f+"\n", args...) }
func (*mainTB) Run(_ string, _ func(t *testing.T)) bool { return true }

func TestMain(m *testing.M) {
	ctx := context.Background()
	tb := &mainTB{}

	sharedInfra = testutil.SetupInfra(ctx, tb)

	createTopics(ctx, sharedInfra.KafkaBrokers,
		consumer.TopicOrders, consumer.TopicCart, consumer.TopicViews, consumer.TopicPayments)

	code := m.Run()
	sharedInfra.Teardown()
	os.Exit(code)
}

func createTopics(ctx context.Context, brokers []string, topics ...string) {
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		panic(fmt.Sprintf("dial kafka: %v", err))
	}
	defer conn.Close()

	configs := make([]kafka.TopicConfig, len(topics))
	for i, t := range topics {
		configs[i] = kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     1,
			ReplicationFactor: 1,
		}
	}
	if err := conn.CreateTopics(configs...); err != nil {
		panic(fmt.Sprintf("create topics: %v", err))
	}
}

func getInfra(t *testing.T) *testutil.Infra {
	t.Helper()
	if sharedInfra == nil {
		t.Fatal("sharedInfra is nil — TestMain setup must have failed")
	}
	return sharedInfra
}

// publishEvent writes a JSON event with a timestamp envelope to the given topic.
func publishEvent(t *testing.T, brokers []string, topic, eventType string, data any, headers ...kafka.Header) {
	t.Helper()

	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	env := struct {
		Type      string          `json:"type"`
		Timestamp time.Time       `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      json.RawMessage(dataBytes),
	}
	value, _ := json.Marshal(env)

	w := &kafka.Writer{
		Addr:     kafka.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	defer w.Close()

	err = w.WriteMessages(context.Background(), kafka.Message{
		Value:   value,
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("write message to %s: %v", topic, err)
	}
}

// testWindowSize is a short window for integration tests so they complete quickly.
const testWindowSize = 2 * time.Second

// testFlushInterval is the flush ticker interval for integration tests.
const testFlushInterval = 500 * time.Millisecond

// testSlideInterval is the slide interval for the trending sliding window.
const testSlideInterval = 1 * time.Second

// pollTimeout is the maximum time to wait for data to appear in the store.
const pollTimeout = 15 * time.Second

// pollInterval is how often we check the store for expected data.
const pollInterval = 200 * time.Millisecond

// setupConsumer creates aggregators with a MockStore, starts the consumer, and
// returns the store for assertions and a cleanup function.
func setupConsumer(t *testing.T, brokers []string) (*store.MockStore, func()) {
	t.Helper()

	mockStore := store.NewMockStore()
	clock := window.RealClock{}

	revenue := aggregator.NewRevenueAggregator(testWindowSize, 0, clock, mockStore)
	trending := aggregator.NewTrendingAggregator(testWindowSize, testSlideInterval, 0, clock, mockStore)
	abandonment := aggregator.NewAbandonmentAggregator(testWindowSize, 0, clock, mockStore)

	cons := consumer.New(brokers, revenue, trending, abandonment, testFlushInterval)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = cons.Run(ctx) }()

	cleanup := func() {
		cancel()
		_ = cons.Close()
	}

	return mockStore, cleanup
}

// pollUntil polls fn every pollInterval until it returns true or pollTimeout is reached.
func pollUntil(t *testing.T, desc string, fn func() bool) {
	t.Helper()

	deadline := time.After(pollTimeout)
	for {
		if fn() {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for: %s", desc)
		case <-time.After(pollInterval):
		}
	}
}

func TestRevenue_EndToEnd(t *testing.T) {
	infra := getInfra(t)
	mockStore, cleanup := setupConsumer(t, infra.KafkaBrokers)
	defer cleanup()

	// Publish two order.completed events.
	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-rev-1",
		"userID":     "user-1",
		"totalCents": 5000,
	})
	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-rev-2",
		"userID":     "user-2",
		"totalCents": 3000,
	})

	// Wait for the window to close and flush to the store.
	pollUntil(t, "revenue data in store", func() bool {
		return mockStore.RevenueLen() > 0
	})

	totalCents := mockStore.TotalRevenueCents()
	if totalCents != 8000 {
		t.Errorf("expected total revenue 8000 cents, got %d", totalCents)
	}

	orderCount := mockStore.TotalOrderCount()
	if orderCount != 2 {
		t.Errorf("expected 2 orders, got %d", orderCount)
	}
}

func TestTrending_EndToEnd(t *testing.T) {
	infra := getInfra(t)
	mockStore, cleanup := setupConsumer(t, infra.KafkaBrokers)
	defer cleanup()

	// Publish view and cart-add events for different products.
	publishEvent(t, infra.KafkaBrokers, consumer.TopicViews, "product.viewed", map[string]any{
		"productID":   "prod-trend-1",
		"productName": "Widget A",
	})
	publishEvent(t, infra.KafkaBrokers, consumer.TopicViews, "product.viewed", map[string]any{
		"productID":   "prod-trend-1",
		"productName": "Widget A",
	})
	publishEvent(t, infra.KafkaBrokers, consumer.TopicCart, "cart.item_added", map[string]any{
		"productID": "prod-trend-2",
		"userID":    "user-trend-1",
	})

	// Wait for the sliding window to emit and flush trending data.
	pollUntil(t, "trending data in store", func() bool {
		return mockStore.TrendingLen() > 0
	})

	scores := mockStore.TrendingScores()
	// prod-trend-1: 2 views * weight 1.0 = 2.0
	if s := scores["prod-trend-1"]; s < 2.0 {
		t.Errorf("expected prod-trend-1 score >= 2.0, got %f", s)
	}
	// prod-trend-2: 1 cart add * weight 3.0 = 3.0
	if s := scores["prod-trend-2"]; s < 3.0 {
		t.Errorf("expected prod-trend-2 score >= 3.0, got %f", s)
	}
}

func TestAbandonment_EndToEnd(t *testing.T) {
	infra := getInfra(t)
	mockStore, cleanup := setupConsumer(t, infra.KafkaBrokers)
	defer cleanup()

	// Two users add to cart; only one completes an order.
	publishEvent(t, infra.KafkaBrokers, consumer.TopicCart, "cart.item_added", map[string]any{
		"productID": "prod-abn-1",
		"userID":    "user-abn-1",
	})
	publishEvent(t, infra.KafkaBrokers, consumer.TopicCart, "cart.item_added", map[string]any{
		"productID": "prod-abn-2",
		"userID":    "user-abn-2",
	})
	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-abn-1",
		"userID":     "user-abn-1",
		"totalCents": 2000,
	})

	// Wait for the abandonment window to close and flush.
	pollUntil(t, "abandonment data in store", func() bool {
		return mockStore.AbandonmentLen() > 0
	})

	started := mockStore.TotalCartsStarted()
	if started < 2 {
		t.Errorf("expected at least 2 carts started, got %d", started)
	}

	converted := mockStore.TotalCartsConverted()
	if converted < 1 {
		t.Errorf("expected at least 1 cart converted, got %d", converted)
	}
}

func TestTracePropagation(t *testing.T) {
	infra := getInfra(t)
	mockStore, cleanup := setupConsumer(t, infra.KafkaBrokers)
	defer cleanup()

	traceID := "0af7651916cd43dd8448eb211c80319c"
	spanID := "b7ad6b7169203331"
	traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)

	headers := []kafka.Header{
		{Key: "traceparent", Value: []byte(traceparent)},
	}

	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-trace",
		"userID":     "user-trace",
		"totalCents": 1000,
	}, headers...)

	// The event should be consumed and flushed to the store without errors.
	// This verifies that trace extraction from Kafka headers doesn't crash.
	pollUntil(t, "traced order in store", func() bool {
		return mockStore.RevenueLen() > 0
	})
}
