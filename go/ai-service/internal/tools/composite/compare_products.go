package composite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/tools"
)

var (
	errProductNotFound  = errors.New("product not found")
	errEmbeddingMissing = errors.New("embedding missing")
)

// Product is the structural view of a product needed for comparison.
type Product struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Category   string `json:"category"`
	PriceCents int    `json:"price_cents"`
	Stock      int    `json:"stock"`
}

// ProductCatalog returns the structural data for a product.
type ProductCatalog interface {
	GetProduct(ctx context.Context, id string) (Product, error)
}

// EmbeddingSource returns the vector embedding for a product, used for
// semantic similarity. Missing embeddings are not fatal — the tool simply
// omits the corresponding similarity entry.
type EmbeddingSource interface {
	Embedding(ctx context.Context, productID string) ([]float32, error)
}

// CompareResult is the output shape of compare_products.
type CompareResult struct {
	Products       []Product            `json:"products"`
	Shared         map[string]string    `json:"shared_attributes"`
	Differing      []DifferingAttribute `json:"differing_attributes"`
	Similarity     []PairSimilarity     `json:"semantic_similarity"`
	Recommendation string               `json:"recommendation"`
}

// DifferingAttribute records a structural attribute that differs across products.
type DifferingAttribute struct {
	Field  string            `json:"field"`
	Values map[string]string `json:"values"`
}

// PairSimilarity is the cosine similarity between two products by id.
type PairSimilarity struct {
	Pair  [2]string `json:"pair"`
	Score float64   `json:"score"`
}

type compareProductsTool struct {
	catalog ProductCatalog
	embed   EmbeddingSource
}

// NewCompareProductsTool constructs the tool.
func NewCompareProductsTool(c ProductCatalog, e EmbeddingSource) *compareProductsTool {
	return &compareProductsTool{catalog: c, embed: e}
}

func (t *compareProductsTool) Name() string { return "compare_products" }

func (t *compareProductsTool) Description() string {
	return "Compares two or more products structurally and semantically. Returns shared and differing attributes, pairwise embedding similarity, and a short recommendation."
}

func (t *compareProductsTool) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"product_ids":{"type":"array","items":{"type":"string"},"minItems":2,"maxItems":5}
		},
		"required":["product_ids"]
	}`)
}

func (t *compareProductsTool) Call(ctx context.Context, args json.RawMessage, userID string) (tools.Result, error) {
	start := time.Now()
	var req struct {
		ProductIDs []string `json:"product_ids"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		slog.WarnContext(ctx, "compare_products: invalid args", "tool", "compare_products", "error", err)
		return tools.Result{}, fmt.Errorf("compare_products: invalid args: %w", err)
	}
	if len(req.ProductIDs) < 2 {
		return tools.Result{}, errors.New("compare_products: at least 2 product_ids required")
	}
	if len(req.ProductIDs) > 5 {
		return tools.Result{}, errors.New("compare_products: at most 5 product_ids supported")
	}
	_ = userID // tool is read-only; per-user authorization not required

	products := make([]Product, 0, len(req.ProductIDs))
	embeddings := make(map[string][]float32, len(req.ProductIDs))
	for _, id := range req.ProductIDs {
		p, err := t.catalog.GetProduct(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "compare_products: get product", "tool", "compare_products", "product_id", id, "error", err)
			return tools.Result{}, fmt.Errorf("compare_products: get product %s: %w", id, err)
		}
		products = append(products, p)
		v, err := t.embed.Embedding(ctx, id)
		if err == nil {
			embeddings[id] = v
		}
	}

	result := CompareResult{
		Products:   products,
		Shared:     sharedAttrs(products),
		Differing:  differingAttrs(products),
		Similarity: pairSimilarities(req.ProductIDs, embeddings),
	}
	result.Recommendation = composeRecommendation(result)

	slog.InfoContext(ctx, "compare_products: result",
		"tool", "compare_products",
		"product_ids", req.ProductIDs,
		"products_returned", len(result.Products),
		"similarity_pairs", len(result.Similarity),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return tools.Result{Content: result}, nil
}

// Compile-time interface check.
var _ tools.Tool = (*compareProductsTool)(nil)

func sharedAttrs(ps []Product) map[string]string {
	out := map[string]string{}
	if len(ps) == 0 {
		return out
	}
	cat := ps[0].Category
	for _, p := range ps[1:] {
		if p.Category != cat {
			cat = ""
		}
	}
	if cat != "" {
		out["category"] = cat
	}
	return out
}

func differingAttrs(ps []Product) []DifferingAttribute {
	out := []DifferingAttribute{}
	priceVals := map[string]string{}
	allSame := true
	for i, p := range ps {
		priceVals[p.ID] = strconv.Itoa(p.PriceCents)
		if i > 0 && p.PriceCents != ps[0].PriceCents {
			allSame = false
		}
	}
	if !allSame {
		out = append(out, DifferingAttribute{Field: "price_cents", Values: priceVals})
	}
	nameVals := map[string]string{}
	for _, p := range ps {
		nameVals[p.ID] = p.Name
	}
	out = append(out, DifferingAttribute{Field: "name", Values: nameVals})
	return out
}

func pairSimilarities(ids []string, embs map[string][]float32) []PairSimilarity {
	out := []PairSimilarity{}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			a, ok1 := embs[ids[i]]
			b, ok2 := embs[ids[j]]
			if !ok1 || !ok2 {
				continue
			}
			out = append(out, PairSimilarity{Pair: [2]string{ids[i], ids[j]}, Score: cosineSim(a, b)})
		}
	}
	return out
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func composeRecommendation(r CompareResult) string {
	if len(r.Products) == 0 {
		return ""
	}
	cheapest := r.Products[0]
	for _, p := range r.Products[1:] {
		if p.PriceCents < cheapest.PriceCents {
			cheapest = p
		}
	}
	return fmt.Sprintf("If price is the primary factor, %s is the lowest-cost option at $%.2f.", cheapest.Name, float64(cheapest.PriceCents)/100)
}
