package window

import (
	"sync"
	"time"
)

type bucket[T any] struct {
	start time.Time
	end   time.Time
	data  T
}

// TumblingWindow buckets events by eventTime.Truncate(windowSize).
type TumblingWindow[T any] struct {
	mu         sync.Mutex
	windowSize time.Duration
	grace      time.Duration
	clock      Clock
	zero       func() T
	buckets    map[string]*bucket[T]
}

// NewTumblingWindow creates a tumbling window with the given size, grace period,
// clock, and zero-value factory.
func NewTumblingWindow[T any](windowSize, grace time.Duration, clock Clock, zero func() T) *TumblingWindow[T] {
	return &TumblingWindow[T]{
		windowSize: windowSize,
		grace:      grace,
		clock:      clock,
		zero:       zero,
		buckets:    make(map[string]*bucket[T]),
	}
}

// Add places an event into the correct window bucket. The fn callback mutates the
// bucket's data. Returns the window key and whether the event was dropped (too late).
func (tw *TumblingWindow[T]) Add(eventTime time.Time, fn func(*T)) (key string, dropped bool) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	start := eventTime.UTC().Truncate(tw.windowSize)
	end := start.Add(tw.windowSize)
	key = start.UTC().Format(time.RFC3339)

	// Drop if the event's window end + grace has already passed.
	if tw.clock.Now().After(end.Add(tw.grace)) {
		return key, true
	}

	b, ok := tw.buckets[key]
	if !ok {
		data := tw.zero()
		b = &bucket[T]{start: start, end: end, data: data}
		tw.buckets[key] = b
	}

	fn(&b.data)
	return key, false
}

// Tick returns all windows whose end time + grace period has passed.
// It does NOT evict them — the caller should call Evict after successful flush.
func (tw *TumblingWindow[T]) Tick() []FlushResult[T] {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	now := tw.clock.Now()
	var results []FlushResult[T]

	for key, b := range tw.buckets {
		if now.After(b.end.Add(tw.grace)) {
			results = append(results, FlushResult[T]{
				Key:   key,
				Start: b.start,
				End:   b.end,
				Data:  b.data,
			})
		}
	}

	return results
}

// Evict removes a window from memory.
func (tw *TumblingWindow[T]) Evict(key string) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	delete(tw.buckets, key)
}

// ActiveKeys returns keys of all open windows.
func (tw *TumblingWindow[T]) ActiveKeys() []string {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	keys := make([]string, 0, len(tw.buckets))
	for k := range tw.buckets {
		keys = append(keys, k)
	}
	return keys
}
