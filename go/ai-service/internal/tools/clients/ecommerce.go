package clients

import (
	"bytes"
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

// ---------- user-scoped types ----------

type Order struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Total     int       `json:"total"` // cents
	CreatedAt time.Time `json:"createdAt"`
}

type ordersResponse struct {
	Orders []Order `json:"orders"`
}

type CartItem struct {
	ID           string `json:"id"`
	ProductID    string `json:"productId"`
	ProductName  string `json:"productName"`
	ProductPrice int    `json:"productPrice"` // cents
	Quantity     int    `json:"quantity"`
}

type Cart struct {
	Items []CartItem `json:"items"`
	Total int        `json:"total"` // cents
}

type Return struct {
	ID      string `json:"id"`
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

// ---------- authenticated helpers ----------

func (c *EcommerceClient) authedRequest(ctx context.Context, method, path string, body io.Reader, jwt string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	return req, nil
}

func (c *EcommerceClient) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(payload))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---------- user-scoped methods ----------

func (c *EcommerceClient) ListOrders(ctx context.Context, jwt string) ([]Order, error) {
	req, err := c.authedRequest(ctx, http.MethodGet, "/orders", nil, jwt)
	if err != nil {
		return nil, err
	}
	var envelope ordersResponse
	if err := c.doJSON(req, &envelope); err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	return envelope.Orders, nil
}

func (c *EcommerceClient) GetOrder(ctx context.Context, jwt, orderID string) (Order, error) {
	req, err := c.authedRequest(ctx, http.MethodGet, "/orders/"+url.PathEscape(orderID), nil, jwt)
	if err != nil {
		return Order{}, err
	}
	var o Order
	if err := c.doJSON(req, &o); err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}
	return o, nil
}

func (c *EcommerceClient) GetCart(ctx context.Context, jwt string) (Cart, error) {
	req, err := c.authedRequest(ctx, http.MethodGet, "/cart", nil, jwt)
	if err != nil {
		return Cart{}, err
	}
	var cart Cart
	if err := c.doJSON(req, &cart); err != nil {
		return Cart{}, fmt.Errorf("get cart: %w", err)
	}
	return cart, nil
}

func (c *EcommerceClient) AddToCart(ctx context.Context, jwt, productID string, qty int) (CartItem, error) {
	body, _ := json.Marshal(map[string]any{"productId": productID, "quantity": qty})
	req, err := c.authedRequest(ctx, http.MethodPost, "/cart", bytes.NewReader(body), jwt)
	if err != nil {
		return CartItem{}, err
	}
	var item CartItem
	if err := c.doJSON(req, &item); err != nil {
		return CartItem{}, fmt.Errorf("add to cart: %w", err)
	}
	return item, nil
}

func (c *EcommerceClient) InitiateReturn(ctx context.Context, jwt, orderID string, itemIDs []string, reason string) (Return, error) {
	body, _ := json.Marshal(map[string]any{
		"itemIds": itemIDs,
		"reason":  reason,
	})
	req, err := c.authedRequest(ctx, http.MethodPost, "/orders/"+url.PathEscape(orderID)+"/returns", bytes.NewReader(body), jwt)
	if err != nil {
		return Return{}, err
	}
	var ret Return
	if err := c.doJSON(req, &ret); err != nil {
		return Return{}, fmt.Errorf("initiate return: %w", err)
	}
	return ret, nil
}
