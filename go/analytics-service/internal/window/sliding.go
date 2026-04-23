package window

import (
	"sync"
	"time"
)

// SlidingWindow aggregates events over a sliding window built on minute-granularity
// sub-buckets.
type SlidingWindow[T any] struct {
	mu            sync.Mutex
	windowSize    time.Duration
	slideInterval time.Duration
	grace         time.Duration
	clock         Clock
	zero          func() T
	merge         func(dst, src *T)
	subBuckets    map[string]*T // keyed by minute truncation (RFC3339)
	lastSlideEnd  time.Time
}

// NewSlidingWindow creates a sliding window with the given parameters.
//   - windowSize: the total window duration
//   - slideInterval: how often the window slides forward
//   - grace: late-arrival tolerance
//   - clock: time source
//   - zero: factory for new zero-value data
//   - merge: combines src into dst
func NewSlidingWindow[T any](
	windowSize, slideInterval, grace time.Duration,
	clock Clock,
	zero func() T,
	merge func(dst, src *T),
) *SlidingWindow[T] {
	now := clock.Now().UTC()
	// Align the first slide end to the next slide boundary.
	lastSlideEnd := now.Truncate(slideInterval).Add(slideInterval)

	return &SlidingWindow[T]{
		windowSize:    windowSize,
		slideInterval: slideInterval,
		grace:         grace,
		clock:         clock,
		zero:          zero,
		merge:         merge,
		subBuckets:    make(map[string]*T),
		lastSlideEnd:  lastSlideEnd,
	}
}

// Add places an event into its minute sub-bucket. Returns false if the event
// is older than windowSize + grace from clock.Now().
func (sw *SlidingWindow[T]) Add(eventTime time.Time, fn func(*T)) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := sw.clock.Now()
	cutoff := now.Add(-(sw.windowSize + sw.grace))
	if eventTime.Before(cutoff) {
		return false
	}

	minuteKey := eventTime.UTC().Truncate(time.Minute).Format(time.RFC3339)
	sb, ok := sw.subBuckets[minuteKey]
	if !ok {
		z := sw.zero()
		sb = &z
		sw.subBuckets[minuteKey] = sb
	}

	fn(sb)
	return true
}

// Tick checks if a new slide interval has passed. If so, it aggregates all
// sub-buckets within [slideEnd - windowSize, slideEnd) into a FlushResult.
// Returns nil if no slide is due yet.
func (sw *SlidingWindow[T]) Tick() []FlushResult[T] {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := sw.clock.Now().UTC()
	var results []FlushResult[T]

	for !now.Before(sw.lastSlideEnd) {
		slideEnd := sw.lastSlideEnd
		slideStart := slideEnd.Add(-sw.windowSize)

		merged := sw.zero()

		for keyStr, sb := range sw.subBuckets {
			t, err := time.Parse(time.RFC3339, keyStr)
			if err != nil {
				continue
			}
			// Include sub-buckets in [slideStart, slideEnd).
			if !t.Before(slideStart) && t.Before(slideEnd) {
				sw.merge(&merged, sb)
			}
		}

		key := slideStart.UTC().Format(time.RFC3339)
		results = append(results, FlushResult[T]{
			Key:   key,
			Start: slideStart,
			End:   slideEnd,
			Data:  merged,
		})

		sw.lastSlideEnd = sw.lastSlideEnd.Add(sw.slideInterval)
	}

	return results
}

// Evict removes sub-buckets that are outside the window + grace period.
func (sw *SlidingWindow[T]) Evict(_ string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	cutoff := sw.clock.Now().UTC().Add(-(sw.windowSize + sw.grace))
	for keyStr := range sw.subBuckets {
		t, err := time.Parse(time.RFC3339, keyStr)
		if err != nil {
			delete(sw.subBuckets, keyStr)
			continue
		}
		// Sub-bucket's minute is before the cutoff — evict.
		if t.Before(cutoff) {
			delete(sw.subBuckets, keyStr)
		}
	}
}
