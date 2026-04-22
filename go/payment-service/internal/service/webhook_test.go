package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

// --- mocks ---

type mockProcessedEventRepo struct {
	inserted bool
	err      error
}

func (m *mockProcessedEventRepo) TryInsert(_ context.Context, _ pgx.Tx, _, _ string) (bool, error) {
	return m.inserted, m.err
}

type mockOutboxRepo struct {
	insertErr error
	calls     int
}

func (m *mockOutboxRepo) Insert(_ context.Context, _ pgx.Tx, _, _ string, _ []byte) error {
	m.calls++
	return m.insertErr
}

type mockPaymentRepoForWebhook struct {
	findErr    error
	updateErr  error
	findCalled int
}

func (m *mockPaymentRepoForWebhook) Create(_ context.Context, _ uuid.UUID, _ int, _ string) (*model.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepoForWebhook) FindByOrderID(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepoForWebhook) FindByStripeIntentID(_ context.Context, _ string) (*model.Payment, error) {
	m.findCalled++
	return nil, m.findErr
}

func (m *mockPaymentRepoForWebhook) UpdateStatus(_ context.Context, _ uuid.UUID, _ model.PaymentStatus) error {
	return m.updateErr
}

func (m *mockPaymentRepoForWebhook) UpdateStripeIDs(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}

// --- tests ---

func TestWebhookService_HandlePaymentSucceeded_DuplicateEvent(t *testing.T) {
	eventRepo := &mockProcessedEventRepo{inserted: false, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{}

	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_dup", "pi_123")
	if err != nil {
		t.Errorf("expected nil error for duplicate event, got %v", err)
	}
	if paymentRepo.findCalled != 0 {
		t.Errorf("expected no FindByStripeIntentID call for duplicate, got %d", paymentRepo.findCalled)
	}
	if outboxRepo.calls != 0 {
		t.Errorf("expected no outbox insert for duplicate event, got %d", outboxRepo.calls)
	}
}
