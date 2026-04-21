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
)

var sharedInfra *testutil.Infra

type mainTB struct{ testing.TB }

func (*mainTB) Helper()                                    {}
func (*mainTB) Log(args ...any)                            { fmt.Println(args...) }
func (*mainTB) Logf(f string, args ...any)                 { fmt.Printf(f+"\n", args...) }
func (*mainTB) Fatal(args ...any)                          { panic(fmt.Sprint(args...)) }
func (*mainTB) Fatalf(f string, args ...any)               { panic(fmt.Sprintf(f, args...)) }
func (*mainTB) Cleanup(_ func())                           {}
func (*mainTB) Setenv(_ string, _ string)                  {}
func (*mainTB) TempDir() string                            { return os.TempDir() }
func (*mainTB) Parallel()                                  {}
func (*mainTB) Skip(args ...any)                           {}
func (*mainTB) Skipf(f string, args ...any)                {}
func (*mainTB) SkipNow()                                   {}
func (*mainTB) Skipped() bool                              { return false }
func (*mainTB) Failed() bool                               { return false }
func (*mainTB) FailNow()                                   { panic("FailNow") }
func (*mainTB) Fail()                                      {}
func (*mainTB) Name() string                               { return "TestMain" }
func (*mainTB) Error(args ...any)                          { fmt.Println(args...) }
func (*mainTB) Errorf(f string, args ...any)               { fmt.Printf(f+"\n", args...) }
func (*mainTB) Run(_ string, _ func(t *testing.T)) bool    { return true }

func TestMain(m *testing.M) {
	ctx := context.Background()
	tb := &mainTB{}

	sharedInfra = testutil.SetupInfra(ctx, tb)

	createTopics(ctx, sharedInfra.KafkaBrokers, consumer.TopicOrders, consumer.TopicCart, consumer.TopicViews)

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

// publishEvent writes a JSON event to the given topic.
func publishEvent(t *testing.T, brokers []string, topic string, eventType string, data any, headers ...kafka.Header) {
	t.Helper()

	dataBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}

	env := struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}{
		Type: eventType,
		Data: json.RawMessage(dataBytes),
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

func TestConsumer_OrderEvent(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.created", map[string]any{
		"orderID":    "ord-1",
		"userID":     "user-1",
		"totalCents": 5000,
	})

	deadline := time.After(15 * time.Second)
	for {
		stats := orders.Stats()
		if stats.StatusBreakdown.Created >= 1 {
			if stats.StatusBreakdown.Created != 1 {
				t.Errorf("expected 1 created, got %d", stats.StatusBreakdown.Created)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for order event to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_CartEvent(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicCart, "cart.item_added", map[string]any{
		"productID": "prod-1",
	})

	deadline := time.After(15 * time.Second)
	for {
		stats := carts.Stats()
		if stats.ActiveCarts >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for cart event to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_ProductViewed(t *testing.T) {
	infra := getInfra(t)

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicViews, "product.viewed", map[string]any{
		"productID":   "prod-view-1",
		"productName": "Test Widget",
	})

	deadline := time.After(15 * time.Second)
	for {
		top := trending.TopProducts()
		if len(top) >= 1 {
			if top[0].Name != "Test Widget" {
				t.Errorf("expected name 'Test Widget', got %s", top[0].Name)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for product view to be consumed")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func TestConsumer_TracePropagation(t *testing.T) {
	infra := getInfra(t)

	traceID := "0af7651916cd43dd8448eb211c80319c"
	spanID := "b7ad6b7169203331"
	traceparent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)

	headers := []kafka.Header{
		{Key: "traceparent", Value: []byte(traceparent)},
	}

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	cons := consumer.New(infra.KafkaBrokers, orders, trending, carts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = cons.Run(ctx) }()
	defer cons.Close()

	publishEvent(t, infra.KafkaBrokers, consumer.TopicOrders, "order.completed", map[string]any{
		"orderID":    "ord-trace",
		"userID":     "user-trace",
		"totalCents": 1000,
	}, headers...)

	deadline := time.After(15 * time.Second)
	for {
		stats := orders.Stats()
		if stats.StatusBreakdown.Completed >= 1 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for traced order event")
		case <-time.After(200 * time.Millisecond):
		}
	}
}
