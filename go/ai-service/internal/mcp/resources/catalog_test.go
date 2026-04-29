package resources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeCatalogClient struct {
	categories []CatalogCategory
	featured   []CatalogProduct
	products   map[string]CatalogProduct
	err        error
}

func (f fakeCatalogClient) Categories(ctx context.Context) ([]CatalogCategory, error) {
	return f.categories, f.err
}
func (f fakeCatalogClient) Featured(ctx context.Context) ([]CatalogProduct, error) {
	return f.featured, f.err
}
func (f fakeCatalogClient) Product(ctx context.Context, id string) (CatalogProduct, error) {
	p, ok := f.products[id]
	if !ok {
		return CatalogProduct{}, ErrResourceNotFound
	}
	return p, nil
}

func TestCategoriesResource(t *testing.T) {
	c := fakeCatalogClient{categories: []CatalogCategory{{Name: "footwear", Count: 12}}}
	r := NewCategoriesResource(c)
	if r.URI() != "catalog://categories" {
		t.Fatalf("uri: %s", r.URI())
	}
	if r.MIMEType() != "application/json" {
		t.Fatalf("mime: %s", r.MIMEType())
	}
	got, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "footwear") {
		t.Fatalf("expected footwear, got %s", got.Text)
	}
}

func TestFeaturedResource(t *testing.T) {
	c := fakeCatalogClient{featured: []CatalogProduct{{ID: "a", Name: "Trail Shoe"}}}
	r := NewFeaturedResource(c)
	if r.URI() != "catalog://featured" {
		t.Fatalf("uri: %s", r.URI())
	}
	got, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "Trail Shoe") {
		t.Fatalf("got %s", got.Text)
	}
}

func TestProductResourceReadsByID(t *testing.T) {
	c := fakeCatalogClient{products: map[string]CatalogProduct{"p1": {ID: "p1", Name: "Trail Shoe"}}}
	r := NewProductResource(c, "p1")
	if r.URI() != "catalog://product/p1" {
		t.Fatalf("uri: %s", r.URI())
	}
	got, err := r.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "Trail Shoe") {
		t.Fatalf("got %s", got.Text)
	}
}

func TestProductResourceMissingIDReturnsErrResourceNotFound(t *testing.T) {
	c := fakeCatalogClient{products: map[string]CatalogProduct{"p1": {ID: "p1"}}}
	r := NewProductResource(c, "missing")
	_, err := r.Read(context.Background())
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestCategoriesResourcePropagatesClientError(t *testing.T) {
	c := fakeCatalogClient{err: errors.New("downstream down")}
	r := NewCategoriesResource(c)
	_, err := r.Read(context.Background())
	if err == nil {
		t.Fatalf("expected error from client")
	}
}
