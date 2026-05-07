package projection

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/event"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/metrics"
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
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 1, 0, 0, 0)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	case "order.completed":
		var d completedData
		// Best-effort extraction of revenue; ignore errors and default to 0.
		_ = json.Unmarshal(evt.Data, &d)
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 0, 1, 0, d.TotalCents)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	case "order.failed":
		inserted, err := s.repo.UpsertOrderStatsOnce(ctx, evt.ID, bucket, 0, 0, 1, 0)
		if err == nil && !inserted {
			metrics.DuplicateEvents.WithLabelValues(ProjectionStats).Inc()
		}
		return err

	default:
		// Other event types do not affect stats.
		return nil
	}
}
