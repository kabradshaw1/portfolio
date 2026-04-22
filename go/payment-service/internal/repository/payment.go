package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var errNilPool = fmt.Errorf("database pool is nil")

var ErrPaymentNotFound = apperror.NotFound("PAYMENT_NOT_FOUND", "payment not found")

type PaymentRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewPaymentRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *PaymentRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &PaymentRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

// IdempotencyKey returns the idempotency key for a given order ID.
func IdempotencyKey(orderID uuid.UUID) string {
	return "payment:" + orderID.String()
}

func (r *PaymentRepository) Create(ctx context.Context, orderID uuid.UUID, amountCents int, currency string) (*model.Payment, error) {
	if r.pool == nil {
		return nil, errNilPool
	}
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Payment, error) {
		var p model.Payment
		err := r.pool.QueryRow(ctx,
			`INSERT INTO payments (id, order_id, amount_cents, currency, status, idempotency_key, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'pending', $5, NOW(), NOW())
			 RETURNING id, order_id, COALESCE(stripe_payment_intent_id, ''), COALESCE(stripe_checkout_session_id, ''),
			           amount_cents, currency, status, idempotency_key, created_at, updated_at`,
			uuid.New(), orderID, amountCents, currency, IdempotencyKey(orderID),
		).Scan(
			&p.ID, &p.OrderID, &p.StripePaymentIntentID, &p.StripeCheckoutSessionID,
			&p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("create payment: %w", err)
		}
		return &p, nil
	})
}

func (r *PaymentRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) (*model.Payment, error) {
	if r.pool == nil {
		return nil, errNilPool
	}
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Payment, error) {
		var p model.Payment
		var intentID, sessionID *string
		err := r.pool.QueryRow(ctx,
			`SELECT id, order_id, stripe_payment_intent_id, stripe_checkout_session_id,
			        amount_cents, currency, status, idempotency_key, created_at, updated_at
			 FROM payments WHERE order_id = $1`,
			orderID,
		).Scan(
			&p.ID, &p.OrderID, &intentID, &sessionID,
			&p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPaymentNotFound
			}
			return nil, fmt.Errorf("find payment by order id: %w", err)
		}
		if intentID != nil {
			p.StripePaymentIntentID = *intentID
		}
		if sessionID != nil {
			p.StripeCheckoutSessionID = *sessionID
		}
		return &p, nil
	})
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.PaymentStatus) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		result, err := r.pool.Exec(ctx,
			"UPDATE payments SET status = $1, updated_at = NOW() WHERE order_id = $2",
			status, orderID,
		)
		if err != nil {
			return fmt.Errorf("update payment status: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrPaymentNotFound
		}
		return nil
	})
}

func (r *PaymentRepository) UpdateStripeIDs(ctx context.Context, orderID uuid.UUID, intentID, sessionID string) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		result, err := r.pool.Exec(ctx,
			"UPDATE payments SET stripe_payment_intent_id = $1, stripe_checkout_session_id = $2, updated_at = NOW() WHERE order_id = $3",
			intentID, sessionID, orderID,
		)
		if err != nil {
			return fmt.Errorf("update stripe ids: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrPaymentNotFound
		}
		return nil
	})
}

func (r *PaymentRepository) FindByStripeIntentID(ctx context.Context, intentID string) (*model.Payment, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Payment, error) {
		var p model.Payment
		var intentIDPtr, sessionID *string
		err := r.pool.QueryRow(ctx,
			`SELECT id, order_id, stripe_payment_intent_id, stripe_checkout_session_id,
			        amount_cents, currency, status, idempotency_key, created_at, updated_at
			 FROM payments WHERE stripe_payment_intent_id = $1`,
			intentID,
		).Scan(
			&p.ID, &p.OrderID, &intentIDPtr, &sessionID,
			&p.AmountCents, &p.Currency, &p.Status, &p.IdempotencyKey, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPaymentNotFound
			}
			return nil, fmt.Errorf("find payment by stripe intent id: %w", err)
		}
		if intentIDPtr != nil {
			p.StripePaymentIntentID = *intentIDPtr
		}
		if sessionID != nil {
			p.StripeCheckoutSessionID = *sessionID
		}
		return &p, nil
	})
}
