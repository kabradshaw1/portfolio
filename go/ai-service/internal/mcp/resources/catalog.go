package resources

import (
	"context"
	"encoding/json"
	"fmt"
)

// CatalogCategory is a category-name + count pair surfaced to the LLM via
// catalog://categories.
type CatalogCategory struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CatalogProduct is the structural product view used by catalog://featured
// and catalog://product/{id}.
type CatalogProduct struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Stock      int    `json:"stock"`
}

// CatalogClient is the data source the catalog resources read from.
// Implementations call product-service over the network; tests use a fake.
type CatalogClient interface {
	Categories(ctx context.Context) ([]CatalogCategory, error)
	Featured(ctx context.Context) ([]CatalogProduct, error)
	Product(ctx context.Context, id string) (CatalogProduct, error)
}

const (
	uriCategories      = "catalog://categories"
	uriFeatured        = "catalog://featured"
	mimeJSON           = "application/json"
	productURIPrefix   = "catalog://product/"
	productURITemplate = productURIPrefix + "%s"
)

// categoriesResource implements catalog://categories.
type categoriesResource struct{ c CatalogClient }

// NewCategoriesResource returns the catalog://categories resource.
func NewCategoriesResource(c CatalogClient) Resource { return categoriesResource{c: c} }
func (r categoriesResource) URI() string             { return uriCategories }
func (r categoriesResource) Name() string            { return "Product categories" }
func (r categoriesResource) Description() string     { return "List of product categories with counts." }
func (r categoriesResource) MIMEType() string        { return mimeJSON }
func (r categoriesResource) Read(ctx context.Context) (Content, error) {
	cats, err := r.c.Categories(ctx)
	if err != nil {
		return Content{}, fmt.Errorf("catalog://categories: %w", err)
	}
	body, err := json.MarshalIndent(cats, "", "  ")
	if err != nil {
		return Content{}, fmt.Errorf("catalog://categories: marshal: %w", err)
	}
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}

// featuredResource implements catalog://featured.
type featuredResource struct{ c CatalogClient }

// NewFeaturedResource returns the catalog://featured resource.
func NewFeaturedResource(c CatalogClient) Resource { return featuredResource{c: c} }
func (r featuredResource) URI() string             { return uriFeatured }
func (r featuredResource) Name() string            { return "Featured products" }
func (r featuredResource) Description() string     { return "Curated featured product set." }
func (r featuredResource) MIMEType() string        { return mimeJSON }
func (r featuredResource) Read(ctx context.Context) (Content, error) {
	items, err := r.c.Featured(ctx)
	if err != nil {
		return Content{}, fmt.Errorf("catalog://featured: %w", err)
	}
	body, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return Content{}, fmt.Errorf("catalog://featured: marshal: %w", err)
	}
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}

// productResource implements catalog://product/{id} for a specific id.
// This resource is templated — task F dispatches to it on `resources/read`
// when the URI matches the productURIPrefix.
type productResource struct {
	c  CatalogClient
	id string
}

// NewProductResource returns a Resource for catalog://product/{id}.
func NewProductResource(c CatalogClient, id string) Resource {
	return productResource{c: c, id: id}
}
func (r productResource) URI() string         { return fmt.Sprintf(productURITemplate, r.id) }
func (r productResource) Name() string        { return fmt.Sprintf("Product %s", r.id) }
func (r productResource) Description() string { return "Single product detail." }
func (r productResource) MIMEType() string    { return mimeJSON }
func (r productResource) Read(ctx context.Context) (Content, error) {
	p, err := r.c.Product(ctx, r.id)
	if err != nil {
		return Content{}, fmt.Errorf("%s: %w", r.URI(), err)
	}
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return Content{}, fmt.Errorf("%s: marshal: %w", r.URI(), err)
	}
	return Content{URI: r.URI(), MIMEType: r.MIMEType(), Text: string(body)}, nil
}
