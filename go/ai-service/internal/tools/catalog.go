package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/kafka"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// ecommerceAPI is the subset of the ecommerce HTTP client the catalog tools use.
// Kept as an interface so tests can swap in a fake.
type ecommerceAPI interface {
	GetProduct(ctx context.Context, id string) (clients.Product, error)
	ListProducts(ctx context.Context, query string, limit int) ([]clients.Product, error)
}

// -------- get_product --------

type getProductTool struct {
	api       ecommerceAPI
	kafkaPub  kafka.Producer
}

func NewGetProductTool(api ecommerceAPI, opts ...kafka.Producer) Tool {
	t := &getProductTool{api: api}
	if len(opts) > 0 {
		t.kafkaPub = opts[0]
	}
	return t
}

func (t *getProductTool) Name() string        { return "get_product" }
func (t *getProductTool) Description() string { return "Fetch the full details of one product by id." }
func (t *getProductTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"product_id":{"type":"string","description":"Opaque product id."}
		},
		"required":["product_id"]
	}`)
}

type getProductArgs struct {
	ProductID string `json:"product_id"`
}

func (t *getProductTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	var a getProductArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("get_product: bad args: %w", err)
	}
	if a.ProductID == "" {
		return Result{}, errors.New("get_product: product_id is required")
	}
	p, err := t.api.GetProduct(ctx, a.ProductID)
	if err != nil {
		return Result{}, fmt.Errorf("get_product: %w", err)
	}
	if t.kafkaPub != nil {
		kafka.SafePublish(ctx, t.kafkaPub, "ecommerce.views", p.ID, kafka.Event{
			Type: "product.viewed",
			Data: map[string]any{"productID": p.ID, "productName": p.Name, "source": "detail"},
		})
	}
	return Result{
		Content: map[string]any{"id": p.ID, "name": p.Name, "price": p.Price, "stock": p.Stock},
		Display: map[string]any{"kind": "product_card", "product": p},
	}, nil
}

// -------- search_products --------

type searchProductsTool struct {
	api      ecommerceAPI
	kafkaPub kafka.Producer
}

func NewSearchProductsTool(api ecommerceAPI, opts ...kafka.Producer) Tool {
	t := &searchProductsTool{api: api}
	if len(opts) > 0 {
		t.kafkaPub = opts[0]
	}
	return t
}

func (t *searchProductsTool) Name() string { return "search_products" }
func (t *searchProductsTool) Description() string {
	return "Search the product catalog by free-text query. Optional max_price in dollars (e.g. 150 for $150). Returns at most 10 results."
}
func (t *searchProductsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","description":"Free-text product query."},
			"max_price":{"type":"number","description":"Optional upper bound on price."},
			"limit":{"type":"integer","description":"Max results to return (cap 10)."}
		},
		"required":["query"]
	}`)
}

type searchArgs struct {
	Query    string  `json:"query"`
	MaxPrice float64 `json:"max_price"`
	Limit    int     `json:"limit"`
}

const maxSearchResults = 10

func (t *searchProductsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	var a searchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("search_products: bad args: %w", err)
	}
	if a.Query == "" {
		return Result{}, errors.New("search_products: query is required")
	}
	limit := a.Limit
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}

	prods, err := t.api.ListProducts(ctx, a.Query, limit)
	if err != nil {
		return Result{}, fmt.Errorf("search_products: %w", err)
	}

	out := make([]map[string]any, 0, len(prods))
	for _, p := range prods {
		if a.MaxPrice > 0 && p.Price > int(a.MaxPrice*100) {
			continue
		}
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "price": p.Price, "stock": p.Stock,
		})
		if len(out) >= limit {
			break
		}
	}
	if t.kafkaPub != nil {
		for _, p := range prods {
			kafka.SafePublish(ctx, t.kafkaPub, "ecommerce.views", p.ID, kafka.Event{
				Type: "product.viewed",
				Data: map[string]any{"productID": p.ID, "productName": p.Name, "source": "search"},
			})
		}
	}
	return Result{
		Content: out,
		Display: map[string]any{"kind": "product_list", "products": out},
	}, nil
}

// -------- check_inventory --------

type checkInventoryTool struct {
	api ecommerceAPI
}

func NewCheckInventoryTool(api ecommerceAPI) Tool { return &checkInventoryTool{api: api} }

func (t *checkInventoryTool) Name() string { return "check_inventory" }
func (t *checkInventoryTool) Description() string {
	return "Check whether a product is in stock. Returns stock count and a boolean."
}
func (t *checkInventoryTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"product_id":{"type":"string"}
		},
		"required":["product_id"]
	}`)
}

func (t *checkInventoryTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	var a getProductArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("check_inventory: bad args: %w", err)
	}
	if a.ProductID == "" {
		return Result{}, errors.New("check_inventory: product_id is required")
	}
	p, err := t.api.GetProduct(ctx, a.ProductID)
	if err != nil {
		return Result{}, fmt.Errorf("check_inventory: %w", err)
	}
	content := map[string]any{
		"product_id": p.ID,
		"stock":      p.Stock,
		"in_stock":   p.Stock > 0,
	}
	return Result{
		Content: content,
		Display: map[string]any{"kind": "inventory", "product_id": p.ID, "stock": p.Stock, "in_stock": p.Stock > 0},
	}, nil
}
