package kafka

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNopProducer_DoesNotError(t *testing.T) {
	p := NopProducer{}
	err := p.Publish(context.Background(), "test", "key", Event{Type: "test"})
	if err != nil {
		t.Fatalf("NopProducer should not error: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("NopProducer.Close should not error: %v", err)
	}
}

func TestEvent_MarshalJSON(t *testing.T) {
	e := Event{
		ID:     "abc",
		Type:   "order.created",
		Source: "ecommerce-service",
		Data: map[string]any{
			"orderID": "123",
			"total":   4999,
		},
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["type"] != "order.created" {
		t.Errorf("expected type order.created, got %v", decoded["type"])
	}
	if decoded["source"] != "ecommerce-service" {
		t.Errorf("expected source ecommerce-service, got %v", decoded["source"])
	}
}

func TestSafePublish_SwallowsNopErrors(t *testing.T) {
	p := NopProducer{}
	// Should not panic or error.
	SafePublish(context.Background(), p, "topic", "key", Event{Type: "test"})
}
