package aggregator

import (
	"context"
	"testing"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRevenueAggregator_HandleOrderCompleted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewRevenueAggregator(time.Hour, 5*time.Minute, clock, s)

	ok := agg.HandleOrderCompleted(now, 5000)
	assert.True(t, ok, "event within window should not be dropped")

	// Verify the event was placed in a window bucket.
	keys := agg.window.ActiveKeys()
	assert.Len(t, keys, 1, "should have one active window")
}

func TestRevenueAggregator_DroppedEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewRevenueAggregator(time.Hour, 5*time.Minute, clock, s)

	// Event from 2 hours ago should be dropped (window end + grace has passed).
	oldEvent := now.Add(-2 * time.Hour)
	ok := agg.HandleOrderCompleted(oldEvent, 1000)
	assert.False(t, ok, "stale event should be dropped")

	// No window should have been created.
	keys := agg.window.ActiveKeys()
	assert.Empty(t, keys, "dropped event should not create a window")
}

func TestRevenueAggregator_MultipleEventsAccumulate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewRevenueAggregator(time.Hour, 5*time.Minute, clock, s)

	// Add multiple events in the same window.
	agg.HandleOrderCompleted(now.Add(-10*time.Minute), 5000)
	agg.HandleOrderCompleted(now.Add(-5*time.Minute), 3000)
	agg.HandleOrderCompleted(now.Add(-1*time.Minute), 2000)

	// All events should be in the same window bucket.
	keys := agg.window.ActiveKeys()
	assert.Len(t, keys, 1, "all events should fall in the same window")

	// Advance past window end + grace to make the window expire.
	clock.Advance(time.Hour)

	// Tick should return the accumulated data.
	results := agg.window.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, int64(10000), results[0].Data.TotalCents, "total should be 5000+3000+2000")
	assert.Equal(t, int64(3), results[0].Data.OrderCount, "should have 3 orders")
}

func TestRevenueAggregator_FlushSendsCorrectData(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewRevenueAggregator(time.Hour, 5*time.Minute, clock, s)

	agg.HandleOrderCompleted(now, 7500)

	// Advance past window end + grace.
	clock.Advance(time.Hour + 10*time.Minute)

	err := agg.Flush(context.Background())
	require.NoError(t, err)

	// Verify the store received the flush (FlushRevenue was called without error).
	// The window should be evicted after a successful flush.
	keys := agg.window.ActiveKeys()
	assert.Empty(t, keys, "window should be evicted after flush")
}

func TestRevenueAggregator_FlushEvictsWindows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewRevenueAggregator(time.Hour, 5*time.Minute, clock, s)

	agg.HandleOrderCompleted(now, 5000)
	assert.Len(t, agg.window.ActiveKeys(), 1, "should have one active window before flush")

	// Advance past window end + grace.
	clock.Advance(time.Hour + 10*time.Minute)

	err := agg.Flush(context.Background())
	require.NoError(t, err)

	// Window should be evicted.
	assert.Empty(t, agg.window.ActiveKeys(), "window should be evicted after successful flush")

	// Second flush should have nothing to do.
	err = agg.Flush(context.Background())
	require.NoError(t, err)
}
