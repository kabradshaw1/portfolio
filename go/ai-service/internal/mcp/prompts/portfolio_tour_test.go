package prompts

import (
	"context"
	"strings"
	"testing"
)

func TestPortfolioTourRendersTour(t *testing.T) {
	p := NewPortfolioTour()
	r, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.Join(messageTexts(r.Messages), " ")
	if !strings.Contains(combined, "runbook://how-this-portfolio-works") {
		t.Fatalf("expected runbook resource URI: %s", combined)
	}
	if !strings.Contains(combined, "schema://ecommerce") {
		t.Fatalf("expected schema resource URI: %s", combined)
	}
	if p.Name() != "tell-me-about-this-portfolio" {
		t.Fatalf("name: %s", p.Name())
	}
	if len(p.Arguments()) != 0 {
		t.Fatalf("expected no arguments, got %d", len(p.Arguments()))
	}
}
