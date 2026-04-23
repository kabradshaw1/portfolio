package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
)

// ProcessedEventRepo records processed Stripe event IDs for idempotency.
type ProcessedEventRepo interface {
	TryInsert(ctx context.Context, tx pgx.Tx, eventID, eventType string) (bool, error)
}

// OutboxRepo writes outbound messages to the transactional outbox.
type OutboxRepo interface {
	Insert(ctx context.Context, tx pgx.Tx, exchange, routingKey string, payload []byte) error
}

// TxBeginner opens a database transaction. Kept for future transactional upgrades.
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WebhookService processes Stripe webhook events with idempotency and outbox writes.
type WebhookService struct {
	paymentRepo PaymentRepo
	eventRepo   ProcessedEventRepo
	outboxRepo  OutboxRepo
	txBeginner  TxBeginner
}

// NewWebhookService creates a WebhookService.
func NewWebhookService(
	paymentRepo PaymentRepo,
	eventRepo ProcessedEventRepo,
	outboxRepo OutboxRepo,
	txBeginner TxBeginner,
) *WebhookService {
	return &WebhookService{
		paymentRepo: paymentRepo,
		eventRepo:   eventRepo,
		outboxRepo:  outboxRepo,
		txBeginner:  txBeginner,
	}
}

// HandlePaymentSucceeded deduplicates the event, updates payment status to succeeded,
// and writes a saga confirmation event to the outbox.
func (s *WebhookService) HandlePaymentSucceeded(ctx context.Context, eventID, intentID string, metadata map[string]string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.succeeded")
	if err != nil {
		return fmt.Errorf("dedup payment succeeded: %w", err)
	}
	if !inserted {
		metrics.WebhookEvents.WithLabelValues("payment_intent.succeeded", "duplicate").Inc()
		slog.InfoContext(ctx, "duplicate payment_intent.succeeded event, skipping", "eventID", eventID)
		return nil
	}

	orderID, err := orderIDFromMetadata(metadata)
	if err != nil {
		return fmt.Errorf("payment succeeded metadata: %w", err)
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find payment for succeeded event: %w", err)
	}

	// Backfill Stripe intent ID for future refund lookups.
	if intentID != "" {
		if backfillErr := s.paymentRepo.UpdateStripeIDs(ctx, orderID, intentID, payment.StripeCheckoutSessionID); backfillErr != nil {
			slog.ErrorContext(ctx, "failed to backfill stripe intent ID", "orderID", orderID, "intentID", intentID, "error", backfillErr)
		}
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusSucceeded); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "succeeded", "error", err)
		return fmt.Errorf("update payment status to succeeded: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.confirmed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment confirmed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment confirmed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment succeeded", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}

// HandlePaymentFailed deduplicates the event, updates payment status to failed,
// and writes a saga failure event to the outbox.
func (s *WebhookService) HandlePaymentFailed(ctx context.Context, eventID, intentID string, metadata map[string]string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "payment_intent.payment_failed")
	if err != nil {
		return fmt.Errorf("dedup payment failed: %w", err)
	}
	if !inserted {
		metrics.WebhookEvents.WithLabelValues("payment_intent.payment_failed", "duplicate").Inc()
		slog.InfoContext(ctx, "duplicate payment_intent.payment_failed event, skipping", "eventID", eventID)
		return nil
	}

	orderID, err := orderIDFromMetadata(metadata)
	if err != nil {
		return fmt.Errorf("payment failed metadata: %w", err)
	}

	payment, err := s.paymentRepo.FindByOrderID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("find payment for failed event: %w", err)
	}

	// Backfill Stripe intent ID for future refund lookups.
	if intentID != "" {
		if backfillErr := s.paymentRepo.UpdateStripeIDs(ctx, orderID, intentID, payment.StripeCheckoutSessionID); backfillErr != nil {
			slog.ErrorContext(ctx, "failed to backfill stripe intent ID", "orderID", orderID, "intentID", intentID, "error", backfillErr)
		}
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusFailed); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "failed", "error", err)
		return fmt.Errorf("update payment status to failed: %w", err)
	}

	payload, err := json.Marshal(map[string]string{
		"event":    "payment.failed",
		"order_id": payment.OrderID.String(),
	})
	if err != nil {
		return fmt.Errorf("marshal payment failed outbox payload: %w", err)
	}

	if err := s.outboxRepo.Insert(ctx, nil, "ecommerce.saga", "saga.order.events", payload); err != nil {
		return fmt.Errorf("insert payment failed outbox message: %w", err)
	}

	slog.InfoContext(ctx, "payment failed", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}

// HandleRefund deduplicates the event and updates payment status to refunded.
// No saga event is written — refunds do not flow through the saga.
func (s *WebhookService) HandleRefund(ctx context.Context, eventID, intentID string) error {
	inserted, err := s.eventRepo.TryInsert(ctx, nil, eventID, "charge.refunded")
	if err != nil {
		return fmt.Errorf("dedup refund: %w", err)
	}
	if !inserted {
		metrics.WebhookEvents.WithLabelValues("charge.refunded", "duplicate").Inc()
		slog.InfoContext(ctx, "duplicate charge.refunded event, skipping", "eventID", eventID)
		return nil
	}

	payment, err := s.paymentRepo.FindByStripeIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find payment for refund event: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(ctx, payment.OrderID, model.PaymentStatusRefunded); err != nil {
		slog.ErrorContext(ctx, "failed to update payment status", "orderID", payment.OrderID, "status", "refunded", "error", err)
		return fmt.Errorf("update payment status to refunded: %w", err)
	}

	slog.InfoContext(ctx, "payment refunded", "orderID", payment.OrderID, "intentID", intentID)
	return nil
}

// orderIDFromMetadata extracts and parses the order_id UUID from PaymentIntent metadata.
func orderIDFromMetadata(metadata map[string]string) (uuid.UUID, error) {
	raw, ok := metadata["order_id"]
	if !ok || raw == "" {
		return uuid.Nil, fmt.Errorf("order_id not found in payment intent metadata")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid order_id in metadata: %w", err)
	}
	return id, nil
}
