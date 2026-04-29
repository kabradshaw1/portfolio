package clients

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogResourceClientCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/categories" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"categories":["footwear","packs"]}`)
	}))
	defer srv.Close()

	got, err := NewCatalogResourceClient(srv.URL).Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "footwear" {
		t.Fatalf("unexpected categories: %+v", got)
	}
}

func TestCatalogResourceClientFeatured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products" || r.URL.Query().Get("limit") != "5" {
			t.Fatalf("request: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"products":[{"id":"p1","name":"Trail Shoe","category":"footwear","price":12000,"stock":4}]}`)
	}))
	defer srv.Close()

	got, err := NewCatalogResourceClient(srv.URL).Featured(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PriceCents != 12000 {
		t.Fatalf("unexpected products: %+v", got)
	}
}

func TestCatalogResourceClientProduct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/products/p1" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":"p1","name":"Trail Shoe","category":"footwear","price":12000,"stock":4}`)
	}))
	defer srv.Close()

	got, err := NewCatalogResourceClient(srv.URL).Product(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "p1" || got.Name != "Trail Shoe" || got.PriceCents != 12000 {
		t.Fatalf("unexpected product: %+v", got)
	}
}
