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

type returnsAPI interface {
	InitiateReturn(ctx context.Context, jwt, orderID string, itemIDs []string, reason string) (clients.Return, error)
}

type initiateReturnTool struct{ api returnsAPI }

func NewInitiateReturnTool(api returnsAPI) Tool { return &initiateReturnTool{api: api} }

func (t *initiateReturnTool) Name() string { return "initiate_return" }
func (t *initiateReturnTool) Description() string {
	return "Initiate a return for specific items on one of the current user's orders. Requires the order id, a non-empty list of item ids, and a short reason."
}
func (t *initiateReturnTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"order_id":{"type":"string"},
			"item_ids":{"type":"array","items":{"type":"string"},"minItems":1},
			"reason":{"type":"string"}
		},
		"required":["order_id","item_ids","reason"]
	}`)
}

type initiateReturnArgs struct {
	OrderID string   `json:"order_id"`
	ItemIDs []string `json:"item_ids"`
	Reason  string   `json:"reason"`
}

func (t *initiateReturnTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	if userID == "" {
		return Result{}, errors.New("initiate_return: authenticated user required")
	}
	start := time.Now()
	var a initiateReturnArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("initiate_return: bad args: %w", err)
	}
	if a.OrderID == "" {
		return Result{}, errors.New("initiate_return: order_id is required")
	}
	if len(a.ItemIDs) == 0 {
		return Result{}, errors.New("initiate_return: item_ids must be non-empty")
	}
	if a.Reason == "" {
		return Result{}, errors.New("initiate_return: reason is required")
	}

	ret, err := t.api.InitiateReturn(ctx, jwtctx.FromContext(ctx), a.OrderID, a.ItemIDs, a.Reason)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "initiate_return", "order_id", a.OrderID, "error", err.Error())
		return Result{}, fmt.Errorf("initiate_return: %w", err)
	}
	slog.InfoContext(ctx, "tool result", "tool", "initiate_return", "order_id", a.OrderID, "item_count", len(a.ItemIDs), "duration_ms", time.Since(start).Milliseconds())
	content := map[string]any{
		"id":       ret.ID,
		"order_id": ret.OrderID,
		"status":   ret.Status,
		"reason":   ret.Reason,
	}
	return Result{Content: content, Display: map[string]any{"kind": "return_confirmation", "return": ret}}, nil
}
