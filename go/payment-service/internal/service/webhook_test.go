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
	findByOrderPayment *model.Payment
	findByOrderErr     error
	findByIntentErr    error
	updateStatusErr    error
	updateStripeIDsErr error
	findByOrderCalled  int
	findByIntentCalled int
	updateStripeCalled int
}

func (m *mockPaymentRepoForWebhook) Create(_ context.Context, _ uuid.UUID, _ int, _ string) (*model.Payment, error) {
	return nil, nil
}

func (m *mockPaymentRepoForWebhook) FindByOrderID(_ context.Context, _ uuid.UUID) (*model.Payment, error) {
	m.findByOrderCalled++
	return m.findByOrderPayment, m.findByOrderErr
}

func (m *mockPaymentRepoForWebhook) FindByStripeIntentID(_ context.Context, _ string) (*model.Payment, error) {
	m.findByIntentCalled++
	return nil, m.findByIntentErr
}

func (m *mockPaymentRepoForWebhook) UpdateStatus(_ context.Context, _ uuid.UUID, _ model.PaymentStatus) error {
	return m.updateStatusErr
}

func (m *mockPaymentRepoForWebhook) UpdateStripeIDs(_ context.Context, _ uuid.UUID, _, _ string) error {
	m.updateStripeCalled++
	return m.updateStripeIDsErr
}

// --- tests ---

func TestWebhookService_HandlePaymentSucceeded_MetadataLookup(t *testing.T) {
	orderID := uuid.MustParse("f5cd888c-c661-41ad-a2fd-e14fdeac800d")
	eventRepo := &mockProcessedEventRepo{inserted: true, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{
		findByOrderPayment: &model.Payment{
			OrderID:                 orderID,
			StripeCheckoutSessionID: "cs_test_abc",
		},
	}
	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	metadata := map[string]string{"order_id": orderID.String()}
	err := svc.HandlePaymentSucceeded(context.Background(), "evt_123", "pi_new_123", metadata)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paymentRepo.findByOrderCalled != 1 {
		t.Errorf("expected FindByOrderID called once, got %d", paymentRepo.findByOrderCalled)
	}
	if paymentRepo.findByIntentCalled != 0 {
		t.Errorf("expected FindByStripeIntentID not called, got %d", paymentRepo.findByIntentCalled)
	}
	if paymentRepo.updateStripeCalled != 1 {
		t.Errorf("expected UpdateStripeIDs called once to backfill, got %d", paymentRepo.updateStripeCalled)
	}
	if outboxRepo.calls != 1 {
		t.Errorf("expected 1 outbox insert, got %d", outboxRepo.calls)
	}
}

func TestWebhookService_HandlePaymentSucceeded_DuplicateEvent(t *testing.T) {
	eventRepo := &mockProcessedEventRepo{inserted: false, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{}

	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_dup", "pi_123", map[string]string{"order_id": "f5cd888c-c661-41ad-a2fd-e14fdeac800d"})
	if err != nil {
		t.Errorf("expected nil error for duplicate event, got %v", err)
	}
	if paymentRepo.findByOrderCalled != 0 {
		t.Errorf("expected no FindByOrderID call for duplicate, got %d", paymentRepo.findByOrderCalled)
	}
	if outboxRepo.calls != 0 {
		t.Errorf("expected no outbox insert for duplicate event, got %d", outboxRepo.calls)
	}
}

func TestWebhookService_HandlePaymentSucceeded_MissingOrderID(t *testing.T) {
	eventRepo := &mockProcessedEventRepo{inserted: true, err: nil}
	outboxRepo := &mockOutboxRepo{}
	paymentRepo := &mockPaymentRepoForWebhook{}

	svc := NewWebhookService(paymentRepo, eventRepo, outboxRepo, nil)

	err := svc.HandlePaymentSucceeded(context.Background(), "evt_123", "pi_123", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing order_id in metadata")
	}
}
