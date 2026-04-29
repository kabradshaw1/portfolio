package prompts

import (
	"context"
	"strings"
	"testing"
)

func TestCompareAndRecommendNoCategory(t *testing.T) {
	p := NewCompareAndRecommend()
	r, err := p.Render(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.Join(messageTexts(r.Messages), " ")
	if !strings.Contains(combined, "recommend_with_rationale") {
		t.Fatalf("expected recommend_with_rationale: %s", combined)
	}
	if !strings.Contains(combined, "compare_products") {
		t.Fatalf("expected compare_products: %s", combined)
	}
	if p.Name() != "compare-and-recommend" {
		t.Fatalf("name: %s", p.Name())
	}
}

func TestCompareAndRecommendWithCategory(t *testing.T) {
	p := NewCompareAndRecommend()
	r, err := p.Render(context.Background(), map[string]string{"category": "footwear"})
	if err != nil {
		t.Fatal(err)
	}
	combined := strings.Join(messageTexts(r.Messages), " ")
	if !strings.Contains(combined, "footwear") {
		t.Fatalf("expected footwear in messages: %s", combined)
	}
}

func TestCompareAndRecommendArgumentsAreOptional(t *testing.T) {
	p := NewCompareAndRecommend()
	args := p.Arguments()
	if len(args) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(args))
	}
	if args[0].Name != "category" || args[0].Required {
		t.Fatalf("expected optional category arg, got %+v", args[0])
	}
}
