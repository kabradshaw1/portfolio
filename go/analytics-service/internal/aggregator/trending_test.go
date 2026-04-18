package aggregator

import "testing"

func TestTrendingAggregator_TopProducts(t *testing.T) {
	a := NewTrendingAggregator()

	// Product A: 10 views, 2 purchases → score = 10 + 2*5 = 20
	for i := 0; i < 10; i++ {
		a.RecordView("a", "Product A")
	}
	a.RecordPurchase("a", "Product A")
	a.RecordPurchase("a", "Product A")

	// Product B: 5 views, 0 purchases → score = 5
	for i := 0; i < 5; i++ {
		a.RecordView("b", "Product B")
	}

	// Product C: 0 views, 3 purchases → score = 15
	for i := 0; i < 3; i++ {
		a.RecordPurchase("c", "Product C")
	}

	top := a.TopProducts()
	if len(top) != 3 {
		t.Fatalf("expected 3 products, got %d", len(top))
	}
	if top[0].ID != "a" {
		t.Errorf("expected product a first, got %s", top[0].ID)
	}
	if top[0].Score != 20 {
		t.Errorf("expected score 20, got %d", top[0].Score)
	}
	if top[1].ID != "c" {
		t.Errorf("expected product c second, got %s", top[1].ID)
	}
}

func TestTrendingAggregator_Empty(t *testing.T) {
	a := NewTrendingAggregator()
	top := a.TopProducts()
	if len(top) != 0 {
		t.Errorf("expected empty list, got %d", len(top))
	}
}

func TestTrendingAggregator_MaxTen(t *testing.T) {
	a := NewTrendingAggregator()
	for i := 0; i < 15; i++ {
		a.RecordView(string(rune('a'+i)), "")
	}
	top := a.TopProducts()
	if len(top) != 10 {
		t.Errorf("expected max 10, got %d", len(top))
	}
}
