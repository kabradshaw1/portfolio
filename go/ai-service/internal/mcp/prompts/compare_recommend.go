package prompts

import (
	"context"
	"fmt"
)

type compareAndRecommend struct{}

// NewCompareAndRecommend returns the compare-and-recommend prompt: chains
// recommend_with_rationale then compare_products on the top results.
func NewCompareAndRecommend() Prompt { return compareAndRecommend{} }

func (compareAndRecommend) Name() string { return "compare-and-recommend" }
func (compareAndRecommend) Description() string {
	return "Recommends products for the user, then compares the top three so the user sees the trade-offs."
}
func (compareAndRecommend) Arguments() []Argument {
	return []Argument{{Name: "category", Description: "Optional category filter, e.g. footwear.", Required: false}}
}
func (compareAndRecommend) Render(_ context.Context, args map[string]string) (Rendered, error) {
	category := args["category"]

	userText := "Use recommend_with_rationale to suggest products for me, then call compare_products on the top three product ids and summarize the trade-offs."
	if category != "" {
		userText = fmt.Sprintf("Use recommend_with_rationale with category=%q to suggest products for me, then call compare_products on the top three product ids and summarize the trade-offs.", category)
	}

	return Rendered{
		Description: "Recommend, then compare the top results.",
		Messages: []Message{
			{Role: "system", Text: "You are a helpful shopping assistant. Surface the rationale from each recommendation in your final summary."},
			{Role: "user", Text: userText},
		},
	}, nil
}
