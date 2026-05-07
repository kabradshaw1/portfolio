package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeProducer struct {
	err error
}

func (f fakeProducer) Publish(context.Context, string, string, Event) error { return f.err }
func (f fakeProducer) Close() error                                         { return nil }

type fakeRecorder struct {
	messages  map[string]int
	durations int
	asyncMode bool
}

func (f *fakeRecorder) ObserveMessage(_, _, outcome string) {
	if f.messages == nil {
		f.messages = make(map[string]int)
	}
	f.messages[outcome]++
}

func (f *fakeRecorder) ObserveWriteDuration(_, _ string, _ time.Duration) {
	f.durations++
}

func (f *fakeRecorder) SetAsyncMode(_ string, enabled bool) {
	f.asyncMode = enabled
}

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

func TestSafePublishRecordsSuccessAndSwallowsErrors(t *testing.T) {
	rec := &fakeRecorder{}
	previous := metricsRecorder
	metricsRecorder = rec
	t.Cleanup(func() { metricsRecorder = previous })

	SafePublish(context.Background(), fakeProducer{}, "ecommerce.orders", "key", Event{Type: "order.created"})
	SafePublish(context.Background(), fakeProducer{err: errors.New("write failed")}, "ecommerce.orders", "key", Event{Type: "order.created"})

	if rec.messages["success"] != 1 {
		t.Fatalf("success count = %d, want 1", rec.messages["success"])
	}
	if rec.messages["error"] != 1 {
		t.Fatalf("error count = %d, want 1", rec.messages["error"])
	}
	if rec.durations != 2 {
		t.Fatalf("durations = %d, want 2", rec.durations)
	}
}
