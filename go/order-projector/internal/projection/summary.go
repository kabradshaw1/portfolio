package projection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// Summary projects order events into the denormalized order_summary read model.
type Summary struct {
	repo *repository.Repository
}

// NewSummary creates a Summary projection backed by the given repository.
func NewSummary(repo *repository.Repository) *Summary {
	return &Summary{repo: repo}
}

// orderCreatedData holds the fields extracted from an order.created event payload.
type orderCreatedData struct {
	UserID     string          `json:"userID"`
	TotalCents int64           `json:"totalCents"`
	Currency   string          `json:"currency"`
	Items      json.RawMessage `json:"items"`
}

// orderFailedData holds the fields extracted from an order.failed event payload.
type orderFailedData struct {
	FailureReason string `json:"failureReason"`
}

// statusFromEventType maps an event type string to the corresponding order status.
func statusFromEventType(eventType string) string {
	switch eventType {
	case "order.created":
		return "created"
	case "order.reserved":
		return "reserved"
	case "order.payment_initiated":
		return "payment_initiated"
	case "order.payment_completed":
		return "payment_completed"
	case "order.completed":
		return "completed"
	case "order.failed":
		return "failed"
	case "order.cancelled":
		return "cancelled"
	default:
		return "unknown"
	}
}

// Apply switches on event type, extracts relevant data, and upserts the order summary.
func (s *Summary) Apply(ctx context.Context, evt *consumer.OrderEvent) error {
	status := statusFromEventType(evt.Type)

	summary := repository.OrderSummary{
		OrderID:   evt.OrderID,
		Status:    status,
		UpdatedAt: evt.Timestamp,
	}

	switch evt.Type {
	case "order.created":
		var d orderCreatedData
		if err := json.Unmarshal(evt.Data, &d); err != nil {
			return fmt.Errorf("unmarshal order.created data: %w", err)
		}
		summary.UserID = d.UserID
		summary.TotalCents = d.TotalCents
		summary.Currency = d.Currency
		summary.Items = d.Items
		summary.CreatedAt = evt.Timestamp

	case "order.completed":
		ts := evt.Timestamp
		summary.CompletedAt = &ts

	case "order.failed":
		var d orderFailedData
		if err := json.Unmarshal(evt.Data, &d); err != nil {
			return fmt.Errorf("unmarshal order.failed data: %w", err)
		}
		summary.FailureReason = &d.FailureReason
	}

	return s.repo.UpsertOrderSummary(ctx, summary)
}
