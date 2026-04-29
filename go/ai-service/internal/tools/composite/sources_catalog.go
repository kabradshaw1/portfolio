package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ProductServiceCatalog calls product-service over REST to retrieve product data.
type ProductServiceCatalog struct {
	BaseURL string
	HTTP    *http.Client
}

// productServiceResponse mirrors the JSON shape returned by GET /products/:id.
// product-service serialises the model.Product struct, which uses "price" for
// the cents integer and "id" as a UUID string.
type productServiceResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"` // int cents — field name is "price", not "price_cents"
	Stock    int    `json:"stock"`
}

// GetProduct fetches a single product from product-service.
// Returns errProductNotFound on HTTP 404, a generic error for any other non-200.
func (p ProductServiceCatalog) GetProduct(ctx context.Context, id string) (Product, error) {
	u, err := url.JoinPath(p.BaseURL, "products", id)
	if err != nil {
		return Product{}, fmt.Errorf("product-service: build url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Product{}, err
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return Product{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Product{}, errProductNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Product{}, fmt.Errorf("product-service: unexpected status %d", resp.StatusCode)
	}

	var raw productServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Product{}, fmt.Errorf("product-service: decode: %w", err)
	}
	return Product{
		ID:         raw.ID,
		Name:       raw.Name,
		Category:   raw.Category,
		PriceCents: raw.Price,
		Stock:      raw.Stock,
	}, nil
}

// NopEmbeddingSource is the default EmbeddingSource used until product vectors
// are stored in Qdrant.  Products are currently not embedded — the ingestion
// service only creates a "documents" collection for PDFs.  compare_products
// handles errEmbeddingMissing gracefully by skipping the similarity pair rather
// than failing the whole call.
type NopEmbeddingSource struct{}

// Embedding always returns errEmbeddingMissing.  compare_products will skip
// the pairwise similarity step for any product whose embedding is absent.
func (NopEmbeddingSource) Embedding(_ context.Context, _ string) ([]float32, error) {
	return nil, errEmbeddingMissing
}
