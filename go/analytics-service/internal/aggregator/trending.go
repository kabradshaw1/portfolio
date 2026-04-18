package aggregator

import (
	"sort"
	"time"
)

// TrendingSlot holds per-minute product interaction data.
type TrendingSlot struct {
	Views     map[string]int // productID → count
	Purchases map[string]int // productID → count
	Names     map[string]string // productID → name
}

func newTrendingSlot() TrendingSlot {
	return TrendingSlot{
		Views:     make(map[string]int),
		Purchases: make(map[string]int),
		Names:     make(map[string]string),
	}
}

// TrendingAggregator tracks product views and purchases in a 1-hour window.
type TrendingAggregator struct {
	window *Window[TrendingSlot]
}

const trendingWindowDuration = 1 * time.Hour

// NewTrendingAggregator creates an aggregator with a 1-hour window.
func NewTrendingAggregator() *TrendingAggregator {
	return &TrendingAggregator{
		window: NewWindow(trendingWindowDuration, newTrendingSlot),
	}
}

// RecordView records a product view.
func (a *TrendingAggregator) RecordView(productID, productName string) {
	a.window.Update(func(s *TrendingSlot) {
		s.Views[productID]++
		if productName != "" {
			s.Names[productID] = productName
		}
	})
}

// RecordPurchase records a product purchase.
func (a *TrendingAggregator) RecordPurchase(productID, productName string) {
	a.window.Update(func(s *TrendingSlot) {
		s.Purchases[productID]++
		if productName != "" {
			s.Names[productID] = productName
		}
	})
}

const (
	purchaseWeight = 5
	maxTrending    = 10
)

// TrendingProduct is a single product's trending score.
type TrendingProduct struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Score     int    `json:"score"`
	Views     int    `json:"views"`
	Purchases int    `json:"purchases"`
}

// TopProducts returns the top 10 trending products by score.
func (a *TrendingAggregator) TopProducts() []TrendingProduct {
	entries := a.window.Get()

	views := make(map[string]int)
	purchases := make(map[string]int)
	names := make(map[string]string)

	for _, e := range entries {
		for pid, count := range e.Value.Views {
			views[pid] += count
		}
		for pid, count := range e.Value.Purchases {
			purchases[pid] += count
		}
		for pid, name := range e.Value.Names {
			names[pid] = name
		}
	}

	// Merge into scored list.
	type scored struct {
		id    string
		score int
	}
	var all []scored
	seen := make(map[string]bool)
	for pid := range views {
		seen[pid] = true
		all = append(all, scored{id: pid, score: views[pid] + purchases[pid]*purchaseWeight})
	}
	for pid := range purchases {
		if !seen[pid] {
			all = append(all, scored{id: pid, score: purchases[pid] * purchaseWeight})
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	if len(all) > maxTrending {
		all = all[:maxTrending]
	}

	result := make([]TrendingProduct, len(all))
	for i, s := range all {
		result[i] = TrendingProduct{
			ID:        s.id,
			Name:      names[s.id],
			Score:     s.score,
			Views:     views[s.id],
			Purchases: purchases[s.id],
		}
	}
	return result
}
