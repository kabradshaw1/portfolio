package aggregator

import (
	"context"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/window"
)

type trendingData struct {
	Scores map[string]float64 // productID -> weighted score
	Names  map[string]string  // productID -> product name
}

// TrendingAggregator tracks product trending scores in sliding windows.
type TrendingAggregator struct {
	window *window.SlidingWindow[trendingData]
	store  store.Store
}

const (
	trendingViewWeight    = 1.0
	trendingCartAddWeight = 3.0
)

// NewTrendingAggregator creates a trending aggregator with the given sliding window parameters.
func NewTrendingAggregator(
	windowSize, slideInterval, grace time.Duration,
	clock window.Clock,
	s store.Store,
) *TrendingAggregator {
	return &TrendingAggregator{
		window: window.NewSlidingWindow(
			windowSize, slideInterval, grace, clock,
			func() trendingData {
				return trendingData{
					Scores: make(map[string]float64),
					Names:  make(map[string]string),
				}
			},
			func(dst, src *trendingData) {
				for pid, score := range src.Scores {
					dst.Scores[pid] += score
				}
				for pid, name := range src.Names {
					dst.Names[pid] = name
				}
			},
		),
		store: s,
	}
}

// HandleView records a product view event with weight 1. Returns false if dropped.
func (a *TrendingAggregator) HandleView(eventTime time.Time, productID, productName string) bool {
	return a.window.Add(eventTime, func(d *trendingData) {
		d.Scores[productID] += trendingViewWeight
		if productName != "" {
			d.Names[productID] = productName
		}
	})
}

// HandleCartAdd records a cart-add event with weight 3. Returns false if dropped.
func (a *TrendingAggregator) HandleCartAdd(eventTime time.Time, productID string) bool {
	return a.window.Add(eventTime, func(d *trendingData) {
		d.Scores[productID] += trendingCartAddWeight
	})
}

// Flush writes all expired sliding windows to the store and evicts old sub-buckets.
// Returns the first error encountered but continues evicting after successful flushes.
func (a *TrendingAggregator) Flush(ctx context.Context) error {
	results := a.window.Tick()

	var firstErr error
	for _, r := range results {
		if err := a.store.FlushTrending(ctx, r.Key, r.Data.Scores, r.Data.Names); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		a.window.Evict(r.Key)
	}
	return firstErr
}
