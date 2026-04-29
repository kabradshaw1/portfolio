package composite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeUserHistory struct {
	orders []HistoricalItem
	cart   []HistoricalItem
	views  []HistoricalItem
}

func (f fakeUserHistory) Orders(ctx context.Context, userID string) ([]HistoricalItem, error) {
	return f.orders, nil
}
func (f fakeUserHistory) CartItems(ctx context.Context, userID string) ([]HistoricalItem, error) {
	return f.cart, nil
}
func (f fakeUserHistory) RecentlyViewed(ctx context.Context, userID string) ([]HistoricalItem, error) {
	return f.views, nil
}

type fakeNeighborSearch struct {
	results []NeighborResult
	err     error
}

func (f fakeNeighborSearch) Nearest(ctx context.Context, vec []float32, k int, excludeIDs []string, category string) ([]NeighborResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func TestRecommendRationaleProducesRationaleAndSignals(t *testing.T) {
	hist := fakeUserHistory{
		orders: []HistoricalItem{
			{ProductID: "shoe-trail", Embedding: []float32{1, 0, 0}, Source: "order:o1", Name: "Trail Shoe"},
		},
		cart: []HistoricalItem{
			{ProductID: "sock", Embedding: []float32{0, 1, 0}, Source: "cart:current", Name: "Wool Sock"},
		},
	}
	neigh := fakeNeighborSearch{results: []NeighborResult{
		{ProductID: "shoe-road", Score: 0.85, Name: "Road Shoe", Category: "footwear"},
		{ProductID: "shoe-trail-2", Score: 0.92, Name: "Trail Shoe v2", Category: "footwear"},
	}}
	tool := NewRecommendWithRationaleTool(hist, neigh)
	res, err := tool.Call(context.Background(), []byte(`{"user_id":"u1"}`), "u1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var r RecommendResult
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Products) != 2 {
		t.Fatalf("want 2 products, got %d", len(r.Products))
	}
	if r.Products[0].Rationale == "" {
		t.Fatalf("expected non-empty rationale")
	}
	if len(r.Products[0].SurfacedSignals) == 0 {
		t.Fatalf("expected signals")
	}
	if r.QueryEmbeddingSource == "" {
		t.Fatalf("expected non-empty query_embedding_source")
	}
}

func TestRecommendRationaleRejectsMissingUser(t *testing.T) {
	tool := NewRecommendWithRationaleTool(fakeUserHistory{}, fakeNeighborSearch{})
	_, err := tool.Call(context.Background(), []byte(`{}`), "u1")
	if err == nil {
		t.Fatalf("expected error on missing user_id")
	}
}

func TestRecommendRationaleNoHistoryReturnsEmptyResult(t *testing.T) {
	tool := NewRecommendWithRationaleTool(fakeUserHistory{}, fakeNeighborSearch{})
	res, err := tool.Call(context.Background(), []byte(`{"user_id":"u1"}`), "u1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(res.Content)
	var r RecommendResult
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Products) != 0 {
		t.Fatalf("expected no products, got %d", len(r.Products))
	}
	if r.QueryEmbeddingSource != "no_history" {
		t.Fatalf("query_embedding_source: %s", r.QueryEmbeddingSource)
	}
}

func TestRecommendRationaleHistoryWithoutEmbeddingsFallsBack(t *testing.T) {
	hist := fakeUserHistory{
		orders: []HistoricalItem{
			{ProductID: "shoe-trail", Source: "order:o1", Name: "Trail Shoe"}, // no embedding
		},
	}
	tool := NewRecommendWithRationaleTool(hist, fakeNeighborSearch{})
	res, err := tool.Call(context.Background(), []byte(`{"user_id":"u1"}`), "u1")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(res.Content)
	var r RecommendResult
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if r.QueryEmbeddingSource != "no_embeddings" {
		t.Fatalf("query_embedding_source: %s", r.QueryEmbeddingSource)
	}
	if len(r.Products) != 0 {
		t.Fatalf("expected no products without embeddings, got %d", len(r.Products))
	}
}

func TestRecommendRationalePropagatesNearestError(t *testing.T) {
	hist := fakeUserHistory{
		orders: []HistoricalItem{
			{ProductID: "shoe-trail", Embedding: []float32{1, 0, 0}, Source: "order:o1", Name: "Trail Shoe"},
		},
	}
	neigh := fakeNeighborSearch{err: errors.New("qdrant down")}
	tool := NewRecommendWithRationaleTool(hist, neigh)
	_, err := tool.Call(context.Background(), []byte(`{"user_id":"u1"}`), "u1")
	if err == nil {
		t.Fatalf("expected error from Nearest to propagate")
	}
}
