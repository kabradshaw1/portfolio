package projection

import (
	"context"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Timeline projects each order event into the timeline read-model table.
type Timeline struct {
	repo *repository.Repository
}

// NewTimeline creates a Timeline projection backed by the given repository.
func NewTimeline(repo *repository.Repository) *Timeline {
	return &Timeline{repo: repo}
}

// Apply converts an OrderEvent to a TimelineEvent and persists it.
func (t *Timeline) Apply(ctx context.Context, evt *consumer.OrderEvent) error {
	return t.repo.InsertTimelineEvent(ctx, repository.TimelineEvent{
		EventID:      evt.ID,
		OrderID:      evt.OrderID,
		EventType:    evt.Type,
		EventVersion: evt.Version,
		Data:         evt.Data,
		Timestamp:    evt.Timestamp,
	})
}
