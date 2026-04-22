package store

import (
	"context"
	"testing"
	"time"
)

func TestFlushAndGetRevenueRoundTrip(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	now := time.Now().UTC()
	key := now.Format(windowKeyLayout)

	if err := s.FlushRevenue(ctx, key, 5000, 2); err != nil {
		t.Fatalf("FlushRevenue: %v", err)
	}

	windows, err := s.GetRevenue(ctx, 2)
	if err != nil {
		t.Fatalf("GetRevenue: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.TotalCents != 5000 {
		t.Errorf("TotalCents = %d, want 5000", w.TotalCents)
	}
	if w.OrderCount != 2 {
		t.Errorf("OrderCount = %d, want 2", w.OrderCount)
	}
	if w.AvgCents != 2500 {
		t.Errorf("AvgCents = %d, want 2500", w.AvgCents)
	}
}

func TestFlushRevenueAccumulates(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)

	_ = s.FlushRevenue(ctx, key, 1000, 1)
	_ = s.FlushRevenue(ctx, key, 3000, 2)

	windows, _ := s.GetRevenue(ctx, 2)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if windows[0].TotalCents != 4000 {
		t.Errorf("TotalCents = %d, want 4000", windows[0].TotalCents)
	}
	if windows[0].OrderCount != 3 {
		t.Errorf("OrderCount = %d, want 3", windows[0].OrderCount)
	}
	if windows[0].AvgCents != 1333 {
		t.Errorf("AvgCents = %d, want 1333", windows[0].AvgCents)
	}
}

func TestGetRevenueFiltersToRequestedHours(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	now := time.Now().UTC()
	recentKey := now.Format(windowKeyLayout)
	oldKey := now.Add(-48 * time.Hour).Format(windowKeyLayout)

	_ = s.FlushRevenue(ctx, recentKey, 1000, 1)
	_ = s.FlushRevenue(ctx, oldKey, 2000, 1)

	windows, err := s.GetRevenue(ctx, 2)
	if err != nil {
		t.Fatalf("GetRevenue: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window (filtered), got %d", len(windows))
	}
	if windows[0].TotalCents != 1000 {
		t.Errorf("TotalCents = %d, want 1000 (the recent one)", windows[0].TotalCents)
	}
}

func TestFlushAndGetTrendingRoundTrip(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)
	scores := map[string]float64{
		"product-a": 10.0,
		"product-b": 25.0,
		"product-c": 5.0,
	}

	if err := s.FlushTrending(ctx, key, scores, nil); err != nil {
		t.Fatalf("FlushTrending: %v", err)
	}

	result, err := s.GetTrending(ctx, 10)
	if err != nil {
		t.Fatalf("GetTrending: %v", err)
	}
	if result == nil {
		t.Fatal("GetTrending returned nil")
	}
	if len(result.Products) != 3 {
		t.Fatalf("expected 3 products, got %d", len(result.Products))
	}
}

func TestGetTrendingReturnsTopByScore(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)
	scores := map[string]float64{
		"product-a": 10.0,
		"product-b": 25.0,
		"product-c": 5.0,
	}
	_ = s.FlushTrending(ctx, key, scores, nil)

	result, _ := s.GetTrending(ctx, 2)
	if result == nil {
		t.Fatal("GetTrending returned nil")
	}
	if len(result.Products) != 2 {
		t.Fatalf("expected 2 products (limited), got %d", len(result.Products))
	}
	if result.Products[0].ProductID != "product-b" {
		t.Errorf("expected product-b first (score 25), got %s", result.Products[0].ProductID)
	}
	if result.Products[1].ProductID != "product-a" {
		t.Errorf("expected product-a second (score 10), got %s", result.Products[1].ProductID)
	}
}

func TestFlushAndGetTrendingNamesRoundTrip(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)
	scores := map[string]float64{
		"product-a": 10.0,
		"product-b": 25.0,
	}
	names := map[string]string{
		"product-a": "Alpha Widget",
		"product-b": "Beta Widget",
	}

	if err := s.FlushTrending(ctx, key, scores, names); err != nil {
		t.Fatalf("FlushTrending: %v", err)
	}

	result, err := s.GetTrending(ctx, 10)
	if err != nil {
		t.Fatalf("GetTrending: %v", err)
	}
	if result == nil {
		t.Fatal("GetTrending returned nil")
	}

	nameMap := make(map[string]string)
	for _, p := range result.Products {
		nameMap[p.ProductID] = p.ProductName
	}
	if nameMap["product-a"] != "Alpha Widget" {
		t.Errorf("product-a name = %q, want %q", nameMap["product-a"], "Alpha Widget")
	}
	if nameMap["product-b"] != "Beta Widget" {
		t.Errorf("product-b name = %q, want %q", nameMap["product-b"], "Beta Widget")
	}
}

func TestGetTrendingReturnsNilWhenEmpty(t *testing.T) {
	t.Parallel()
	s := NewMockStore()

	result, err := s.GetTrending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetTrending: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty store, got %+v", result)
	}
}

func TestFlushAndGetAbandonmentRoundTrip(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)

	if err := s.FlushAbandonment(ctx, key, 100, 60); err != nil {
		t.Fatalf("FlushAbandonment: %v", err)
	}

	windows, err := s.GetAbandonment(ctx, 2)
	if err != nil {
		t.Fatalf("GetAbandonment: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.CartsStarted != 100 {
		t.Errorf("CartsStarted = %d, want 100", w.CartsStarted)
	}
	if w.CartsConverted != 60 {
		t.Errorf("CartsConverted = %d, want 60", w.CartsConverted)
	}
	if w.CartsAbandoned != 40 {
		t.Errorf("CartsAbandoned = %d, want 40", w.CartsAbandoned)
	}
}

func TestAbandonmentRateCalculation(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)
	_ = s.FlushAbandonment(ctx, key, 200, 50)

	windows, _ := s.GetAbandonment(ctx, 2)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	// 150 abandoned out of 200 = 0.75
	if windows[0].AbandonmentRate != 0.75 {
		t.Errorf("AbandonmentRate = %f, want 0.75", windows[0].AbandonmentRate)
	}
}

func TestAbandonmentZeroStarted(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)
	_ = s.FlushAbandonment(ctx, key, 0, 0)

	windows, _ := s.GetAbandonment(ctx, 2)
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if windows[0].AbandonmentRate != 0.0 {
		t.Errorf("AbandonmentRate = %f, want 0.0 when no carts started", windows[0].AbandonmentRate)
	}
}

func TestTrackAndCountAbandonmentUsers(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	key := time.Now().UTC().Format(windowKeyLayout)

	_ = s.TrackAbandonmentUser(ctx, key, "user-1", "started")
	_ = s.TrackAbandonmentUser(ctx, key, "user-2", "started")
	_ = s.TrackAbandonmentUser(ctx, key, "user-1", "started") // duplicate
	_ = s.TrackAbandonmentUser(ctx, key, "user-3", "converted")

	startedCount, err := s.CountAbandonmentUsers(ctx, key, "started")
	if err != nil {
		t.Fatalf("CountAbandonmentUsers: %v", err)
	}
	if startedCount != 2 {
		t.Errorf("started count = %d, want 2 (deduplicated)", startedCount)
	}

	convertedCount, _ := s.CountAbandonmentUsers(ctx, key, "converted")
	if convertedCount != 1 {
		t.Errorf("converted count = %d, want 1", convertedCount)
	}
}

func TestCountAbandonmentUsersEmptyBucket(t *testing.T) {
	t.Parallel()
	s := NewMockStore()

	count, err := s.CountAbandonmentUsers(context.Background(), "nonexistent", "started")
	if err != nil {
		t.Fatalf("CountAbandonmentUsers: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for nonexistent bucket, got %d", count)
	}
}

func TestGetRevenueSortedChronologically(t *testing.T) {
	t.Parallel()
	s := NewMockStore()
	ctx := context.Background()

	now := time.Now().UTC()
	key1 := now.Add(-2 * time.Hour).Format(windowKeyLayout)
	key2 := now.Add(-1 * time.Hour).Format(windowKeyLayout)
	key3 := now.Format(windowKeyLayout)

	// Insert out of order.
	_ = s.FlushRevenue(ctx, key3, 3000, 1)
	_ = s.FlushRevenue(ctx, key1, 1000, 1)
	_ = s.FlushRevenue(ctx, key2, 2000, 1)

	windows, _ := s.GetRevenue(ctx, 4)
	if len(windows) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(windows))
	}
	for i := 1; i < len(windows); i++ {
		if windows[i].WindowStart.Before(windows[i-1].WindowStart) {
			t.Errorf("windows not sorted: [%d]=%v before [%d]=%v",
				i, windows[i].WindowStart, i-1, windows[i-1].WindowStart)
		}
	}
}

// Verify MockStore satisfies the Store interface at compile time.
var _ Store = (*MockStore)(nil)
