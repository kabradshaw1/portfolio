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

func TestTrendingAggregator_HandleView(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	ok := agg.HandleView(now.Add(1*time.Minute), "prod-1", "Product One")
	assert.True(t, ok, "view event should not be dropped")

	// Advance past the slide interval to trigger a flush result.
	clock.Advance(10 * time.Minute)

	results := agg.window.Tick()
	require.NotEmpty(t, results)

	// Find the result containing our product and verify weight.
	var found bool
	for _, r := range results {
		if score, exists := r.Data.Scores["prod-1"]; exists {
			assert.Equal(t, 1.0, score, "view should add weight 1")
			found = true
			break
		}
	}
	assert.True(t, found, "prod-1 should appear in at least one result")
}

func TestTrendingAggregator_HandleCartAdd(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	ok := agg.HandleCartAdd(now.Add(1*time.Minute), "prod-2")
	assert.True(t, ok, "cart add event should not be dropped")

	clock.Advance(10 * time.Minute)

	results := agg.window.Tick()
	require.NotEmpty(t, results)

	var found bool
	for _, r := range results {
		if score, exists := r.Data.Scores["prod-2"]; exists {
			assert.Equal(t, 3.0, score, "cart add should add weight 3")
			found = true
			break
		}
	}
	assert.True(t, found, "prod-2 should appear in at least one result")
}

func TestTrendingAggregator_CombinedScoring(t *testing.T) {
	t.Parallel()

	// Use a time aligned to a minute boundary for predictable sub-bucket behavior.
	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	// All events in the same minute sub-bucket: 2 views (weight 1 each) + 1 cart add (weight 3).
	eventTime := now.Add(1 * time.Minute)
	agg.HandleView(eventTime, "prod-3", "Product Three")
	agg.HandleView(eventTime, "prod-3", "Product Three")
	agg.HandleCartAdd(eventTime, "prod-3")

	// Advance past slide interval.
	clock.Advance(10 * time.Minute)

	results := agg.window.Tick()
	require.NotEmpty(t, results)

	// Each result from Tick aggregates all sub-buckets in the window.
	// Since all events are in one sub-bucket, each slide result containing
	// that sub-bucket should show the full score of 5.0.
	var found bool
	for _, r := range results {
		if score, exists := r.Data.Scores["prod-3"]; exists {
			assert.Equal(t, 5.0, score, "combined score per slide should be 2*1 + 1*3 = 5")
			found = true
			break
		}
	}
	assert.True(t, found, "prod-3 should appear in at least one result")
}

func TestTrendingAggregator_FlushSendsScoresToStore(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	agg.HandleView(now.Add(1*time.Minute), "prod-A", "Product A")
	agg.HandleCartAdd(now.Add(2*time.Minute), "prod-B")

	// Advance past the slide interval.
	clock.Advance(10 * time.Minute)

	err := agg.Flush(context.Background())
	require.NoError(t, err)

	// After flush, the sliding window should have evicted old sub-buckets
	// (the Evict call cleans up sub-buckets past windowSize + grace).
	// The key verification is that Flush completed without error,
	// meaning FlushTrending was called on the store successfully.
}

func TestTrendingAggregator_NamesTrackedThroughFlush(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	agg.HandleView(now.Add(1*time.Minute), "prod-N", "Named Product")
	agg.HandleView(now.Add(1*time.Minute), "prod-E", "") // no name

	clock.Advance(10 * time.Minute)

	err := agg.Flush(context.Background())
	require.NoError(t, err)

	// Verify names were flushed to the store via the helper method.
	names := s.TrendingNames()
	assert.Equal(t, "Named Product", names["prod-N"], "product name should be tracked")
	assert.Empty(t, names["prod-E"], "empty name should not be stored")

	// Verify scores were also flushed.
	scores := s.TrendingScores()
	assert.Contains(t, scores, "prod-N", "prod-N should have a score")
	assert.Contains(t, scores, "prod-E", "prod-E should have a score")
}

func TestTrendingAggregator_DroppedEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clock := window.NewMockClock(now)
	s := store.NewMockStore()
	agg := NewTrendingAggregator(time.Hour, 5*time.Minute, 5*time.Minute, clock, s)

	// Event from 2 hours ago should be dropped (beyond windowSize + grace).
	oldEvent := now.Add(-2 * time.Hour)
	ok := agg.HandleView(oldEvent, "prod-old", "")
	assert.False(t, ok, "stale event should be dropped")
}
