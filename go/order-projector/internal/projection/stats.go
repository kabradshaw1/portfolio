package projection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/event"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Stats projects order events into hourly aggregated order_stats buckets.
// Only order.created, order.completed, and order.failed events are relevant.
type Stats struct {
	repo *repository.Repository
}

// NewStats creates a Stats projection backed by the given repository.
func NewStats(repo *repository.Repository) *Stats {
	return &Stats{repo: repo}
}

// completedData extracts totalCents from an order.completed event payload.
type completedData struct {
	TotalCents int64 `json:"totalCents"`
}

// Apply increments the appropriate hourly counter based on event type.
func (s *Stats) Apply(ctx context.Context, evt *event.OrderEvent) error {
	bucket := evt.Timestamp.Truncate(time.Hour)

	switch evt.Type {
	case "order.created":
		return s.repo.UpsertOrderStats(ctx, bucket, 1, 0, 0, 0)

	case "order.completed":
		var d completedData
		// Best-effort extraction of revenue; ignore errors and default to 0.
		_ = json.Unmarshal(evt.Data, &d)
		return s.repo.UpsertOrderStats(ctx, bucket, 0, 1, 0, d.TotalCents)

	case "order.failed":
		return s.repo.UpsertOrderStats(ctx, bucket, 0, 0, 1, 0)

	default:
		// Other event types do not affect stats.
		return nil
	}
}
