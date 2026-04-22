CREATE TABLE payments (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id                   UUID NOT NULL UNIQUE,
    stripe_payment_intent_id   TEXT UNIQUE,
    stripe_checkout_session_id TEXT,
    amount_cents               INTEGER NOT NULL CHECK (amount_cents > 0),
    currency                   TEXT NOT NULL DEFAULT 'usd',
    status                     TEXT NOT NULL DEFAULT 'pending',
    idempotency_key            TEXT NOT NULL UNIQUE,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_status ON payments(status);
