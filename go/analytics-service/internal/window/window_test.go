package window

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// counter is a simple test data type for window aggregation.
type counter struct {
	Count int
	Total float64
}

func zeroCounter() counter {
	return counter{}
}

func mergeCounter(dst, src *counter) {
	dst.Count += src.Count
	dst.Total += src.Total
}

func addOne(c *counter) {
	c.Count++
}

func addAmount(amount float64) func(*counter) {
	return func(c *counter) {
		c.Count++
		c.Total += amount
	}
}

// ---------- Tumbling Window Tests ----------

func TestTumbling_EventLandsInCorrectBucket(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	// Event at 14:03 should land in the 14:00 bucket (truncated to 5m).
	eventTime := base.Add(3 * time.Minute)
	key, dropped := tw.Add(eventTime, addOne)

	assert.False(t, dropped)
	assert.Equal(t, "2026-04-22T14:00:00Z", key)
}

func TestTumbling_MultipleEventsAggregate(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	tw.Add(base.Add(1*time.Minute), addAmount(10.0))
	tw.Add(base.Add(2*time.Minute), addAmount(20.0))
	tw.Add(base.Add(3*time.Minute), addAmount(30.0))

	// Advance past window end + grace to flush.
	clk.Set(base.Add(6*time.Minute + 1*time.Second))

	results := tw.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, 3, results[0].Data.Count)
	assert.Equal(t, 60.0, results[0].Data.Total)
}

func TestTumbling_DifferentWindowsSeparateBuckets(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	// Event in 14:00-14:05 window.
	tw.Add(base.Add(2*time.Minute), addOne)
	// Event in 14:05-14:10 window.
	tw.Add(base.Add(6*time.Minute), addOne)

	keys := tw.ActiveKeys()
	sort.Strings(keys)
	assert.Equal(t, []string{"2026-04-22T14:00:00Z", "2026-04-22T14:05:00Z"}, keys)
}

func TestTumbling_LateEventWithinGraceAccepted(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base.Add(5*time.Minute + 30*time.Second)) // 14:05:30
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	// Event at 14:03, window ends at 14:05, grace 1m => deadline 14:06.
	// Clock is 14:05:30 which is before 14:06.
	key, dropped := tw.Add(base.Add(3*time.Minute), addOne)

	assert.False(t, dropped)
	assert.Equal(t, "2026-04-22T14:00:00Z", key)
}

func TestTumbling_LateEventPastGraceDropped(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base.Add(7 * time.Minute)) // 14:07
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	// Event at 14:03, window ends at 14:05, grace 1m => deadline 14:06.
	// Clock is 14:07 which is past deadline.
	key, dropped := tw.Add(base.Add(3*time.Minute), addOne)

	assert.True(t, dropped)
	assert.Equal(t, "2026-04-22T14:00:00Z", key)

	// Should not have created a bucket.
	assert.Empty(t, tw.ActiveKeys())
}

func TestTumbling_TickReturnsOnlyExpiredWindows(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	// Add events to two windows.
	tw.Add(base.Add(1*time.Minute), addOne)  // 14:00 window
	tw.Add(base.Add(6*time.Minute), addOne)  // 14:05 window

	// Advance clock to 14:06:01 — only 14:00 window should be expired.
	clk.Set(base.Add(6*time.Minute + 1*time.Second))

	results := tw.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, "2026-04-22T14:00:00Z", results[0].Key)
}

func TestTumbling_EvictRemovesWindow(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	tw.Add(base.Add(1*time.Minute), addOne)
	assert.Len(t, tw.ActiveKeys(), 1)

	tw.Evict("2026-04-22T14:00:00Z")
	assert.Empty(t, tw.ActiveKeys())
}

func TestTumbling_ActiveKeysReturnsCorrectSet(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	tw := NewTumblingWindow[counter](5*time.Minute, 1*time.Minute, clk, zeroCounter)

	tw.Add(base.Add(1*time.Minute), addOne)
	tw.Add(base.Add(6*time.Minute), addOne)
	tw.Add(base.Add(11*time.Minute), addOne)

	keys := tw.ActiveKeys()
	sort.Strings(keys)
	assert.Equal(t, []string{
		"2026-04-22T14:00:00Z",
		"2026-04-22T14:05:00Z",
		"2026-04-22T14:10:00Z",
	}, keys)
}

// ---------- Sliding Window Tests ----------

func TestSliding_EventsAddToCorrectSubBuckets(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 1*time.Minute,
		clk, zeroCounter, mergeCounter,
	)

	// Two events in the same minute, one in a different minute.
	ok1 := sw.Add(base.Add(30*time.Second), addOne)  // 14:00 minute
	ok2 := sw.Add(base.Add(45*time.Second), addOne)  // 14:00 minute
	ok3 := sw.Add(base.Add(90*time.Second), addOne)  // 14:01 minute

	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.True(t, ok3)

	// Should have 2 sub-buckets.
	assert.Len(t, sw.subBuckets, 2)
}

func TestSliding_TickReturnsAggregatedWindow(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 0,
		clk, zeroCounter, mergeCounter,
	)

	// Add events across several minutes.
	sw.Add(base.Add(10*time.Second), addAmount(10.0))  // 14:00 minute
	sw.Add(base.Add(70*time.Second), addAmount(20.0))  // 14:01 minute
	sw.Add(base.Add(130*time.Second), addAmount(30.0)) // 14:02 minute

	// Advance past the first slide boundary (14:01).
	clk.Set(base.Add(1 * time.Minute))

	results := sw.Tick()
	require.Len(t, results, 1)
	// Window is [13:56, 14:01). Only 14:00 sub-bucket is in range.
	assert.Equal(t, 1, results[0].Data.Count)
	assert.Equal(t, 10.0, results[0].Data.Total)
}

func TestSliding_MultipleSlides(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 0,
		clk, zeroCounter, mergeCounter,
	)

	sw.Add(base.Add(10*time.Second), addAmount(10.0))  // 14:00 minute
	sw.Add(base.Add(70*time.Second), addAmount(20.0))  // 14:01 minute

	// Jump ahead 3 minutes — should produce 3 slide results.
	clk.Set(base.Add(3 * time.Minute))

	results := sw.Tick()
	require.Len(t, results, 3)
}

func TestSliding_LateEventWithinGraceAccepted(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base.Add(5 * time.Minute)) // 14:05
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 2*time.Minute,
		clk, zeroCounter, mergeCounter,
	)

	// Event at 13:58 — cutoff is 14:05 - (5m + 2m) = 13:58.
	// 13:58 is not before 13:58, so it should be accepted.
	ok := sw.Add(base.Add(-2*time.Minute), addOne)
	assert.True(t, ok)
}

func TestSliding_LateEventPastGraceDropped(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base.Add(5 * time.Minute)) // 14:05
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 2*time.Minute,
		clk, zeroCounter, mergeCounter,
	)

	// Event at 13:57 — cutoff is 13:58. 13:57 < 13:58, dropped.
	ok := sw.Add(base.Add(-3*time.Minute), addOne)
	assert.False(t, ok)
}

func TestSliding_EvictRemovesOldSubBuckets(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	sw := NewSlidingWindow[counter](
		5*time.Minute, 1*time.Minute, 1*time.Minute,
		clk, zeroCounter, mergeCounter,
	)

	sw.Add(base.Add(10*time.Second), addOne)  // 14:00 minute
	sw.Add(base.Add(70*time.Second), addOne)  // 14:01 minute

	assert.Len(t, sw.subBuckets, 2)

	// Advance clock far enough that 14:00 sub-bucket is outside window + grace.
	// cutoff = 14:10 - (5m + 1m) = 14:04. Both 14:00 and 14:01 are before 14:04.
	clk.Set(base.Add(10 * time.Minute))

	sw.Evict("")
	assert.Empty(t, sw.subBuckets)
}

func TestSliding_MergeFunctionCombinesSubBuckets(t *testing.T) {
	base := time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC)
	clk := NewMockClock(base)
	sw := NewSlidingWindow[counter](
		3*time.Minute, 1*time.Minute, 0,
		clk, zeroCounter, mergeCounter,
	)

	// Add events to 3 consecutive minutes that will all fall in one window.
	sw.Add(base.Add(-2*time.Minute+10*time.Second), addAmount(10.0)) // 13:58
	sw.Add(base.Add(-1*time.Minute+10*time.Second), addAmount(20.0)) // 13:59
	sw.Add(base.Add(10*time.Second), addAmount(30.0))                // 14:00

	// Slide at 14:01 covers [13:58, 14:01) — all three sub-buckets.
	clk.Set(base.Add(1 * time.Minute))

	results := sw.Tick()
	require.Len(t, results, 1)
	assert.Equal(t, 3, results[0].Data.Count)
	assert.Equal(t, 60.0, results[0].Data.Total)
}
