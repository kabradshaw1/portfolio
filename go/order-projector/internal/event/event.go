// Package event defines the domain event types shared across the order-projector.
package event

import (
	"encoding/json"
	"time"
)

// OrderEvent represents a deserialized Kafka event for the order domain.
type OrderEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Version   int             `json:"version"`
	Source    string          `json:"source"`
	OrderID   string          `json:"order_id"`
	Timestamp time.Time       `json:"timestamp"`
	TraceID   string          `json:"traceID"`
	Data      json.RawMessage `json:"data"`
}
