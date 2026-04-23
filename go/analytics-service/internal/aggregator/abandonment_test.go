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

func TestAbandonmentAggregator_HandleCartItemAdded(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewAbandonmentAggregator(time.Hour, 5*time.Minute, clock, s)

	ok := agg.HandleCartItemAdded(now, "user-1")
	assert.True(t, ok, "event within window should not be dropped")

	keys := agg.window.ActiveKeys()
	assert.Len(t, keys, 1, "should have one active window")
}

func TestAbandonmentAggregator_HandleOrderCompleted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewAbandonmentAggregator(time.Hour, 5*time.Minute, clock, s)

	ok := agg.HandleOrderCompleted(now, "user-1")
	assert.True(t, ok, "event within window should not be dropped")
}

func TestAbandonmentAggregator_FlushComputesCorrectCounts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewAbandonmentAggregator(time.Hour, 5*time.Minute, clock, s)

	// 3 users start carts, 1 converts.
	agg.HandleCartItemAdded(now, "user-1")
	agg.HandleCartItemAdded(now, "user-2")
	agg.HandleCartItemAdded(now, "user-3")
	agg.HandleOrderCompleted(now, "user-1")

	// Advance past window end + grace.
	clock.Advance(time.Hour + 10*time.Minute)

	// Verify the window data before flushing.
	results := agg.window.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, 3, len(results[0].Data.StartedUsers), "should have 3 started users")
	assert.Equal(t, 1, len(results[0].Data.ConvertedUsers), "should have 1 converted user")

	// Now flush (need to re-tick since we consumed the results, but Tick doesn't evict).
	err := agg.Flush(context.Background())
	require.NoError(t, err)

	// Window should be evicted after successful flush.
	assert.Empty(t, agg.window.ActiveKeys(), "window should be evicted after flush")
}

func TestAbandonmentAggregator_UserDedup(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewAbandonmentAggregator(time.Hour, 5*time.Minute, clock, s)

	// Same user adds to cart multiple times — should count as 1.
	agg.HandleCartItemAdded(now, "user-1")
	agg.HandleCartItemAdded(now, "user-1")
	agg.HandleCartItemAdded(now, "user-1")

	// Advance past window end + grace.
	clock.Advance(time.Hour + 10*time.Minute)

	// Verify dedup via Tick data.
	results := agg.window.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, 1, len(results[0].Data.StartedUsers), "duplicate user should count once")
}

func TestAbandonmentAggregator_DroppedEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 30, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewAbandonmentAggregator(time.Hour, 5*time.Minute, clock, s)

	// Event from 2 hours ago should be dropped.
	oldEvent := now.Add(-2 * time.Hour)
	ok := agg.HandleCartItemAdded(oldEvent, "user-old")
	assert.False(t, ok, "stale event should be dropped")

	// No window should have been created.
	keys := agg.window.ActiveKeys()
	assert.Empty(t, keys, "dropped event should not create a window")
}
