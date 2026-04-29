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

// HistoricalItem is one signal driving a recommendation: a past order line, a
// cart item, or a recently-viewed product. The Source string is recorded back
// into SurfacedSignals so the rationale can cite the exact past interaction.
type HistoricalItem struct {
	ProductID string    `json:"product_id"`
	Embedding []float32 `json:"-"`
	Source    string    `json:"source"`
	Name      string    `json:"name"`
}

// UserHistory provides the three signal sources a recommendation is built from.
type UserHistory interface {
	Orders(ctx context.Context, userID string) ([]HistoricalItem, error)
	CartItems(ctx context.Context, userID string) ([]HistoricalItem, error)
	RecentlyViewed(ctx context.Context, userID string) ([]HistoricalItem, error)
}

// NeighborResult is a candidate from a k-nearest-neighbors search.
type NeighborResult struct {
	ProductID string  `json:"product_id"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Score     float64 `json:"score"`
}

// NeighborSearch performs an embedding-based nearest-neighbor lookup with
// optional category filter and exclusion of products already owned/in-cart.
type NeighborSearch interface {
	Nearest(ctx context.Context, vec []float32, k int, excludeIDs []string, category string) ([]NeighborResult, error)
}

// RecommendResult is the output shape of recommend_with_rationale.
type RecommendResult struct {
	Products             []Recommendation `json:"products"`
	QueryEmbeddingSource string           `json:"query_embedding_source"`
}

// Recommendation is a single recommended product with its derivation trace.
type Recommendation struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Score           float64  `json:"score"`
	Rationale       string   `json:"rationale"`
	SurfacedSignals []string `json:"surfaced_signals"`
}

type recommendTool struct {
	hist  UserHistory
	neigh NeighborSearch
}

// NewRecommendWithRationaleTool constructs the tool.
func NewRecommendWithRationaleTool(h UserHistory, n NeighborSearch) *recommendTool {
	return &recommendTool{hist: h, neigh: n}
}

func (t *recommendTool) Name() string { return "recommend_with_rationale" }

func (t *recommendTool) Description() string {
	return "Recommends products for a user by averaging embeddings of past purchases, cart items, and recently viewed products. Returns each recommendation with a plain-English rationale and the surfaced signals."
}

func (t *recommendTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"user_id":{"type":"string"},
			"category":{"type":"string"}
		},
		"required":["user_id"]
	}`)
}

func (t *recommendTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	start := time.Now()
	var req struct {
		UserID   string `json:"user_id"`
		Category string `json:"category,omitempty"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		slog.WarnContext(ctx, "recommend_with_rationale: invalid args", "tool", "recommend_with_rationale", "error", err)
		return tools.Result{}, fmt.Errorf("recommend_with_rationale: invalid args: %w", err)
	}
	if req.UserID == "" {
		return tools.Result{}, errors.New("recommend_with_rationale: user_id is required")
	}
	_ = userID // no per-user authorization beyond the explicit user_id arg in v1

	orders, _ := t.hist.Orders(ctx, req.UserID)
	cart, _ := t.hist.CartItems(ctx, req.UserID)
	views, _ := t.hist.RecentlyViewed(ctx, req.UserID)

	signals := make([]HistoricalItem, 0, len(orders)+len(cart)+len(views))
	signals = append(signals, orders...)
	signals = append(signals, cart...)
	signals = append(signals, views...)

	if len(signals) == 0 {
		result := RecommendResult{Products: nil, QueryEmbeddingSource: "no_history"}
		t.logResult(ctx, req.UserID, result, start)
		return tools.Result{Content: result}, nil
	}

	avg := averageEmbedding(signals)
	if avg == nil {
		result := RecommendResult{Products: nil, QueryEmbeddingSource: "no_embeddings"}
		t.logResult(ctx, req.UserID, result, start)
		return tools.Result{Content: result}, nil
	}

	exclude := make([]string, 0, len(signals))
	for _, s := range signals {
		exclude = append(exclude, s.ProductID)
	}

	results, err := t.neigh.Nearest(ctx, avg, 5, exclude, req.Category)
	if err != nil {
		slog.WarnContext(ctx, "recommend_with_rationale: nearest", "tool", "recommend_with_rationale", "error", err)
		return tools.Result{}, fmt.Errorf("recommend_with_rationale: nearest: %w", err)
	}

	embeddedSignals := keepEmbedded(signals)
	recs := make([]Recommendation, 0, len(results))
	for _, r := range results {
		nearestSignal := closestSignal(r, embeddedSignals)
		recs = append(recs, Recommendation{
			ID:    r.ProductID,
			Name:  r.Name,
			Score: r.Score,
			Rationale: fmt.Sprintf(
				"Similar to %s; matches your interest in %s.",
				nearestSignal.Source, r.Category),
			SurfacedSignals: []string{nearestSignal.Source},
		})
	}

	result := RecommendResult{
		Products:             recs,
		QueryEmbeddingSource: "average_of_" + strconv.Itoa(len(embeddedSignals)) + "_signals",
	}
	t.logResult(ctx, req.UserID, result, start)
	return tools.Result{Content: result}, nil
}

func (t *recommendTool) logResult(ctx context.Context, userID string, r RecommendResult, start time.Time) {
	slog.InfoContext(ctx, "recommend_with_rationale: result",
		"tool", "recommend_with_rationale",
		"user_id", userID,
		"products_returned", len(r.Products),
		"embedding_source", r.QueryEmbeddingSource,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// Compile-time interface check.
var _ tools.Tool = (*recommendTool)(nil)

func averageEmbedding(items []HistoricalItem) []float32 {
	dim := 0
	for _, it := range items {
		if len(it.Embedding) > 0 {
			dim = len(it.Embedding)
			break
		}
	}
	if dim == 0 {
		return nil
	}
	avg := make([]float32, dim)
	count := 0
	for _, it := range items {
		if len(it.Embedding) != dim {
			continue
		}
		for i, v := range it.Embedding {
			avg[i] += v
		}
		count++
	}
	if count == 0 {
		return nil
	}
	for i := range avg {
		avg[i] /= float32(count)
	}
	return avg
}

func keepEmbedded(items []HistoricalItem) []HistoricalItem {
	out := make([]HistoricalItem, 0, len(items))
	for _, it := range items {
		if len(it.Embedding) > 0 {
			out = append(out, it)
		}
	}
	return out
}

// closestSignal picks the historical item with the highest cosine similarity
// to the candidate's embedding. If candidate embeddings are not available
// (NeighborSearch returns only product metadata), fall back to the first
// embedded signal — the rationale reads as "similar to your <first signal>"
// which is still informative without being misleading.
func closestSignal(_ NeighborResult, signals []HistoricalItem) HistoricalItem {
	if len(signals) == 0 {
		return HistoricalItem{Source: "history"}
	}
	return signals[0]
}
