package composite

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeProductCatalog struct{ items map[string]Product }

func (f fakeProductCatalog) GetProduct(ctx context.Context, id string) (Product, error) {
	p, ok := f.items[id]
	if !ok {
		return Product{}, errProductNotFound
	}
	return p, nil
}

type fakeEmbeddings struct{ vecs map[string][]float32 }

func (f fakeEmbeddings) Embedding(ctx context.Context, productID string) ([]float32, error) {
	v, ok := f.vecs[productID]
	if !ok {
		return nil, errEmbeddingMissing
	}
	return v, nil
}

func TestCompareProductsTwoItems(t *testing.T) {
	cat := fakeProductCatalog{items: map[string]Product{
		"a": {ID: "a", Name: "Trail Shoe", Category: "footwear", PriceCents: 12000},
		"b": {ID: "b", Name: "Road Shoe", Category: "footwear", PriceCents: 9000},
	}}
	emb := fakeEmbeddings{vecs: map[string][]float32{
		"a": {1, 0, 0},
		"b": {0.9, 0.1, 0},
	}}
	tool := NewCompareProductsTool(cat, emb)
	res, err := tool.Call(context.Background(), []byte(`{"product_ids":["a","b"]}`), "u1")
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(res.Content)
	if err != nil {
		t.Fatal(err)
	}
	var r CompareResult
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Products) != 2 {
		t.Fatalf("want 2 products, got %d", len(r.Products))
	}
	if len(r.Similarity) != 1 {
		t.Fatalf("want 1 similarity entry, got %d", len(r.Similarity))
	}
	if r.Similarity[0].Score < 0.9 {
		t.Fatalf("expected high similarity, got %f", r.Similarity[0].Score)
	}
	if _, ok := r.Shared["category"]; !ok {
		t.Fatalf("expected shared category")
	}
	foundPrice := false
	for _, d := range r.Differing {
		if d.Field == "price_cents" {
			foundPrice = true
		}
	}
	if !foundPrice {
		t.Fatalf("expected price difference")
	}
}

func TestCompareProductsRejectsLessThanTwo(t *testing.T) {
	tool := NewCompareProductsTool(fakeProductCatalog{}, fakeEmbeddings{})
	_, err := tool.Call(context.Background(), []byte(`{"product_ids":["a"]}`), "u1")
	if err == nil {
		t.Fatalf("expected error for <2 ids")
	}
}
