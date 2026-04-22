package window

import "time"

// FlushResult holds a window's aggregated data ready for flushing to storage.
type FlushResult[T any] struct {
	Key   string    // ISO8601 window key (e.g., "2026-04-22T14:00:00Z")
	Start time.Time
	End   time.Time
	Data  T
}
