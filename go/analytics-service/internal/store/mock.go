package store

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MockStore is a thread-safe in-memory Store implementation for testing.
type MockStore struct {
	mu          sync.Mutex
	revenue     map[string]*RevenueWindow
	trending    map[string]map[string]float64
	trendNames  map[string]map[string]string // windowKey -> productID -> productName
	abandonment map[string]*AbandonmentWindow
	users       map[string]map[string]bool // key: "{windowKey}:{bucket}", value: set of userIDs
}

// NewMockStore creates an empty MockStore.
func NewMockStore() *MockStore {
	return &MockStore{
		revenue:     make(map[string]*RevenueWindow),
		trending:    make(map[string]map[string]float64),
		trendNames:  make(map[string]map[string]string),
		abandonment: make(map[string]*AbandonmentWindow),
		users:       make(map[string]map[string]bool),
	}
}

// FlushRevenue accumulates revenue data for the given window key.
func (m *MockStore) FlushRevenue(_ context.Context, windowKey string, totalCents, orderCount int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, ok := m.revenue[windowKey]
	if !ok {
		t, _ := time.Parse(windowKeyLayout, windowKey)
		w = &RevenueWindow{
			WindowStart: t,
			WindowEnd:   t.Add(time.Hour),
		}
		m.revenue[windowKey] = w
	}
	w.TotalCents += totalCents
	w.OrderCount += orderCount
	if w.OrderCount > 0 {
		w.AvgCents = w.TotalCents / w.OrderCount
	}
	return nil
}

// GetRevenue returns revenue windows for the last N hours, sorted chronologically.
func (m *MockStore) GetRevenue(_ context.Context, hours int) ([]RevenueWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	var result []RevenueWindow
	for _, w := range m.revenue {
		if w.WindowStart.After(cutoff) || w.WindowStart.Equal(cutoff) {
			result = append(result, *w)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].WindowStart.Before(result[j].WindowStart)
	})
	return result, nil
}

// FlushTrending writes product scores and names for the given window key.
func (m *MockStore) FlushTrending(_ context.Context, windowKey string, scores map[string]float64, names map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.trending[windowKey] = make(map[string]float64, len(scores))
	for k, v := range scores {
		m.trending[windowKey][k] = v
	}

	if len(names) > 0 {
		m.trendNames[windowKey] = make(map[string]string, len(names))
		for k, v := range names {
			m.trendNames[windowKey][k] = v
		}
	}
	return nil
}

// GetTrending returns trending products from the most recent window.
func (m *MockStore) GetTrending(_ context.Context, limit int) (*TrendingResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.trending) == 0 {
		return nil, nil
	}

	// Find the most recent window key.
	var latestKey string
	var latestTime time.Time
	for k := range m.trending {
		t, err := time.Parse(windowKeyLayout, k)
		if err != nil {
			continue
		}
		if latestKey == "" || t.After(latestTime) {
			latestKey = k
			latestTime = t
		}
	}

	scores := m.trending[latestKey]
	nameMap := m.trendNames[latestKey]
	products := make([]TrendingProduct, 0, len(scores))
	for pid, score := range scores {
		products = append(products, TrendingProduct{
			ProductID:   pid,
			ProductName: nameMap[pid],
			Score:       score,
		})
	}
	sort.Slice(products, func(i, j int) bool {
		return products[i].Score > products[j].Score
	})
	if limit > 0 && len(products) > limit {
		products = products[:limit]
	}

	return &TrendingResult{
		WindowEnd: latestTime.Add(time.Hour),
		Products:  products,
	}, nil
}

// FlushAbandonment writes cart abandonment metrics for the given window key.
func (m *MockStore) FlushAbandonment(_ context.Context, windowKey string, started, converted int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	abandoned := started - converted
	if abandoned < 0 {
		abandoned = 0
	}
	var rate float64
	if started > 0 {
		rate = float64(abandoned) / float64(started)
	}

	t, _ := time.Parse(windowKeyLayout, windowKey)
	m.abandonment[windowKey] = &AbandonmentWindow{
		WindowStart:     t,
		WindowEnd:       t.Add(time.Hour),
		CartsStarted:    started,
		CartsConverted:  converted,
		CartsAbandoned:  abandoned,
		AbandonmentRate: rate,
	}
	return nil
}

// GetAbandonment returns abandonment windows for the last N hours, sorted chronologically.
func (m *MockStore) GetAbandonment(_ context.Context, hours int) ([]AbandonmentWindow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
	var result []AbandonmentWindow
	for _, w := range m.abandonment {
		if w.WindowStart.After(cutoff) || w.WindowStart.Equal(cutoff) {
			result = append(result, *w)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].WindowStart.Before(result[j].WindowStart)
	})
	return result, nil
}

// TrackAbandonmentUser adds a user to the abandonment tracking set.
func (m *MockStore) TrackAbandonmentUser(_ context.Context, windowKey, userID, bucket string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	setKey := windowKey + ":" + bucket
	if m.users[setKey] == nil {
		m.users[setKey] = make(map[string]bool)
	}
	m.users[setKey][userID] = true
	return nil
}

// CountAbandonmentUsers returns the number of unique users in a given abandonment bucket.
func (m *MockStore) CountAbandonmentUsers(_ context.Context, windowKey, bucket string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	setKey := windowKey + ":" + bucket
	return int64(len(m.users[setKey])), nil
}

// RevenueLen returns the number of revenue window entries.
func (m *MockStore) RevenueLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.revenue)
}

// TotalRevenueCents sums all revenue across all windows.
func (m *MockStore) TotalRevenueCents() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, w := range m.revenue {
		total += w.TotalCents
	}
	return total
}

// TotalOrderCount sums all order counts across all windows.
func (m *MockStore) TotalOrderCount() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, w := range m.revenue {
		total += w.OrderCount
	}
	return total
}

// TrendingLen returns the number of trending window entries.
func (m *MockStore) TrendingLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.trending)
}

// TrendingScores returns a merged map of all product scores across all windows.
func (m *MockStore) TrendingScores() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := make(map[string]float64)
	for _, scores := range m.trending {
		for pid, score := range scores {
			merged[pid] += score
		}
	}
	return merged
}

// AbandonmentLen returns the number of abandonment window entries.
func (m *MockStore) AbandonmentLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.abandonment)
}

// TotalCartsStarted sums all carts started across all windows.
func (m *MockStore) TotalCartsStarted() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, w := range m.abandonment {
		total += w.CartsStarted
	}
	return total
}

// TotalCartsConverted sums all carts converted across all windows.
func (m *MockStore) TotalCartsConverted() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total int64
	for _, w := range m.abandonment {
		total += w.CartsConverted
	}
	return total
}

// TrendingNames returns a merged map of all product names across all windows.
func (m *MockStore) TrendingNames() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := make(map[string]string)
	for _, names := range m.trendNames {
		for pid, name := range names {
			merged[pid] = name
		}
	}
	return merged
}

// compile-time interface check
var _ Store = (*MockStore)(nil)

