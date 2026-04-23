DROP INDEX IF EXISTS payments_stripe_intent_id_unique;

ALTER TABLE payments ADD CONSTRAINT payments_stripe_payment_intent_id_key UNIQUE (stripe_payment_intent_id);
