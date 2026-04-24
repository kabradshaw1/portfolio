package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/jwtctx"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
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
	start := time.Now()
	var a listOrdersArgs
	_ = json.Unmarshal(args, &a) // empty args are fine
	limit := a.Limit
	if limit <= 0 || limit > maxListedOrders {
		limit = maxListedOrders
	}

	orders, err := t.api.ListOrders(ctx, jwtctx.FromContext(ctx))
	if err != nil {
		slog.Warn("list_orders failed", "tool", "list_orders", "error", err.Error())
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
	slog.Info("list_orders ok", "tool", "list_orders", "order_count", len(orders), "duration_ms", time.Since(start).Milliseconds())
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
	start := time.Now()
	var a getOrderArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("get_order: bad args: %w", err)
	}
	if a.OrderID == "" {
		return Result{}, errors.New("get_order: order_id is required")
	}

	o, err := t.api.GetOrder(ctx, jwtctx.FromContext(ctx), a.OrderID)
	if err != nil {
		slog.Warn("get_order failed", "tool", "get_order", "order_id", a.OrderID, "error", err.Error())
		return Result{}, fmt.Errorf("get_order: %w", err)
	}
	content := map[string]any{
		"id":         o.ID,
		"status":     o.Status,
		"total":      o.Total,
		"created_at": o.CreatedAt,
	}
	slog.Info("get_order ok", "tool", "get_order", "order_id", a.OrderID, "duration_ms", time.Since(start).Milliseconds())
	return Result{
		Content: content,
		Display: map[string]any{"kind": "order_card", "order": o},
	}, nil
}

// ---------- summarize_orders ----------

type summarizeOrdersTool struct {
	api ordersAPI
	llm llm.Client
}

// NewSummarizeOrdersTool builds a tool that lists the user's recent orders and
// asks a small sub-LLM call to summarize them. It reuses the parent turn's
// context so the agent's wall-clock timeout still covers the sub-call.
func NewSummarizeOrdersTool(api ordersAPI, llmc llm.Client) Tool {
	return &summarizeOrdersTool{api: api, llm: llmc}
}

func (t *summarizeOrdersTool) Name() string { return "summarize_orders" }
func (t *summarizeOrdersTool) Description() string {
	return "Summarize the current user's recent orders in plain English."
}
func (t *summarizeOrdersTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"period":{"type":"string","enum":["week","month","all"]}
		}
	}`)
}

type summarizeArgs struct {
	Period string `json:"period"`
}

func (t *summarizeOrdersTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("summarize_orders: authenticated user required")
	}
	start := time.Now()
	var a summarizeArgs
	_ = json.Unmarshal(args, &a)

	orders, err := t.api.ListOrders(ctx, jwtctx.FromContext(ctx))
	if err != nil {
		slog.Warn("summarize_orders list failed", "tool", "summarize_orders", "error", err.Error())
		return Result{}, fmt.Errorf("summarize_orders: %w", err)
	}
	if len(orders) > maxListedOrders {
		orders = orders[:maxListedOrders]
	}
	if len(orders) == 0 {
		out := map[string]any{"summary": "You have no orders yet."}
		return Result{Content: out, Display: out}, nil
	}

	orderJSON, _ := json.Marshal(orders)
	prompt := fmt.Sprintf(
		"Summarize these %d orders for the user in two or three sentences. "+
			"Totals are in cents — convert to dollars. Period requested: %q. Orders JSON: %s",
		len(orders), a.Period, string(orderJSON),
	)

	resp, err := t.llm.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}, nil)
	if err != nil {
		slog.Warn("summarize_orders sub-llm failed", "tool", "summarize_orders", "error", err.Error())
		return Result{}, fmt.Errorf("summarize_orders: sub-llm: %w", err)
	}
	slog.Info("summarize_orders ok", "tool", "summarize_orders", "order_count", len(orders), "duration_ms", time.Since(start).Milliseconds())
	out := map[string]any{"summary": resp.Content, "order_count": len(orders)}
	return Result{Content: out, Display: out}, nil
}
