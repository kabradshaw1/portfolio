package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// fakeEcommerce is a stand-in for *clients.EcommerceClient that satisfies ecommerceAPI.
type fakeEcommerce struct {
	products map[string]clients.Product
	listOut  []clients.Product
	listErr  error
}

func (f *fakeEcommerce) GetProduct(ctx context.Context, id string) (clients.Product, error) {
	p, ok := f.products[id]
	if !ok {
		return clients.Product{}, errors.New("not found")
	}
	return p, nil
}

func (f *fakeEcommerce) ListProducts(ctx context.Context, query string, limit int) ([]clients.Product, error) {
	return f.listOut, f.listErr
}

func TestGetProductTool_Success(t *testing.T) {
	fake := &fakeEcommerce{products: map[string]clients.Product{
		"p1": {ID: "p1", Name: "Waterproof Jacket", Price: 12999, Stock: 4},
	}}
	tool := NewGetProductTool(fake)

	args := json.RawMessage(`{"product_id":"p1"}`)
	res, err := tool.Call(context.Background(), args, "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m, ok := res.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected map content, got %T", res.Content)
	}
	if m["id"] != "p1" || m["name"] != "Waterproof Jacket" {
		t.Errorf("bad content: %+v", m)
	}
}

func TestGetProductTool_MissingArg(t *testing.T) {
	tool := NewGetProductTool(&fakeEcommerce{products: map[string]clients.Product{}})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing product_id")
	}
}

func TestSearchProductsTool_BoundsAndFilters(t *testing.T) {
	fake := &fakeEcommerce{listOut: []clients.Product{
		{ID: "p1", Name: "Waterproof Jacket", Price: 12999, Stock: 4},
		{ID: "p2", Name: "Rain Jacket", Price: 8900, Stock: 10},
		{ID: "p3", Name: "Expensive Jacket", Price: 50000, Stock: 1},
	}}
	tool := NewSearchProductsTool(fake)

	args := json.RawMessage(`{"query":"jacket","max_price":150}`)
	res, err := tool.Call(context.Background(), args, "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	items, ok := res.Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map content, got %T", res.Content)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 filtered results, got %d", len(items))
	}
	for _, it := range items {
		price := it["price"].(int)
		if price > 15000 {
			t.Errorf("result above max_price in cents: %+v", it)
		}
	}
}

func TestSearchProductsTool_MissingQuery(t *testing.T) {
	tool := NewSearchProductsTool(&fakeEcommerce{})
	_, err := tool.Call(context.Background(), json.RawMessage(`{}`), "")
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestCheckInventoryTool(t *testing.T) {
	fake := &fakeEcommerce{products: map[string]clients.Product{
		"p1": {ID: "p1", Name: "Waterproof Jacket", Price: 12999, Stock: 4},
		"p2": {ID: "p2", Name: "Rain Jacket", Price: 8900, Stock: 0},
	}}
	tool := NewCheckInventoryTool(fake)

	res, err := tool.Call(context.Background(), json.RawMessage(`{"product_id":"p1"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	m := res.Content.(map[string]any)
	if m["in_stock"] != true || m["stock"].(int) != 4 {
		t.Errorf("bad content: %+v", m)
	}

	res, err = tool.Call(context.Background(), json.RawMessage(`{"product_id":"p2"}`), "")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.Content.(map[string]any)["in_stock"] != false {
		t.Errorf("expected out of stock: %+v", res.Content)
	}
}
