-- Replace the blanket UNIQUE constraint on stripe_payment_intent_id with a
-- partial unique index that only enforces uniqueness on non-empty values.
-- This allows multiple pending payments (with NULL/empty intent IDs) to coexist
-- during saga retries while still preventing duplicate real Stripe intents.

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_stripe_payment_intent_id_key;

-- migration-lint: ignore=MIG001 reason="early-stage migration; ran on empty payments in dev/QA before launch — would be CONCURRENTLY today"
CREATE UNIQUE INDEX IF NOT EXISTS payments_stripe_intent_id_unique
    ON payments (stripe_payment_intent_id)
    WHERE stripe_payment_intent_id IS NOT NULL AND stripe_payment_intent_id != '';
