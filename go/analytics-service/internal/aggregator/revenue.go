package aggregator

import (
	"context"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
)

type revenueData struct {
	TotalCents int64
	OrderCount int64
}

// RevenueAggregator tracks order revenue in tumbling windows.
type RevenueAggregator struct {
	window *window.TumblingWindow[revenueData]
	store  store.Store
}

// NewRevenueAggregator creates a revenue aggregator with the given window size and grace period.
func NewRevenueAggregator(windowSize, grace time.Duration, clock window.Clock, s store.Store) *RevenueAggregator {
	return &RevenueAggregator{
		window: window.NewTumblingWindow(windowSize, grace, clock, func() revenueData {
			return revenueData{}
		}),
		store: s,
	}
}

// HandleOrderCompleted records an order completion event. Returns false if the event was dropped
// (too late for its window).
func (a *RevenueAggregator) HandleOrderCompleted(eventTime time.Time, totalCents int64) bool {
	_, dropped := a.window.Add(eventTime, func(d *revenueData) {
		d.TotalCents += totalCents
		d.OrderCount++
	})
	return !dropped
}

// Flush writes all expired windows to the store and evicts them from memory.
// Returns the first error encountered but continues evicting successfully flushed windows.
func (a *RevenueAggregator) Flush(ctx context.Context) error {
	results := a.window.Tick()

	var firstErr error
	for _, r := range results {
		if err := a.store.FlushRevenue(ctx, r.Key, r.Data.TotalCents, r.Data.OrderCount); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		a.window.Evict(r.Key)
	}
	return firstErr
}
