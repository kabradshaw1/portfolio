package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeCartService struct {
	reserveErr error
	releaseErr error
	clearErr   error
}

func (f fakeCartService) ReserveItems(context.Context, uuid.UUID) error {
	return f.reserveErr
}

func (f fakeCartService) ReleaseItems(context.Context, uuid.UUID) error {
	return f.releaseErr
}

func (f fakeCartService) ClearCart(context.Context, uuid.UUID) error {
	return f.clearErr
}

type fakePublisher struct {
	err        error
	exchange   string
	routingKey string
	msg        amqp.Publishing
	calls      int
}

func (f *fakePublisher) Publish(_ context.Context, exchange, routingKey string, msg amqp.Publishing) error {
	f.calls++
	f.exchange = exchange
	f.routingKey = routingKey
	f.msg = msg
	return f.err
}

func TestHandleMessageInvalidJSONIsPermanent(t *testing.T) {
	h := NewSagaHandler(fakeCartService{}, nil)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: []byte("{")})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json should be permanent, got %v", err)
	}
}

func TestHandleMessageInvalidOrderIDIsPermanent(t *testing.T) {
	h := NewSagaHandler(fakeCartService{}, nil)
	body := []byte(`{"command":"reserve.items","order_id":"not-a-uuid","user_id":"550e8400-e29b-41d4-a716-446655440000"}`)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: body})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid order ID should be permanent, got %v", err)
	}
}

func TestHandleMessageInvalidUserIDIsPermanent(t *testing.T) {
	h := NewSagaHandler(fakeCartService{}, nil)
	body := []byte(`{"command":"reserve.items","order_id":"550e8400-e29b-41d4-a716-446655440000","user_id":"not-a-uuid"}`)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: body})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid user ID should be permanent, got %v", err)
	}
}

func TestHandleMessageUnknownCommandIsPermanent(t *testing.T) {
	h := NewSagaHandler(fakeCartService{}, nil)
	body := []byte(`{"command":"unknown.command","order_id":"550e8400-e29b-41d4-a716-446655440000","user_id":"650e8400-e29b-41d4-a716-446655440000"}`)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: body})

	if !rabbitmq.IsPermanent(err) {
		t.Fatalf("unknown command should be permanent, got %v", err)
	}
}

func TestHandleMessagePublishesPersistentConfirmedReply(t *testing.T) {
	pub := &fakePublisher{}
	h := newSagaHandlerWithPublisher(fakeCartService{}, pub)
	body := []byte(`{"command":"clear.cart","order_id":"550e8400-e29b-41d4-a716-446655440000","user_id":"650e8400-e29b-41d4-a716-446655440000"}`)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: body})

	if err != nil {
		t.Fatalf("HandleMessageForTest returned error: %v", err)
	}
	if pub.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", pub.calls)
	}
	if pub.exchange != sagaExchange || pub.routingKey != orderEventsKey {
		t.Fatalf("published to %s/%s, want %s/%s", pub.exchange, pub.routingKey, sagaExchange, orderEventsKey)
	}
	if pub.msg.DeliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want persistent", pub.msg.DeliveryMode)
	}

	var got event
	if err := json.Unmarshal(pub.msg.Body, &got); err != nil {
		t.Fatalf("unmarshal reply: %v", err)
	}
	if got.Event != "cart.cleared" || !got.Success {
		t.Fatalf("reply = %+v, want successful cart.cleared", got)
	}
}

func TestHandleMessageReturnsPublishErrorAsRetryable(t *testing.T) {
	want := errors.New("publish failed")
	pub := &fakePublisher{err: want}
	h := newSagaHandlerWithPublisher(fakeCartService{}, pub)
	body := []byte(`{"command":"clear.cart","order_id":"550e8400-e29b-41d4-a716-446655440000","user_id":"650e8400-e29b-41d4-a716-446655440000"}`)

	err := h.HandleMessageForTest(context.Background(), amqp.Delivery{Body: body})

	if !errors.Is(err, want) {
		t.Fatalf("HandleMessageForTest error = %v, want wrapped %v", err, want)
	}
	if rabbitmq.IsPermanent(err) {
		t.Fatalf("publish error should be retryable, got permanent %v", err)
	}
}
