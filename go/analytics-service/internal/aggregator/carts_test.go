package aggregator

import "testing"

func TestCartAggregator_Stats(t *testing.T) {
	a := NewCartAggregator()

	a.RecordItemAdded("p1")
	a.RecordItemAdded("p1")
	a.RecordItemAdded("p2")
	a.RecordItemRemoved()

	stats := a.Stats()

	if stats.ActiveCarts != 2 {
		t.Errorf("expected 2 active carts, got %d", stats.ActiveCarts)
	}
	if len(stats.MostAdded) != 2 {
		t.Fatalf("expected 2 products, got %d", len(stats.MostAdded))
	}
}

func TestCartAggregator_Empty(t *testing.T) {
	a := NewCartAggregator()
	stats := a.Stats()
	if stats.ActiveCarts != 0 {
		t.Errorf("expected 0, got %d", stats.ActiveCarts)
	}
}

func TestCartAggregator_MoreRemovesThanAdds(t *testing.T) {
	a := NewCartAggregator()
	a.RecordItemAdded("p1")
	a.RecordItemRemoved()
	a.RecordItemRemoved()

	stats := a.Stats()
	if stats.ActiveCarts != 0 {
		t.Errorf("expected 0, got %d", stats.ActiveCarts)
	}
}
