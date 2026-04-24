package consumer

import (
	"encoding/json"
	"testing"
	"time"
)

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func makeEvent(t *testing.T, eventType string, version int, data map[string]any) []byte {
	t.Helper()
	evt := map[string]any{
		"id":        "evt-001",
		"type":      eventType,
		"version":   version,
		"source":    "order-service",
		"order_id":  "order-123",
		"timestamp": time.Now().Format(time.RFC3339),
		"traceID":   "trace-abc",
		"data":      data,
	}
	return mustMarshal(t, evt)
}

func TestDeserialize_V1OrderCreatedGetsCurrencyBackfilled(t *testing.T) {
	t.Parallel()
	raw := makeEvent(t, "order.created", 1, map[string]any{
		"total": 99.99,
		"items": []string{"item-a", "item-b"},
	})

	evt, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if evt.Version != LatestVersion {
		t.Errorf("version = %d, want %d", evt.Version, LatestVersion)
	}

	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}

	currency, ok := data["currency"]
	if !ok {
		t.Fatal("currency field missing after upgrade")
	}
	if currency != "USD" {
		t.Errorf("currency = %q, want %q", currency, "USD")
	}

	// Verify original fields are preserved.
	if _, ok := data["total"]; !ok {
		t.Error("total field missing after upgrade")
	}
}

func TestDeserialize_V2OrderCreatedKeepsExistingCurrency(t *testing.T) {
	t.Parallel()
	raw := makeEvent(t, "order.created", 2, map[string]any{
		"total":    150.00,
		"currency": "EUR",
	})

	evt, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if evt.Version != LatestVersion {
		t.Errorf("version = %d, want %d", evt.Version, LatestVersion)
	}

	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}

	if data["currency"] != "EUR" {
		t.Errorf("currency = %q, want %q", data["currency"], "EUR")
	}
}

func TestDeserialize_UnversionedEventTreatedAsV1(t *testing.T) {
	t.Parallel()
	// version=0 should be treated as v1 and upgraded.
	raw := makeEvent(t, "order.created", 0, map[string]any{
		"total": 50.00,
	})

	evt, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if evt.Version != LatestVersion {
		t.Errorf("version = %d, want %d", evt.Version, LatestVersion)
	}

	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}

	if data["currency"] != "USD" {
		t.Errorf("currency = %q, want %q", data["currency"], "USD")
	}
}

func TestDeserialize_EventTypeWithoutUpgraders(t *testing.T) {
	t.Parallel()
	originalData := map[string]any{
		"reason": "customer_request",
	}
	raw := makeEvent(t, "order.reserved", 1, originalData)

	evt, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Version should still be bumped to latest even without upgraders.
	if evt.Version != LatestVersion {
		t.Errorf("version = %d, want %d", evt.Version, LatestVersion)
	}

	// Data should remain unchanged (no upgraders registered).
	var data map[string]any
	if err := json.Unmarshal(evt.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}

	if data["reason"] != "customer_request" {
		t.Errorf("reason = %q, want %q", data["reason"], "customer_request")
	}
}

func TestUpgradeOrderCreatedV1toV2_DoesNotOverwriteExistingCurrency(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"total":    200.00,
		"currency": "GBP",
	}

	result := upgradeOrderCreatedV1toV2(data)

	if result["currency"] != "GBP" {
		t.Errorf("currency = %q, want %q (should not overwrite)", result["currency"], "GBP")
	}
}

func TestDeserialize_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := Deserialize([]byte(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDeserialize_PreservesEventMetadata(t *testing.T) {
	t.Parallel()
	raw := makeEvent(t, "order.created", 1, map[string]any{"total": 10.0})

	evt, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if evt.ID != "evt-001" {
		t.Errorf("ID = %q, want %q", evt.ID, "evt-001")
	}
	if evt.Type != "order.created" {
		t.Errorf("Type = %q, want %q", evt.Type, "order.created")
	}
	if evt.OrderID != "order-123" {
		t.Errorf("OrderID = %q, want %q", evt.OrderID, "order-123")
	}
	if evt.Source != "order-service" {
		t.Errorf("Source = %q, want %q", evt.Source, "order-service")
	}
	if evt.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want %q", evt.TraceID, "trace-abc")
	}
}
