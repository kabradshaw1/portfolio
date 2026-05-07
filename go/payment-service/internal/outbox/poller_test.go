package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

type mockFetcher struct {
	pingErr    error
	pingCalls  int
	messages   []model.OutboxMessage
	marked     []uuid.UUID
	fetchCalls int
}

func (m *mockFetcher) FetchUnpublished(_ context.Context, _ int) ([]model.OutboxMessage, error) {
	m.fetchCalls++
	return m.messages, nil
}

func (m *mockFetcher) MarkPublished(_ context.Context, id uuid.UUID) error {
	m.marked = append(m.marked, id)
	return nil
}

func (m *mockFetcher) Ping(ctx context.Context) error {
	m.pingCalls++
	return m.pingErr
}

func TestNewPoller(t *testing.T) {
	p := NewPoller(nil, (*amqp.Channel)(nil), time.Second, 10)
	if p == nil {
		t.Fatal("expected non-nil Poller")
	}
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

func TestPollPublishesPersistentMessageAndMarksPublishedAfterConfirm(t *testing.T) {
	messageID := uuid.New()
	fetcher := &mockFetcher{messages: []model.OutboxMessage{{
		ID:         messageID,
		Exchange:   "ecommerce.saga",
		RoutingKey: "saga.order.events",
		Payload:    []byte(`{"event":"payment.confirmed"}`),
	}}}
	publisher := &fakePublisher{}
	poller := NewPollerWithPublisher(fetcher, publisher, nil, time.Second, 10)

	poller.poll(context.Background())

	if publisher.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", publisher.calls)
	}
	if publisher.exchange != "ecommerce.saga" || publisher.routingKey != "saga.order.events" {
		t.Fatalf("published to %s/%s", publisher.exchange, publisher.routingKey)
	}
	if publisher.msg.DeliveryMode != amqp.Persistent {
		t.Fatalf("delivery mode = %d, want persistent", publisher.msg.DeliveryMode)
	}
	if publisher.msg.MessageId != messageID.String() {
		t.Fatalf("message ID = %q, want %q", publisher.msg.MessageId, messageID.String())
	}
	if len(fetcher.marked) != 1 || fetcher.marked[0] != messageID {
		t.Fatalf("marked published = %v, want [%s]", fetcher.marked, messageID)
	}
}

func TestPollDoesNotMarkPublishedWhenPublishFails(t *testing.T) {
	messageID := uuid.New()
	fetcher := &mockFetcher{messages: []model.OutboxMessage{{
		ID:         messageID,
		Exchange:   "ecommerce.saga",
		RoutingKey: "saga.order.events",
		Payload:    []byte(`{"event":"payment.confirmed"}`),
	}}}
	publisher := &fakePublisher{err: errors.New("publish failed")}
	poller := NewPollerWithPublisher(fetcher, publisher, nil, time.Second, 10)

	poller.poll(context.Background())

	if publisher.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", publisher.calls)
	}
	if len(fetcher.marked) != 0 {
		t.Fatalf("marked published = %v, want none", fetcher.marked)
	}
}

func TestWaitForDB_SuccessFirstAttempt(t *testing.T) {
	f := &mockFetcher{}
	p := NewPoller(f, (*amqp.Channel)(nil), time.Second, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.waitForDB(ctx)

	if f.pingCalls != 1 {
		t.Errorf("expected 1 ping call, got %d", f.pingCalls)
	}
}

func TestWaitForDB_ContextCancelled(t *testing.T) {
	f := &mockFetcher{pingErr: errors.New("connection refused")}
	p := NewPoller(f, (*amqp.Channel)(nil), time.Second, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p.waitForDB(ctx)

	if f.pingCalls < 1 {
		t.Errorf("expected at least 1 ping call, got %d", f.pingCalls)
	}
}
