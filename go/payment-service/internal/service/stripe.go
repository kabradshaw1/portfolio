package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/repository"
)

// PaymentRepo is the interface the service expects from the repository layer.
type PaymentRepo interface {
	Create(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error)
	FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error)
	FindByStripeIntentID(ctx context.Context, intentID string) (*model.Payment, error)
	UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error
	UpdateStripeIDs(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error
}

// CheckoutParams holds the data needed to create a Stripe Checkout Session.
type CheckoutParams struct {
	AmountCents    int
	Currency       string
	OrderID        string
	IdempotencyKey string
	SuccessURL     string
	CancelURL      string
}

// CheckoutResult is returned by the Stripe client after session creation.
type CheckoutResult struct {
	SessionURL      string
	PaymentIntentID string
	SessionID       string
}

// StripeClient abstracts the Stripe API for testability.
type StripeClient interface {
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutResult, error)
	Refund(ctx context.Context, paymentIntentID, reason string) (string, error)
}

// CreatePaymentResult bundles the persisted payment record with the redirect URL.
type CreatePaymentResult struct {
	Payment     *model.Payment
	CheckoutURL string
}

// PaymentService orchestrates payment creation, status queries, and refunds.
type PaymentService struct {
	repo   PaymentRepo
	stripe StripeClient
}

// NewPaymentService creates a PaymentService with the given repository and Stripe client.
func NewPaymentService(repo PaymentRepo, stripe StripeClient) *PaymentService {
	return &PaymentService{repo: repo, stripe: stripe}
}

// CreatePayment creates a DB payment record, opens a Stripe Checkout Session, and
// persists the Stripe IDs back to the database. If Stripe returns an error the
// payment record is marked as failed before the error is returned.
func (s *PaymentService) CreatePayment(
	ctx context.Context,
	orderID uuid.UUID,
	amountCents int,
	currency, successURL, cancelURL string,
) (*CreatePaymentResult, error) {
	payment, err := s.repo.Create(ctx, orderID, amountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("create payment record: %w", err)
	}

	params := CheckoutParams{
		AmountCents:    amountCents,
		Currency:       currency,
		OrderID:        orderID.String(),
		IdempotencyKey: repository.IdempotencyKey(orderID),
		SuccessURL:     successURL,
		CancelURL:      cancelURL,
	}

	result, err := s.stripe.CreateCheckoutSession(ctx, params)
	if err != nil {
		// Best-effort: mark the record failed so the caller can surface the right status.
		_ = s.repo.UpdateStatus(ctx, orderID, model.PaymentStatusFailed)
		return nil, fmt.Errorf("create stripe checkout session: %w", err)
	}

	if updateErr := s.repo.UpdateStripeIDs(ctx, orderID, result.PaymentIntentID, result.SessionID); updateErr != nil {
		return nil, fmt.Errorf("persist stripe ids: %w", updateErr)
	}

	payment.StripePaymentIntentID = result.PaymentIntentID
	payment.StripeCheckoutSessionID = result.SessionID

	return &CreatePaymentResult{
		Payment:     payment,
		CheckoutURL: result.SessionURL,
	}, nil
}

// GetPaymentStatus fetches the current payment record for an order.
func (s *PaymentService) GetPaymentStatus(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	return s.repo.FindByOrderID(ctx, orderID)
}

// RefundPayment issues a Stripe refund and transitions the local record to refunded.
// Returns the payment, the Stripe refund ID, and any error.
func (s *PaymentService) RefundPayment(
	ctx context.Context,
	orderID uuid.UUID,
	reason string,
) (*model.Payment, string, error) {
	payment, err := s.repo.FindByOrderID(ctx, orderID)
	if err != nil {
		return nil, "", fmt.Errorf("find payment for refund: %w", err)
	}

	refundID, err := s.stripe.Refund(ctx, payment.StripePaymentIntentID, reason)
	if err != nil {
		return nil, "", fmt.Errorf("stripe refund: %w", err)
	}

	if updateErr := s.repo.UpdateStatus(ctx, orderID, model.PaymentStatusRefunded); updateErr != nil {
		return nil, "", fmt.Errorf("update refunded status: %w", updateErr)
	}

	payment.Status = model.PaymentStatusRefunded
	return payment, refundID, nil
}
