package aggregator

import (
	"context"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
)

type abandonmentData struct {
	StartedUsers   map[string]bool // userIDs who added to cart
	ConvertedUsers map[string]bool // userIDs who completed orders
}

// AbandonmentAggregator tracks cart abandonment in tumbling windows.
type AbandonmentAggregator struct {
	window *window.TumblingWindow[abandonmentData]
	store  store.Store
}

// NewAbandonmentAggregator creates an abandonment aggregator with the given window size and grace period.
func NewAbandonmentAggregator(windowSize, grace time.Duration, clock window.Clock, s store.Store) *AbandonmentAggregator {
	return &AbandonmentAggregator{
		window: window.NewTumblingWindow(windowSize, grace, clock, func() abandonmentData {
			return abandonmentData{
				StartedUsers:   make(map[string]bool),
				ConvertedUsers: make(map[string]bool),
			}
		}),
		store: s,
	}
}

// HandleCartItemAdded records a user adding an item to their cart. Returns false if dropped.
func (a *AbandonmentAggregator) HandleCartItemAdded(eventTime time.Time, userID string) bool {
	_, dropped := a.window.Add(eventTime, func(d *abandonmentData) {
		d.StartedUsers[userID] = true
	})
	return !dropped
}

// HandleOrderCompleted records a user completing an order. Returns false if dropped.
func (a *AbandonmentAggregator) HandleOrderCompleted(eventTime time.Time, userID string) bool {
	_, dropped := a.window.Add(eventTime, func(d *abandonmentData) {
		d.ConvertedUsers[userID] = true
	})
	return !dropped
}

// Flush writes all expired windows to the store and evicts them from memory.
// Returns the first error encountered but continues evicting successfully flushed windows.
func (a *AbandonmentAggregator) Flush(ctx context.Context) error {
	results := a.window.Tick()

	var firstErr error
	for _, r := range results {
		started := int64(len(r.Data.StartedUsers))
		converted := int64(len(r.Data.ConvertedUsers))

		if err := a.store.FlushAbandonment(ctx, r.Key, started, converted); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		a.window.Evict(r.Key)
	}
	return firstErr
}
