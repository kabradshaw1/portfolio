package prompts

import (
	"context"
	"errors"
	"fmt"
)

type explainMyOrder struct{}

// NewExplainMyOrder returns the explain-my-order prompt: wraps the
// investigate_my_order tool in a customer-friendly framing.
func NewExplainMyOrder() Prompt { return explainMyOrder{} }

func (explainMyOrder) Name() string { return "explain-my-order" }
func (explainMyOrder) Description() string {
	return "Explains the current state of an order in plain language by walking the saga."
}
func (explainMyOrder) Arguments() []Argument {
	return []Argument{{Name: "order_id", Description: "The order id.", Required: true}}
}
func (explainMyOrder) Render(_ context.Context, args map[string]string) (Rendered, error) {
	id := args["order_id"]
	if id == "" {
		return Rendered{}, errors.New("explain-my-order: order_id is required")
	}
	return Rendered{
		Description: "Walk the checkout saga for this order and explain it to the customer.",
		Messages: []Message{
			{Role: "system", Text: "You are a helpful assistant explaining order status to a customer. Be specific about the saga stage. Avoid jargon."},
			{Role: "user", Text: fmt.Sprintf("Use the investigate_my_order tool with order_id=%s, then explain in 2-3 sentences what's happening with my order. If the verdict says the order is stalled, suggest the next action.", id)},
		},
	}, nil
}
