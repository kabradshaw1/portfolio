package composite

import (
	"context"
	"database/sql"
	"errors"
)

// ─── OrderSource ──────────────────────────────────────────────────────────────
//
// Schema (orderdb.orders — partitioned by created_at):
//
//	id         UUID  PRIMARY KEY (composite with created_at)
//	user_id    UUID
//	status     VARCHAR(20)
//	saga_step  TEXT
//	total      INTEGER
//	created_at TIMESTAMPTZ
//	updated_at TIMESTAMPTZ
//
// NOTE: orders has no trace_id or correlation_id columns. Those fields on
// OrderRecord will always be empty strings when populated from Postgres. They
// are provided by Jaeger / OTel context at the application layer, not stored
// in the DB.

// PostgresOrderSource fetches the primary order row from orderdb.
type PostgresOrderSource struct{ DB *sql.DB }

func (p PostgresOrderSource) FetchOrder(ctx context.Context, id string) (OrderRecord, error) {
	const q = `
SELECT id,
       status,
       COALESCE(user_id::text, ''),
       COALESCE(saga_step, ''),
       EXTRACT(EPOCH FROM created_at)::bigint,
       EXTRACT(EPOCH FROM updated_at)::bigint
FROM orders
WHERE id = $1
LIMIT 1`

	var r OrderRecord
	r.ID = id
	var sagaStep string
	err := p.DB.QueryRowContext(ctx, q, id).
		Scan(&r.ID, &r.Status, &r.UserID, &sagaStep, &r.CreatedUnix, &r.UpdatedUnix)
	if err != nil {
		return OrderRecord{}, err
	}
	// TraceID and CorrelationID are not stored in the DB; leave as empty strings.
	return r, nil
}

// ─── SagaSource ───────────────────────────────────────────────────────────────
//
// The order-service does not have a separate saga_state table. The saga step is
// a single TEXT column (saga_step) on the orders table. SagaHistory.Events and
// Retries cannot be populated from the DB — they are left at zero values.
// A missing row (sql.ErrNoRows) returns an empty SagaHistory with no error.

// PostgresSagaSource fetches the saga step from orderdb.orders.
type PostgresSagaSource struct{ DB *sql.DB }

func (p PostgresSagaSource) FetchSaga(ctx context.Context, orderID string) (SagaHistory, error) {
	const q = `
SELECT COALESCE(saga_step, '')
FROM orders
WHERE id = $1
LIMIT 1`

	var step string
	err := p.DB.QueryRowContext(ctx, q, orderID).Scan(&step)
	if errors.Is(err, sql.ErrNoRows) {
		return SagaHistory{}, nil
	}
	if err != nil {
		return SagaHistory{}, err
	}
	return SagaHistory{Step: step}, nil
}

// ─── PaymentSource ────────────────────────────────────────────────────────────
//
// Schema (paymentdb.payments):
//
//	id                         UUID
//	order_id                   UUID  UNIQUE
//	stripe_payment_intent_id   TEXT  (maps to PaymentRecord.StripeChargeID)
//	stripe_checkout_session_id TEXT
//	amount_cents               INTEGER
//	currency                   TEXT
//	status                     TEXT  ("succeeded" → WebhookReceived=true)
//	idempotency_key            TEXT
//	created_at                 TIMESTAMPTZ
//	updated_at                 TIMESTAMPTZ
//
// NOTE: paymentdb also has a generic outbox table whose payload is JSONB — no
// Stripe-specific columns there. We read from payments directly.
// PaymentRecord.WebhookReceived is approximated from status == "succeeded";
// there is no dedicated webhook_received boolean in the schema.
// A missing row returns a zero PaymentRecord with no error.

// PostgresPaymentSource fetches the payment row from paymentdb.
type PostgresPaymentSource struct{ DB *sql.DB }

func (p PostgresPaymentSource) FetchPayment(ctx context.Context, orderID string) (PaymentRecord, error) {
	const q = `
SELECT COALESCE(stripe_payment_intent_id, ''),
       status
FROM payments
WHERE order_id = $1
LIMIT 1`

	var chargeID, status string
	err := p.DB.QueryRowContext(ctx, q, orderID).Scan(&chargeID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentRecord{}, nil
	}
	if err != nil {
		return PaymentRecord{}, err
	}
	return PaymentRecord{
		StripeChargeID:  chargeID,
		WebhookReceived: status == "succeeded",
	}, nil
}

// ─── CartSource ───────────────────────────────────────────────────────────────
//
// Schema (cartdb.cart_items):
//
//	id         UUID
//	user_id    UUID
//	product_id UUID
//	quantity   INTEGER
//	created_at TIMESTAMPTZ
//	reserved   BOOLEAN
//
// There is no cart_reservations table. Cart holds are tracked via the reserved
// boolean column on individual cart_items rows. CartReservation.Released is
// true when no reserved items remain for the given user.
//
// NOTE: The cart_items table is keyed by user_id, not order_id. The investigate
// tool supplies order_id — this adapter queries for any items still reserved for
// users, but without a user_id↔order_id mapping in cartdb the query cannot be
// exact. We query using the order_id as user_id (a known limitation); in
// practice the investigate tool will receive correct user_id from the order row
// once main.go is wired (task A6). The interface signature accepts a string id
// that the caller controls — document this limitation here.
//
// A missing row (no items at all) returns CartReservation{Released: true}.

// PostgresCartSource fetches the reservation status from cartdb.cart_items.
type PostgresCartSource struct{ DB *sql.DB }

func (p PostgresCartSource) FetchCartReservation(ctx context.Context, userID string) (CartReservation, error) {
	const q = `
SELECT COUNT(*) FROM cart_items
WHERE user_id = $1 AND reserved = true`

	var reservedCount int
	err := p.DB.QueryRowContext(ctx, q, userID).Scan(&reservedCount)
	if err != nil {
		return CartReservation{}, err
	}
	// ReleasedAt is not stored in the DB — left at zero.
	return CartReservation{Released: reservedCount == 0}, nil
}
