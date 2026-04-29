package prompts

import (
	"context"
	"strings"
	"testing"
)

func TestExplainMyOrderRequiresOrderID(t *testing.T) {
	p := NewExplainMyOrder()
	_, err := p.Render(context.Background(), map[string]string{})
	if err == nil {
		t.Fatalf("expected error for missing order_id")
	}
}

func TestExplainMyOrderEmptyOrderID(t *testing.T) {
	p := NewExplainMyOrder()
	_, err := p.Render(context.Background(), map[string]string{"order_id": ""})
	if err == nil {
		t.Fatalf("expected error for empty order_id")
	}
}

func TestExplainMyOrderRendersWithOrderID(t *testing.T) {
	p := NewExplainMyOrder()
	r, err := p.Render(context.Background(), map[string]string{"order_id": "ord1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Messages) == 0 {
		t.Fatalf("no messages")
	}
	combined := strings.Join(messageTexts(r.Messages), " ")
	if !strings.Contains(combined, "ord1") {
		t.Fatalf("expected order_id in messages: %s", combined)
	}
	if !strings.Contains(combined, "investigate_my_order") {
		t.Fatalf("expected mention of investigate_my_order tool: %s", combined)
	}
	if p.Name() != "explain-my-order" {
		t.Fatalf("name: %s", p.Name())
	}
	args := p.Arguments()
	if len(args) != 1 || args[0].Name != "order_id" || !args[0].Required {
		t.Fatalf("args: %+v", args)
	}
}

func messageTexts(ms []Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Text
	}
	return out
}
