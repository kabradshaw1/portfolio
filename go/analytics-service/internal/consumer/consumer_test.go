package consumer

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
)

const (
	testWindowSize    = time.Hour
	testGrace         = 5 * time.Minute
	testSlideInterval = 15 * time.Minute
)

// flushRecorder wraps MockStore and records which flush methods were called.
type flushRecorder struct {
	store.MockStore // embed to satisfy the interface but we override Flush* methods
	mu              sync.Mutex
	revenueCalls    []revenueFlush
	trendingCalls   []trendingFlush
	abandonCalls    []abandonFlush
}

type revenueFlush struct {
	Key        string
	TotalCents int64
	OrderCount int64
}

type trendingFlush struct {
	Key    string
	Scores map[string]float64
}

type abandonFlush struct {
	Key       string
	Started   int64
	Converted int64
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{}
}

func (f *flushRecorder) FlushRevenue(_ context.Context, key string, totalCents, orderCount int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revenueCalls = append(f.revenueCalls, revenueFlush{key, totalCents, orderCount})
	return nil
}

func (f *flushRecorder) FlushTrending(_ context.Context, key string, scores map[string]float64, _ map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make(map[string]float64, len(scores))
	for k, v := range scores {
		cp[k] = v
	}
	f.trendingCalls = append(f.trendingCalls, trendingFlush{key, cp})
	return nil
}

func (f *flushRecorder) FlushAbandonment(_ context.Context, key string, started, converted int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.abandonCalls = append(f.abandonCalls, abandonFlush{key, started, converted})
	return nil
}

func (f *flushRecorder) GetRevenue(_ context.Context, _ int) ([]store.RevenueWindow, error) {
	return nil, nil
}

func (f *flushRecorder) GetTrending(_ context.Context, _ int) (*store.TrendingResult, error) {
	return nil, nil
}

func (f *flushRecorder) GetAbandonment(_ context.Context, _ int) ([]store.AbandonmentWindow, error) {
	return nil, nil
}

func (f *flushRecorder) TrackAbandonmentUser(_ context.Context, _, _, _ string) error {
	return nil
}

func (f *flushRecorder) CountAbandonmentUsers(_ context.Context, _, _ string) (int64, error) {
	return 0, nil
}

// testConsumer creates a Consumer with mock aggregators backed by a flushRecorder.
func testConsumer(t *testing.T) (*Consumer, *window.MockClock, *flushRecorder) {
	t.Helper()

	now := time.Date(2026, 4, 22, 12, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	rec := newFlushRecorder()

	rev := aggregator.NewRevenueAggregator(testWindowSize, testGrace, clock, rec)
	trend := aggregator.NewTrendingAggregator(testWindowSize, testSlideInterval, testGrace, clock, rec)
	aband := aggregator.NewAbandonmentAggregator(testWindowSize, testGrace, clock, rec)

	c := &Consumer{
		revenue:       rev,
		trending:      trend,
		abandonment:   aband,
		flushInterval: 30 * time.Second,
	}

	return c, clock, rec
}

func makeMessage(topic, eventType string, data any, ts time.Time) kafka.Message {
	rawData, _ := json.Marshal(data)
	env := event{
		Type:      eventType,
		Timestamp: ts,
		Data:      json.RawMessage(rawData),
	}
	body, _ := json.Marshal(env)
	return kafka.Message{Topic: topic, Value: body}
}

func TestRoute_OrderCompleted_RoutesToRevenueAndAbandonment(t *testing.T) {
	c, clock, rec := testConsumer(t)
	now := clock.Now()

	msg := makeMessage(TopicOrders, "order.completed", orderData{
		OrderID: "ord-1", UserID: "user-1", TotalCents: 5000,
	}, now)

	c.route(msg)

	// Advance clock past window + grace so Flush produces results.
	clock.Advance(testWindowSize + testGrace + time.Second)

	ctx := context.Background()
	if err := c.revenue.Flush(ctx); err != nil {
		t.Fatalf("revenue flush: %v", err)
	}
	if err := c.abandonment.Flush(ctx); err != nil {
		t.Fatalf("abandonment flush: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.revenueCalls) == 0 {
		t.Fatal("expected revenue flush call")
	}
	if rec.revenueCalls[0].TotalCents != 5000 {
		t.Errorf("expected 5000 cents, got %d", rec.revenueCalls[0].TotalCents)
	}
	if rec.revenueCalls[0].OrderCount != 1 {
		t.Errorf("expected 1 order, got %d", rec.revenueCalls[0].OrderCount)
	}

	if len(rec.abandonCalls) == 0 {
		t.Fatal("expected abandonment flush call")
	}
	if rec.abandonCalls[0].Converted != 1 {
		t.Errorf("expected 1 converted, got %d", rec.abandonCalls[0].Converted)
	}
}

func TestRoute_CartItemAdded_RoutesToTrendingAndAbandonment(t *testing.T) {
	c, clock, rec := testConsumer(t)
	now := clock.Now()

	msg := makeMessage(TopicCart, "cart.item_added", cartData{
		ProductID: "prod-1", UserID: "user-1",
	}, now)

	c.route(msg)

	// Advance past window + grace.
	clock.Advance(testWindowSize + testGrace + time.Second)

	ctx := context.Background()
	if err := c.trending.Flush(ctx); err != nil {
		t.Fatalf("trending flush: %v", err)
	}
	if err := c.abandonment.Flush(ctx); err != nil {
		t.Fatalf("abandonment flush: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.trendingCalls) == 0 {
		t.Fatal("expected trending flush call")
	}
	score, ok := rec.trendingCalls[0].Scores["prod-1"]
	if !ok {
		t.Fatal("expected prod-1 in trending scores")
	}
	if score < 1.0 {
		t.Errorf("expected score >= 1.0 for cart add, got %f", score)
	}

	if len(rec.abandonCalls) == 0 {
		t.Fatal("expected abandonment flush call")
	}
	if rec.abandonCalls[0].Started != 1 {
		t.Errorf("expected 1 cart started, got %d", rec.abandonCalls[0].Started)
	}
}

func TestRoute_CartItemAdded_NoUserID_SkipsAbandonment(t *testing.T) {
	c, clock, rec := testConsumer(t)
	now := clock.Now()

	msg := makeMessage(TopicCart, "cart.item_added", cartData{
		ProductID: "prod-1", UserID: "",
	}, now)

	c.route(msg)

	clock.Advance(testWindowSize + testGrace + time.Second)

	ctx := context.Background()
	_ = c.trending.Flush(ctx)
	_ = c.abandonment.Flush(ctx)

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.trendingCalls) == 0 {
		t.Fatal("expected trending flush call")
	}

	if len(rec.abandonCalls) != 0 {
		t.Errorf("expected no abandonment flush without userID, got %d", len(rec.abandonCalls))
	}
}

func TestRoute_ProductView_RoutesToTrending(t *testing.T) {
	c, clock, rec := testConsumer(t)
	now := clock.Now()

	msg := makeMessage(TopicViews, "product.viewed", viewData{
		ProductID: "prod-1", ProductName: "Widget",
	}, now)

	c.route(msg)

	clock.Advance(testWindowSize + testGrace + time.Second)

	ctx := context.Background()
	if err := c.trending.Flush(ctx); err != nil {
		t.Fatalf("trending flush: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.trendingCalls) == 0 {
		t.Fatal("expected trending flush call")
	}
	if _, ok := rec.trendingCalls[0].Scores["prod-1"]; !ok {
		t.Error("expected prod-1 in trending scores")
	}
}

func TestRoute_Payment_DoesNotPanic(t *testing.T) {
	c, _, _ := testConsumer(t)

	msg := makeMessage(TopicPayments, "payment.succeeded", map[string]string{
		"paymentID": "pay-1",
	}, time.Now())

	c.route(msg)
}

func TestRoute_UnknownTopic_DoesNotPanic(t *testing.T) {
	c, _, _ := testConsumer(t)

	msg := kafka.Message{
		Topic: "unknown.topic",
		Value: []byte(`{"type":"foo","data":{}}`),
	}

	c.route(msg)
}

func TestRoute_MalformedJSON_DoesNotPanic(t *testing.T) {
	c, _, _ := testConsumer(t)

	c.route(kafka.Message{Topic: TopicOrders, Value: []byte("not json")})
}

func TestRoute_MalformedInnerData_DoesNotPanic(t *testing.T) {
	c, _, _ := testConsumer(t)

	body, _ := json.Marshal(event{
		Type: "order.completed",
		Data: json.RawMessage(`not valid json`),
	})
	c.route(kafka.Message{Topic: TopicOrders, Value: body})
}

func TestRoute_ZeroTimestamp_FallsBackToNow(t *testing.T) {
	// Use a mock clock set to "now" so the time.Now() fallback lands in a window
	// the mock clock considers current.
	now := time.Now().UTC()
	clock := window.NewMockClock(now)
	rec := newFlushRecorder()

	rev := aggregator.NewRevenueAggregator(testWindowSize, testGrace, clock, rec)
	trend := aggregator.NewTrendingAggregator(testWindowSize, testSlideInterval, testGrace, clock, rec)
	aband := aggregator.NewAbandonmentAggregator(testWindowSize, testGrace, clock, rec)

	c := &Consumer{
		revenue:       rev,
		trending:      trend,
		abandonment:   aband,
		flushInterval: 30 * time.Second,
	}

	// Send event with zero timestamp — consumer falls back to time.Now().
	rawData, _ := json.Marshal(orderData{OrderID: "ord-1", UserID: "user-1", TotalCents: 2000})
	env := event{
		Type: "order.completed",
		Data: json.RawMessage(rawData),
	}
	body, _ := json.Marshal(env)
	msg := kafka.Message{Topic: TopicOrders, Value: body}

	c.route(msg)

	// Advance well past any window that time.Now() would land in.
	clock.Advance(3*testWindowSize + testGrace)

	ctx := context.Background()
	_ = c.revenue.Flush(ctx)

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.revenueCalls) == 0 {
		t.Fatal("expected revenue flush call for zero-timestamp event")
	}
	if rec.revenueCalls[0].TotalCents != 2000 {
		t.Errorf("expected 2000 cents, got %d", rec.revenueCalls[0].TotalCents)
	}
}

func TestFlushLoop_CallsFlushPeriodically(t *testing.T) {
	c, clock, rec := testConsumer(t)
	now := clock.Now()

	// Add an event so there's something to flush.
	msg := makeMessage(TopicOrders, "order.completed", orderData{
		OrderID: "ord-1", UserID: "user-1", TotalCents: 3000,
	}, now)
	c.route(msg)

	// Advance past window + grace so the window is expired.
	clock.Advance(testWindowSize + testGrace + time.Second)

	// Use a very short flush interval for testing.
	c.flushInterval = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	go c.flushLoop(ctx)

	// Wait enough time for at least one flush tick.
	time.Sleep(150 * time.Millisecond)
	cancel()

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.revenueCalls) == 0 {
		t.Fatal("expected revenue flush call from flush loop")
	}
	if rec.revenueCalls[0].TotalCents != 3000 {
		t.Errorf("expected 3000 cents, got %d", rec.revenueCalls[0].TotalCents)
	}
}
