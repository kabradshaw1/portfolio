package events

import (
	"context"
	"testing"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

// mockProducer records Publish calls for assertions.
type mockProducer struct {
	calls []publishCall
}

type publishCall struct {
	topic string
	key   string
	event kafka.Event
}

func (m *mockProducer) Publish(_ context.Context, topic, key string, event kafka.Event) error {
	m.calls = append(m.calls, publishCall{topic: topic, key: key, event: event})
	return nil
}

func (m *mockProducer) Close() error { return nil }

func TestPublish_SendsCorrectTopicAndKey(t *testing.T) {
	mock := &mockProducer{}
	pub := NewPublisher(mock)

	data := OrderCreatedData{UserID: "u1", TotalCents: 500, Currency: "USD"}
	pub.Publish(context.Background(), "order-123", OrderCreated, data)

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.topic != TopicOrderEvents {
		t.Errorf("topic = %q, want %q", call.topic, TopicOrderEvents)
	}
	if call.key != "order-123" {
		t.Errorf("key = %q, want %q", call.key, "order-123")
	}
}

func TestPublish_SetsEventTypeAndVersion(t *testing.T) {
	mock := &mockProducer{}
	pub := NewPublisher(mock)

	pub.Publish(context.Background(), "order-456", OrderFailed, OrderFailedData{
		FailureReason: "stock unavailable",
		FailedStep:    "ITEMS_RESERVED",
	})

	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	evt := mock.calls[0].event
	if evt.Type != OrderFailed {
		t.Errorf("event type = %q, want %q", evt.Type, OrderFailed)
	}
	if evt.Version != CurrentVersion {
		t.Errorf("version = %d, want %d", evt.Version, CurrentVersion)
	}
}

func TestPublish_NilProducerDoesNotPanic(t *testing.T) {
	pub := NewPublisher(nil)
	// Should not panic.
	pub.Publish(context.Background(), "order-789", OrderCompleted, OrderCompletedData{
		CompletedAt: "2026-04-23T00:00:00Z",
	})
}
