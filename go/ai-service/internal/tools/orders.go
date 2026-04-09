package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools/clients"
)

// ordersAPI is the subset of the ecommerce client the order tools use.
type ordersAPI interface {
	ListOrders(ctx context.Context, jwt string) ([]clients.Order, error)
	GetOrder(ctx context.Context, jwt, orderID string) (clients.Order, error)
}

const maxListedOrders = 20

// ---------- list_orders ----------

type listOrdersTool struct{ api ordersAPI }

func NewListOrdersTool(api ordersAPI) Tool { return &listOrdersTool{api: api} }

func (t *listOrdersTool) Name() string { return "list_orders" }
func (t *listOrdersTool) Description() string {
	return "List the current user's orders. Returns at most 20 most recent orders with id, status, total in cents, and creation date."
}
func (t *listOrdersTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"limit":{"type":"integer","description":"Max orders to return (cap 20)."}
		}
	}`)
}

type listOrdersArgs struct {
	Limit int `json:"limit"`
}

func (t *listOrdersTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("list_orders: authenticated user required")
	}
	var a listOrdersArgs
	_ = json.Unmarshal(args, &a) // empty args are fine
	limit := a.Limit
	if limit <= 0 || limit > maxListedOrders {
		limit = maxListedOrders
	}

	orders, err := t.api.ListOrders(ctx, jwtctx.FromContext(ctx))
	if err != nil {
		return Result{}, fmt.Errorf("list_orders: %w", err)
	}
	if len(orders) > limit {
		orders = orders[:limit]
	}

	out := make([]map[string]any, 0, len(orders))
	for _, o := range orders {
		out = append(out, map[string]any{
			"id":         o.ID,
			"status":     o.Status,
			"total":      o.Total,
			"created_at": o.CreatedAt,
		})
	}
	return Result{
		Content: out,
		Display: map[string]any{"kind": "order_list", "orders": out},
	}, nil
}

// ---------- get_order ----------

type getOrderTool struct{ api ordersAPI }

func NewGetOrderTool(api ordersAPI) Tool { return &getOrderTool{api: api} }

func (t *getOrderTool) Name() string { return "get_order" }
func (t *getOrderTool) Description() string {
	return "Fetch one order by id for the current user. Returns id, status, total, and creation date."
}
func (t *getOrderTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"order_id":{"type":"string"}
		},
		"required":["order_id"]
	}`)
}

type getOrderArgs struct {
	OrderID string `json:"order_id"`
}

func (t *getOrderTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("get_order: authenticated user required")
	}
	var a getOrderArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("get_order: bad args: %w", err)
	}
	if a.OrderID == "" {
		return Result{}, errors.New("get_order: order_id is required")
	}

	o, err := t.api.GetOrder(ctx, jwtctx.FromContext(ctx), a.OrderID)
	if err != nil {
		return Result{}, fmt.Errorf("get_order: %w", err)
	}
	content := map[string]any{
		"id":         o.ID,
		"status":     o.Status,
		"total":      o.Total,
		"created_at": o.CreatedAt,
	}
	return Result{
		Content: content,
		Display: map[string]any{"kind": "order_card", "order": o},
	}, nil
}
