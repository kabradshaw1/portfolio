package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/mcp/resources"
)

// CatalogResourceClient adapts product-service REST responses to MCP catalog
// resource shapes.
type CatalogResourceClient struct {
	baseURL string
	http    *http.Client
}

func NewCatalogResourceClient(baseURL string) *CatalogResourceClient {
	return &CatalogResourceClient{
		baseURL: baseURL,
		http:    &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}

func (c *CatalogResourceClient) Categories(ctx context.Context) ([]resources.CatalogCategory, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/categories", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("categories: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("categories: status %d: %s", resp.StatusCode, string(payload))
	}
	var body struct {
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode categories: %w", err)
	}
	out := make([]resources.CatalogCategory, 0, len(body.Categories))
	for _, category := range body.Categories {
		out = append(out, resources.CatalogCategory{Name: category})
	}
	return out, nil
}

func (c *CatalogResourceClient) Featured(ctx context.Context) ([]resources.CatalogProduct, error) {
	u, err := url.Parse(c.baseURL + "/products")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("limit", "5")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("featured products: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("featured products: status %d: %s", resp.StatusCode, string(payload))
	}
	var body struct {
		Products []productResourceDTO `json:"products"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode featured products: %w", err)
	}
	return mapCatalogProducts(body.Products), nil
}

func (c *CatalogResourceClient) Product(ctx context.Context, id string) (resources.CatalogProduct, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/products/"+url.PathEscape(id), nil)
	if err != nil {
		return resources.CatalogProduct{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return resources.CatalogProduct{}, fmt.Errorf("product %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resources.CatalogProduct{}, resources.ErrResourceNotFound
	}
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return resources.CatalogProduct{}, fmt.Errorf("product %s: status %d: %s", id, resp.StatusCode, string(payload))
	}
	var dto productResourceDTO
	if err := json.NewDecoder(resp.Body).Decode(&dto); err != nil {
		return resources.CatalogProduct{}, fmt.Errorf("decode product %s: %w", id, err)
	}
	return dto.toResource(), nil
}

type productResourceDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	Stock    int    `json:"stock"`
}

func (p productResourceDTO) toResource() resources.CatalogProduct {
	return resources.CatalogProduct{
		ID:         p.ID,
		Name:       p.Name,
		Category:   p.Category,
		PriceCents: p.Price,
		Stock:      p.Stock,
	}
}

func mapCatalogProducts(items []productResourceDTO) []resources.CatalogProduct {
	out := make([]resources.CatalogProduct, 0, len(items))
	for _, item := range items {
		out = append(out, item.toResource())
	}
	return out
}
