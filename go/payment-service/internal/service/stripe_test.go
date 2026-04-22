package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/service"
)

// --- mock PaymentRepo ---

type mockPaymentRepo struct {
	createFn          func(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error)
	findByOrderIDFn   func(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	findByIntentIDFn  func(ctx context.Context, intentID string) (*model.Payment, error)
	updateStatusFn    func(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error
	updateStripeIDsFn func(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error
}

func (m *mockPaymentRepo) Create(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error) {
	return m.createFn(ctx, orderID, amountCents, currency)
}

func (m *mockPaymentRepo) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	return m.findByOrderIDFn(ctx, orderID)
}

func (m *mockPaymentRepo) FindByStripeIntentID(ctx context.Context, intentID string) (*model.Payment, error) {
	return m.findByIntentIDFn(ctx, intentID)
}

func (m *mockPaymentRepo) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error {
	return m.updateStatusFn(ctx, orderID, status)
}

func (m *mockPaymentRepo) UpdateStripeIDs(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error {
	return m.updateStripeIDsFn(ctx, orderID, intentID, sessionID)
}

// --- mock StripeClient ---

type mockStripeClient struct {
	createSessionFn func(ctx context.Context, params service.CheckoutParams) (*service.CheckoutResult, error)
	refundFn        func(ctx context.Context, paymentIntentID, reason string) (string, error)
}

func (m *mockStripeClient) CreateCheckoutSession(ctx context.Context, params service.CheckoutParams) (*service.CheckoutResult, error) {
	return m.createSessionFn(ctx, params)
}

func (m *mockStripeClient) Refund(ctx context.Context, paymentIntentID, reason string) (string, error) {
	return m.refundFn(ctx, paymentIntentID, reason)
}

// --- helpers ---

func newPayment(orderID uuid.UUID) *model.Payment {
	return &model.Payment{
		ID:           uuid.New(),
		OrderID:      orderID,
		AmountCents:  1000,
		Currency:     "usd",
		Status:       model.PaymentStatusPending,
		IdempotencyKey: "payment:" + orderID.String(),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// --- tests ---

func TestPaymentService_CreatePayment(t *testing.T) {
	orderID := uuid.New()
	payment := newPayment(orderID)

	repo := &mockPaymentRepo{
		createFn: func(_ context.Context, id uuid.UUID, _ int, _ string) (*model.Payment, error) {
			return payment, nil
		},
		updateStripeIDsFn: func(_ context.Context, _ uuid.UUID, _, _ string) error {
			return nil
		},
		// updateStatusFn should NOT be called on the happy path; if called it panics to catch unexpected use.
		updateStatusFn: func(_ context.Context, _ uuid.UUID, _ model.PaymentStatus) error {
			t.Fatal("UpdateStatus should not be called on the happy path")
			return nil
		},
	}

	stripe := &mockStripeClient{
		createSessionFn: func(_ context.Context, _ service.CheckoutParams) (*service.CheckoutResult, error) {
			return &service.CheckoutResult{
				SessionURL:      "https://checkout.stripe.com/pay/cs_test_abc",
				PaymentIntentID: "pi_test_123",
				SessionID:       "cs_test_abc",
			}, nil
		},
	}

	svc := service.NewPaymentService(repo, stripe)
	result, err := svc.CreatePayment(context.Background(), orderID, 1000, "usd",
		"https://example.com/success", "https://example.com/cancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.CheckoutURL != "https://checkout.stripe.com/pay/cs_test_abc" {
		t.Errorf("unexpected checkout URL: %s", result.CheckoutURL)
	}
	if result.Payment.StripePaymentIntentID != "pi_test_123" {
		t.Errorf("unexpected intent ID: %s", result.Payment.StripePaymentIntentID)
	}
	if result.Payment.StripeCheckoutSessionID != "cs_test_abc" {
		t.Errorf("unexpected session ID: %s", result.Payment.StripeCheckoutSessionID)
	}
}

func TestPaymentService_CreatePayment_StripeError(t *testing.T) {
	orderID := uuid.New()
	payment := newPayment(orderID)
	stripeErr := errors.New("stripe unavailable")

	statusUpdated := false
	repo := &mockPaymentRepo{
		createFn: func(_ context.Context, _ uuid.UUID, _ int, _ string) (*model.Payment, error) {
			return payment, nil
		},
		updateStatusFn: func(_ context.Context, _ uuid.UUID, status model.PaymentStatus) error {
			if status == model.PaymentStatusFailed {
				statusUpdated = true
			}
			return nil
		},
		// UpdateStripeIDs must not be called when Stripe fails.
		updateStripeIDsFn: func(_ context.Context, _ uuid.UUID, _, _ string) error {
			t.Fatal("UpdateStripeIDs should not be called when Stripe errors")
			return nil
		},
	}

	stripe := &mockStripeClient{
		createSessionFn: func(_ context.Context, _ service.CheckoutParams) (*service.CheckoutResult, error) {
			return nil, stripeErr
		},
	}

	svc := service.NewPaymentService(repo, stripe)
	result, err := svc.CreatePayment(context.Background(), orderID, 1000, "usd",
		"https://example.com/success", "https://example.com/cancel")
	if err == nil {
		t.Fatal("expected an error but got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on error, got %+v", result)
	}
	if !statusUpdated {
		t.Error("expected payment status to be set to failed")
	}
}
