package composite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- ProductServiceCatalog tests ----

func TestProductServiceCatalogHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/products/") || !strings.HasSuffix(r.URL.Path, "/abc-123") {
			t.Errorf("path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{
			"id":"abc-123","name":"Widget","category":"Tools",
			"price":1999,"stock":42
		}`)
	}))
	defer srv.Close()

	cat := ProductServiceCatalog{BaseURL: srv.URL, HTTP: srv.Client()}
	got, err := cat.GetProduct(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "abc-123" {
		t.Errorf("ID: want abc-123, got %s", got.ID)
	}
	if got.Name != "Widget" {
		t.Errorf("Name: want Widget, got %s", got.Name)
	}
	if got.Category != "Tools" {
		t.Errorf("Category: want Tools, got %s", got.Category)
	}
	if got.PriceCents != 1999 {
		t.Errorf("PriceCents: want 1999, got %d", got.PriceCents)
	}
	if got.Stock != 42 {
		t.Errorf("Stock: want 42, got %d", got.Stock)
	}
}

func TestProductServiceCatalog404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cat := ProductServiceCatalog{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := cat.GetProduct(context.Background(), "missing")
	if err != errProductNotFound {
		t.Fatalf("want errProductNotFound, got %v", err)
	}
}

func TestProductServiceCatalogNon200Non404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cat := ProductServiceCatalog{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := cat.GetProduct(context.Background(), "p1")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---- NopEmbeddingSource tests ----

func TestNopEmbeddingSourceReturnsMissing(t *testing.T) {
	var src NopEmbeddingSource
	vec, err := src.Embedding(context.Background(), "any-id")
	if err != errEmbeddingMissing {
		t.Fatalf("want errEmbeddingMissing, got %v", err)
	}
	if vec != nil {
		t.Errorf("expected nil vector, got %v", vec)
	}
}
