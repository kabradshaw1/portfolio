package aggregator

import (
	"sync"
	"time"
)

// slotDuration is the granularity of each time bucket.
const slotDuration = 1 * time.Minute

// Window is a thread-safe sliding time window that accumulates values per minute.
// T is the type of value stored in each slot.
type Window[T any] struct {
	mu       sync.RWMutex
	slots    []slot[T]
	duration time.Duration // total window span
	zero     func() T     // creates a zero value for a new slot
	now      func() time.Time
}

type slot[T any] struct {
	start time.Time
	value T
}

// NewWindow creates a sliding window with the given total duration.
// zero returns a fresh zero-value for each new slot.
func NewWindow[T any](duration time.Duration, zero func() T) *Window[T] {
	return &Window[T]{
		duration: duration,
		zero:     zero,
		now:      time.Now,
	}
}

// Get returns a copy of all non-expired slot values and their timestamps.
func (w *Window[T]) Get() []SlotEntry[T] {
	w.mu.RLock()
	defer w.mu.RUnlock()

	cutoff := w.now().Add(-w.duration)
	var result []SlotEntry[T]
	for _, s := range w.slots {
		if s.start.After(cutoff) || s.start.Equal(cutoff) {
			result = append(result, SlotEntry[T]{Start: s.start, Value: s.value})
		}
	}
	return result
}

// SlotEntry is a read-only copy of a slot for external use.
type SlotEntry[T any] struct {
	Start time.Time
	Value T
}

// Update applies fn to the current slot's value, creating a new slot if needed.
func (w *Window[T]) Update(fn func(*T)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	slotStart := now.Truncate(slotDuration)

	// Evict expired slots.
	cutoff := now.Add(-w.duration)
	first := 0
	for first < len(w.slots) && w.slots[first].start.Before(cutoff) {
		first++
	}
	if first > 0 {
		w.slots = w.slots[first:]
	}

	// Find or create the current slot.
	if len(w.slots) > 0 && w.slots[len(w.slots)-1].start.Equal(slotStart) {
		fn(&w.slots[len(w.slots)-1].value)
		return
	}

	v := w.zero()
	fn(&v)
	w.slots = append(w.slots, slot[T]{start: slotStart, value: v})
}
