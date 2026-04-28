package composite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

// Verdict is the structured output of investigate_my_order.
type Verdict struct {
	Stage           string   `json:"stage"`
	Status          string   `json:"status"`
	CustomerMessage string   `json:"customer_message"`
	TechnicalDetail string   `json:"technical_detail"`
	NextAction      string   `json:"next_action"`
	Evidence        Evidence `json:"evidence"`
}

// Evidence records the data sources consulted to produce the verdict.
type Evidence struct {
	TraceID  string   `json:"trace_id"`
	SagaStep string   `json:"saga_step"`
	Partial  bool     `json:"partial_evidence"`
	LastLogs []string `json:"last_logs"`
}

// ComputeVerdict reduces an EvidenceBundle to a Verdict.
// The mapping encodes the customer-facing interpretation of the saga state machine:
// completed orders return "ok", in-flight ones return "retrying" with a saga-step
// specific message, and failed orders surface a contact-support next action.
func ComputeVerdict(b EvidenceBundle) Verdict {
	v := Verdict{
		Evidence: Evidence{
			TraceID:  b.Order.TraceID,
			SagaStep: b.Saga.Step,
			Partial:  b.Partial,
			LastLogs: b.Logs,
		},
	}

	switch b.Order.Status {
	case "completed":
		v.Stage = "completed"
		v.Status = "ok"
		v.CustomerMessage = "Your order is complete."
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "none"
	case "processing":
		v.Stage = b.Saga.Step
		v.Status = "retrying"
		switch b.Saga.Step {
		case "payment_captured":
			v.CustomerMessage = "Your payment cleared and we're handing off to the warehouse."
		case "warehouse_pending":
			v.CustomerMessage = "Warehouse hasn't acknowledged yet — we're retrying."
		default:
			v.CustomerMessage = "Your order is in progress."
		}
		v.TechnicalDetail = "saga=" + b.Saga.Step
		v.NextAction = "wait"
	case "failed":
		v.Stage = "failed"
		v.Status = "failed"
		v.CustomerMessage = "Your order didn't go through. Please contact support."
		v.TechnicalDetail = "saga=" + b.Saga.Step + " retries=" + strconv.Itoa(b.Saga.Retries)
		v.NextAction = "contact_support"
	default:
		v.Stage = b.Order.Status
		v.Status = "stalled"
		v.CustomerMessage = "Your order is in an unusual state."
		v.NextAction = "contact_support"
	}
	return v
}

// investigateMyOrderTool implements tools.Tool, wrapping EvidenceFetcher as an
// MCP-registerable tool that runs parallel fan-out and reduces to a customer-facing verdict.
type investigateMyOrderTool struct {
	fetcher EvidenceFetcher
}

// compile-time assertion — fails at build if the interface contract drifts.
var _ tools.Tool = (*investigateMyOrderTool)(nil)

// NewInvestigateMyOrderTool wraps an EvidenceFetcher as an MCP-registerable tool.
// It runs the parallel fan-out and reduces the bundle to a customer-facing verdict.
func NewInvestigateMyOrderTool(f EvidenceFetcher) *investigateMyOrderTool {
	return &investigateMyOrderTool{fetcher: f}
}

func (t *investigateMyOrderTool) Name() string {
	return "investigate_my_order"
}

func (t *investigateMyOrderTool) Description() string {
	return "Investigates the full checkout saga for a given order, correlating order, payment, cart reservation, RabbitMQ events, trace, and logs into a structured verdict with a customer-facing message."
}

func (t *investigateMyOrderTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"order_id": { "type": "string", "description": "The order id to investigate." }
		},
		"required": ["order_id"]
	}`)
}

func (t *investigateMyOrderTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	start := time.Now()
	var req struct {
		OrderID string `json:"order_id"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "investigate_my_order", "error", err.Error())
		return tools.Result{}, fmt.Errorf("investigate_my_order: invalid args: %w", err)
	}
	if req.OrderID == "" {
		return tools.Result{}, errors.New("investigate_my_order: order_id is required")
	}
	// TODO(auth): verify order.UserID == userID once source adapters are wired (A5).
	_ = userID
	bundle, err := t.fetcher.Fetch(ctx, req.OrderID)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "investigate_my_order", "order_id", req.OrderID, "error", err.Error())
		return tools.Result{}, fmt.Errorf("investigate_my_order: %w", err)
	}
	verdict := ComputeVerdict(bundle)
	slog.InfoContext(ctx, "tool result", "tool", "investigate_my_order",
		"order_id", req.OrderID,
		"stage", verdict.Stage,
		"status", verdict.Status,
		"partial", verdict.Evidence.Partial,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return tools.Result{Content: verdict}, nil
}
