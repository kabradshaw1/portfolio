package aggregator

import "time"

// CartSlot holds per-minute cart interaction data.
type CartSlot struct {
	Added   int
	Removed int
	Products map[string]int // productID → add count
}

func newCartSlot() CartSlot {
	return CartSlot{Products: make(map[string]int)}
}

// CartAggregator tracks cart events in a 1-hour sliding window.
type CartAggregator struct {
	window *Window[CartSlot]
}

const cartWindowDuration = 1 * time.Hour

// NewCartAggregator creates an aggregator with a 1-hour window.
func NewCartAggregator() *CartAggregator {
	return &CartAggregator{
		window: NewWindow(cartWindowDuration, newCartSlot),
	}
}

// RecordItemAdded records a cart.item_added event.
func (a *CartAggregator) RecordItemAdded(productID string) {
	a.window.Update(func(s *CartSlot) {
		s.Added++
		s.Products[productID]++
	})
}

// RecordItemRemoved records a cart.item_removed event.
func (a *CartAggregator) RecordItemRemoved() {
	a.window.Update(func(s *CartSlot) {
		s.Removed++
	})
}

// CartStats returns aggregate cart statistics.
type CartStats struct {
	ActiveCarts int              `json:"activeCarts"`
	MostAdded   []ProductCount   `json:"mostAdded"`
}

// ProductCount pairs a product with its add count.
type ProductCount struct {
	ProductID string `json:"productId"`
	Count     int    `json:"count"`
}

// Stats computes aggregate cart statistics from the window.
func (a *CartAggregator) Stats() CartStats {
	entries := a.window.Get()

	var totalAdded, totalRemoved int
	products := make(map[string]int)

	for _, e := range entries {
		totalAdded += e.Value.Added
		totalRemoved += e.Value.Removed
		for pid, count := range e.Value.Products {
			products[pid] += count
		}
	}

	activeCarts := totalAdded - totalRemoved
	if activeCarts < 0 {
		activeCarts = 0
	}

	mostAdded := make([]ProductCount, 0, len(products))
	for pid, count := range products {
		mostAdded = append(mostAdded, ProductCount{ProductID: pid, Count: count})
	}

	return CartStats{
		ActiveCarts: activeCarts,
		MostAdded:   mostAdded,
	}
}
