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
	pingErr   error
	pingCalls int
}

func (m *mockFetcher) FetchUnpublished(_ context.Context, _ int) ([]model.OutboxMessage, error) {
	return nil, nil
}

func (m *mockFetcher) MarkPublished(_ context.Context, _ uuid.UUID) error {
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
