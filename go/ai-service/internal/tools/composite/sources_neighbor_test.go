package composite

import (
	"context"
	"testing"
)

func TestNopNeighborSearchReturnsEmpty(t *testing.T) {
	got, err := NopNeighborSearch{}.Nearest(context.Background(), []float32{1, 2, 3}, 5, nil, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestNopNeighborSearchWithCategoryFilter(t *testing.T) {
	got, err := NopNeighborSearch{}.Nearest(context.Background(), []float32{0.1, 0.2}, 10, []string{"p1", "p2"}, "footwear")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil with category filter, got %+v", got)
	}
}
