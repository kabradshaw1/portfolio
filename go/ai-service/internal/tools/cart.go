package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// cartAPI is the subset of the ecommerce client the cart tools use.
type cartAPI interface {
	GetCart(ctx context.Context, jwt string) (clients.Cart, error)
	AddToCart(ctx context.Context, jwt, productID string, qty int) (clients.CartItem, error)
}

// ---------- view_cart ----------

type viewCartTool struct{ api cartAPI }

func NewViewCartTool(api cartAPI) Tool { return &viewCartTool{api: api} }

func (t *viewCartTool) Name() string        { return "view_cart" }
func (t *viewCartTool) Description() string { return "Return the current user's shopping cart with items and total in cents." }
func (t *viewCartTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *viewCartTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("view_cart: authenticated user required")
	}
	start := time.Now()
	cart, err := t.api.GetCart(ctx, jwtctx.FromContext(ctx))
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "view_cart", "error", err.Error())
		return Result{}, fmt.Errorf("view_cart: %w", err)
	}
	slog.InfoContext(ctx, "tool result", "tool", "view_cart", "item_count", len(cart.Items), "duration_ms", time.Since(start).Milliseconds())
	content := map[string]any{
		"items": cart.Items,
		"total": cart.Total,
	}
	return Result{Content: content, Display: map[string]any{"kind": "cart", "cart": cart}}, nil
}

// ---------- add_to_cart ----------

type addToCartTool struct{ api cartAPI }

func NewAddToCartTool(api cartAPI) Tool { return &addToCartTool{api: api} }

func (t *addToCartTool) Name() string { return "add_to_cart" }
func (t *addToCartTool) Description() string {
	return "Add a product to the current user's cart. Quantity must be a positive integer."
}
func (t *addToCartTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"product_id":{"type":"string"},
			"qty":{"type":"integer","minimum":1}
		},
		"required":["product_id","qty"]
	}`)
}

type addToCartArgs struct {
	ProductID string `json:"product_id"`
	Qty       int    `json:"qty"`
}

func (t *addToCartTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("add_to_cart: authenticated user required")
	}
	start := time.Now()
	var a addToCartArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("add_to_cart: bad args: %w", err)
	}
	if a.ProductID == "" {
		return Result{}, errors.New("add_to_cart: product_id is required")
	}
	if a.Qty <= 0 {
		return Result{}, errors.New("add_to_cart: qty must be positive")
	}

	item, err := t.api.AddToCart(ctx, jwtctx.FromContext(ctx), a.ProductID, a.Qty)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "add_to_cart", "product_id", a.ProductID, "error", err.Error())
		return Result{}, fmt.Errorf("add_to_cart: %w", err)
	}
	slog.InfoContext(ctx, "tool result", "tool", "add_to_cart", "product_id", a.ProductID, "quantity", a.Qty, "duration_ms", time.Since(start).Milliseconds())
	content := map[string]any{
		"id":         item.ID,
		"product_id": item.ProductID,
		"name":       item.ProductName,
		"price":      item.ProductPrice,
		"quantity":   item.Quantity,
	}
	return Result{Content: content, Display: map[string]any{"kind": "cart_item", "item": item}}, nil
}
