package store

import (
	"context"
	"time"
)

// RevenueWindow holds aggregated revenue data for a single time window.
type RevenueWindow struct {
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	TotalCents  int64     `json:"total_cents"`
	OrderCount  int64     `json:"order_count"`
	AvgCents    int64     `json:"avg_order_value_cents"`
}

// TrendingProduct holds a single product's trending score and interaction counts.
type TrendingProduct struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Score       float64 `json:"score"`
	Views       int     `json:"views"`
	CartAdds    int     `json:"cart_adds"`
}

// TrendingResult holds the trending products for a given window.
type TrendingResult struct {
	WindowEnd time.Time         `json:"window_end"`
	Products  []TrendingProduct `json:"products"`
}

// AbandonmentWindow holds cart abandonment metrics for a single time window.
type AbandonmentWindow struct {
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	CartsStarted    int64     `json:"carts_started"`
	CartsConverted  int64     `json:"carts_converted"`
	CartsAbandoned  int64     `json:"carts_abandoned"`
	AbandonmentRate float64   `json:"abandonment_rate"`
}

// Store abstracts persistence for windowed aggregation results.
type Store interface {
	// Revenue
	FlushRevenue(ctx context.Context, windowKey string, totalCents, orderCount int64) error
	GetRevenue(ctx context.Context, hours int) ([]RevenueWindow, error)

	// Trending
	FlushTrending(ctx context.Context, windowKey string, scores map[string]float64, names map[string]string) error
	GetTrending(ctx context.Context, limit int) (*TrendingResult, error)

	// Abandonment
	FlushAbandonment(ctx context.Context, windowKey string, started, converted int64) error
	GetAbandonment(ctx context.Context, hours int) ([]AbandonmentWindow, error)
	TrackAbandonmentUser(ctx context.Context, windowKey, userID, bucket string) error
	CountAbandonmentUsers(ctx context.Context, windowKey, bucket string) (int64, error)
}
