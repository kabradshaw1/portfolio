package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEcommerceClient_GetProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/abc-123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc-123","name":"Waterproof Jacket","price":129.99,"stock":4}`))
	}))
	defer server.Close()

	c := NewEcommerceClient(server.URL)
	p, err := c.GetProduct(context.Background(), "abc-123")
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.ID != "abc-123" || p.Name != "Waterproof Jacket" || p.Price != 129.99 || p.Stock != 4 {
		t.Errorf("unexpected product: %+v", p)
	}
}

func TestEcommerceClient_GetProduct_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := NewEcommerceClient(server.URL).GetProduct(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestEcommerceClient_ListProducts_TextSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "jacket" {
			t.Fatalf("expected q=jacket, got %q", r.URL.Query().Get("q"))
		}
		_, _ = w.Write([]byte(`[
			{"id":"p1","name":"Waterproof Jacket","price":129.99,"stock":4},
			{"id":"p2","name":"Rain Jacket","price":89.00,"stock":10}
		]`))
	}))
	defer server.Close()

	c := NewEcommerceClient(server.URL)
	ps, err := c.ListProducts(context.Background(), "jacket", 10)
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(ps) != 2 {
		t.Fatalf("expected 2 results, got %d", len(ps))
	}
	if ps[0].Name != "Waterproof Jacket" {
		t.Errorf("first product wrong: %+v", ps[0])
	}
}
