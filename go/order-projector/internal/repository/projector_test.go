package repository

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTimelineEventJSON(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	ev := TimelineEvent{
		EventID:      "evt-001",
		OrderID:      "ord-001",
		EventType:    "order.created",
		EventVersion: 1,
		Data:         json.RawMessage(`{"items":2}`),
		Timestamp:    ts,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got TimelineEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.EventID != ev.EventID {
		t.Errorf("eventId = %q, want %q", got.EventID, ev.EventID)
	}
	if got.OrderID != ev.OrderID {
		t.Errorf("orderId = %q, want %q", got.OrderID, ev.OrderID)
	}
	if got.EventType != ev.EventType {
		t.Errorf("eventType = %q, want %q", got.EventType, ev.EventType)
	}
	if got.EventVersion != ev.EventVersion {
		t.Errorf("eventVersion = %d, want %d", got.EventVersion, ev.EventVersion)
	}
	if string(got.Data) != string(ev.Data) {
		t.Errorf("data = %s, want %s", got.Data, ev.Data)
	}
	if !got.Timestamp.Equal(ev.Timestamp) {
		t.Errorf("timestamp = %v, want %v", got.Timestamp, ev.Timestamp)
	}

	// Verify JSON field names.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"eventId", "orderId", "eventType", "eventVersion", "data", "timestamp"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestOrderSummaryJSON(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	completed := now.Add(5 * time.Minute)
	reason := "payment declined"

	s := OrderSummary{
		OrderID:       "ord-002",
		UserID:        "usr-001",
		Status:        "completed",
		TotalCents:    4999,
		Currency:      "USD",
		Items:         json.RawMessage(`[{"id":"p1","qty":1}]`),
		CreatedAt:     now,
		UpdatedAt:     now,
		CompletedAt:   &completed,
		FailureReason: &reason,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got OrderSummary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.OrderID != s.OrderID {
		t.Errorf("orderId = %q, want %q", got.OrderID, s.OrderID)
	}
	if got.TotalCents != s.TotalCents {
		t.Errorf("totalCents = %d, want %d", got.TotalCents, s.TotalCents)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completed) {
		t.Errorf("completedAt = %v, want %v", got.CompletedAt, completed)
	}
	if got.FailureReason == nil || *got.FailureReason != reason {
		t.Errorf("failureReason = %v, want %q", got.FailureReason, reason)
	}
}

func TestOrderSummaryJSON_OmitsNilFields(t *testing.T) {
	t.Parallel()

	s := OrderSummary{
		OrderID:    "ord-003",
		UserID:     "usr-002",
		Status:     "pending",
		TotalCents: 1000,
		Currency:   "USD",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	if _, ok := m["completedAt"]; ok {
		t.Error("completedAt should be omitted when nil")
	}
	if _, ok := m["failureReason"]; ok {
		t.Error("failureReason should be omitted when nil")
	}
	if _, ok := m["items"]; ok {
		t.Error("items should be omitted when nil")
	}
}

func TestOrderStatsJSON(t *testing.T) {
	t.Parallel()

	s := OrderStats{
		HourBucket:           time.Date(2026, 4, 23, 14, 0, 0, 0, time.UTC),
		OrdersCreated:        10,
		OrdersCompleted:      8,
		OrdersFailed:         1,
		AvgCompletionSeconds: 45.5,
		TotalRevenueCents:    125000,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got OrderStats
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.OrdersCreated != s.OrdersCreated {
		t.Errorf("ordersCreated = %d, want %d", got.OrdersCreated, s.OrdersCreated)
	}
	if got.AvgCompletionSeconds != s.AvgCompletionSeconds {
		t.Errorf("avgCompletionSeconds = %f, want %f", got.AvgCompletionSeconds, s.AvgCompletionSeconds)
	}
	if got.TotalRevenueCents != s.TotalRevenueCents {
		t.Errorf("totalRevenueCents = %d, want %d", got.TotalRevenueCents, s.TotalRevenueCents)
	}

	// Verify JSON field names.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	for _, key := range []string{"hourBucket", "ordersCreated", "ordersCompleted", "ordersFailed", "avgCompletionSeconds", "totalRevenueCents"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestReplayStatusJSON(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 4, 23, 13, 0, 0, 0, time.UTC)
	s := ReplayStatus{
		IsReplaying:     true,
		Projection:      "all",
		StartedAt:       &started,
		CompletedAt:     nil,
		EventsProcessed: 500,
		TotalEvents:     1000,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ReplayStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.IsReplaying != s.IsReplaying {
		t.Errorf("isReplaying = %v, want %v", got.IsReplaying, s.IsReplaying)
	}
	if got.Projection != s.Projection {
		t.Errorf("projection = %q, want %q", got.Projection, s.Projection)
	}
	if got.StartedAt == nil || !got.StartedAt.Equal(started) {
		t.Errorf("startedAt = %v, want %v", got.StartedAt, started)
	}
	if got.CompletedAt != nil {
		t.Error("completedAt should be nil")
	}
	if got.EventsProcessed != s.EventsProcessed {
		t.Errorf("eventsProcessed = %d, want %d", got.EventsProcessed, s.EventsProcessed)
	}

	// Verify completedAt is omitted.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := m["completedAt"]; ok {
		t.Error("completedAt should be omitted when nil")
	}
}

func TestNewRepository(t *testing.T) {
	t.Parallel()

	// Verify constructor does not panic with nil pool (used in unit test contexts).
	r := New(nil, nil)
	if r == nil {
		t.Fatal("New returned nil")
	}
}
