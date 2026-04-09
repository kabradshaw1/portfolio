package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Product is the subset of ecommerce-service's product representation that ai-service needs.
// Price is in cents (matches ecommerce-service's model.Product).
type Product struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
	Stock int    `json:"stock"`
}

// listResponse mirrors ecommerce-service's model.ProductListResponse envelope.
type listResponse struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	Limit    int       `json:"limit"`
}

type EcommerceClient struct {
	baseURL string
	http    *http.Client
}

func NewEcommerceClient(baseURL string) *EcommerceClient {
	return &EcommerceClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *EcommerceClient) GetProduct(ctx context.Context, id string) (Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/products/"+url.PathEscape(id), nil)
	if err != nil {
		return Product{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Product{}, fmt.Errorf("get product: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return Product{}, fmt.Errorf("get product %s: status %d: %s", id, resp.StatusCode, string(payload))
	}
	var p Product
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return Product{}, fmt.Errorf("decode product: %w", err)
	}
	return p, nil
}

// ListProducts does a text search via ecommerce-service's /products endpoint.
// The endpoint returns a ProductListResponse envelope; we unwrap to []Product.
func (c *EcommerceClient) ListProducts(ctx context.Context, query string, limit int) ([]Product, error) {
	u, err := url.Parse(c.baseURL + "/products")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if query != "" {
		q.Set("q", query)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list products: status %d: %s", resp.StatusCode, string(payload))
	}
	var out listResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode product list: %w", err)
	}
	return out.Products, nil
}
