# Fix Stripe Webhook Payment Lookup

**Date:** 2026-04-23
**Status:** Draft
**Scope:** Payment-service webhook handler + outbox poller resilience

## Problem

When a customer completes Stripe checkout, the `payment_intent.succeeded` webhook fires. The payment-service handler calls `FindByStripeIntentID(intentID)` to locate the payment record. However, `stripe_payment_intent_id` is never stored — Stripe doesn't include the PaymentIntent on the Checkout Session object at creation time (`sess.PaymentIntent` is nil).

**Result:** The handler can't find the payment, returns an error, no outbox message is written, the `payment.confirmed` saga event never publishes, and the order stays stuck at `PAYMENT_CREATED` with the cart never cleared.

**Secondary issue:** The outbox poller fails with connection refused errors to paymentdb during startup, tripping the circuit breaker. This creates a window where even if the webhook handler wrote to the outbox, the poller couldn't publish messages.

## Root Cause Analysis

1. `go/payment-service/internal/stripe/client.go:63-66` — `sess.PaymentIntent` is nil for Checkout Sessions, so `intentID` is empty
2. `go/payment-service/internal/service/stripe.go:97` — `UpdateStripeIDs` saves the empty intent ID to the DB
3. `go/payment-service/internal/service/webhook.go:65` — `FindByStripeIntentID("")` finds nothing
4. `go/payment-service/internal/outbox/poller.go:51-63` — no startup readiness check before polling

**Evidence from production:**
- Payment records for orders `f5cd888c` and `db98de34` have `stripe_checkout_session_id` populated but `stripe_payment_intent_id` empty
- Webhook log shows `payment_intent.succeeded` received with `intentID: pi_3TPQ...` but no follow-up processing
- Outbox table is empty — webhook handler error prevented the write
- Outbox poller logs show `"dependency temporarily unavailable: payment-postgres"` errors

## Solution

### 1. Extend EventVerifier to return metadata

**File:** `go/payment-service/internal/stripe/verifier.go`

Change `VerifyAndParse` return signature to include `metadata map[string]string`. For `payment_intent.*` events, extract `Metadata` from the PaymentIntent in `event.Data.Raw`. For `charge.*` events, return nil metadata.

**Interface update in:** `go/payment-service/internal/handler/webhook.go` — `EventVerifier` interface gets the updated signature.

### 2. Webhook service looks up by order_id from metadata

**File:** `go/payment-service/internal/service/webhook.go`

`HandlePaymentSucceeded(ctx, eventID, intentID, metadata)`:
1. Dedup check (unchanged)
2. Parse `metadata["order_id"]` as UUID — return error if missing
3. Call `FindByOrderID(ctx, orderID)` instead of `FindByStripeIntentID`
4. Backfill `stripe_payment_intent_id` via `UpdateStripeIDs(ctx, orderID, intentID, payment.StripeCheckoutSessionID)`
5. Update status to succeeded + write outbox message (unchanged)

Same change for `HandlePaymentFailed`.

`HandleRefund` stays unchanged — refunds happen after payment succeeded, so the intent ID is already backfilled by that point.

### 3. Handler passes metadata through

**File:** `go/payment-service/internal/handler/webhook.go`

`HandleWebhook` receives `metadata` from `VerifyAndParse` and passes it to `HandlePaymentSucceeded`/`HandlePaymentFailed`.

`WebhookService` interface updates to accept `metadata map[string]string` on the relevant methods.

### 4. Outbox poller startup resilience

**File:** `go/payment-service/internal/outbox/poller.go`

Add a `waitForDB` method that pings postgres with exponential backoff (1s, 2s, 4s, 8s... capped at 30s) before entering the polling loop. The `Run` method calls `waitForDB(ctx)` first.

The `OutboxFetcher` interface needs a `Ping(ctx) error` method, implemented in `repository/outbox.go`.

### 5. Manual DB cleanup (post-deploy)

```sql
-- Mark stuck PAYMENT_CREATED orders as failed
UPDATE orders SET status = 'failed', saga_step = 'COMPENSATION_COMPLETE'
WHERE status = 'pending' AND saga_step = 'PAYMENT_CREATED';

-- Mark stuck COMPENSATING orders as failed
UPDATE orders SET status = 'failed', saga_step = 'COMPENSATION_COMPLETE'
WHERE status = 'failed' AND saga_step = 'COMPENSATING';
```

## Files Changed

| File | Change |
|------|--------|
| `go/payment-service/internal/stripe/verifier.go` | Return metadata from VerifyAndParse |
| `go/payment-service/internal/handler/webhook.go` | Update EventVerifier + WebhookService interfaces, pass metadata |
| `go/payment-service/internal/service/webhook.go` | Accept metadata, lookup by order_id, backfill intent ID |
| `go/payment-service/internal/outbox/poller.go` | Add waitForDB startup check |
| `go/payment-service/internal/repository/outbox.go` | Add Ping method to OutboxFetcher |
| `go/payment-service/internal/stripe/verifier_test.go` | Test metadata extraction |
| `go/payment-service/internal/handler/webhook_test.go` | Update mock interfaces, test metadata passthrough |
| `go/payment-service/internal/service/webhook_test.go` | Test order_id lookup path, backfill |

## Verification

1. `make preflight-go` — lint + tests pass
2. Deploy to QA
3. Place a test order, complete Stripe checkout
4. Verify in Loki: `"payment succeeded"` log with orderID
5. Verify in DB: order status = `completed`, saga_step = `COMPLETED`, cart empty
6. Verify outbox poller starts cleanly on pod restart (check for absence of circuit breaker errors in first 30s)

## Out of Scope

- Database rename (ecommercedb) — separate spec
- Admin replay endpoint for stuck orders — manual SQL cleanup is sufficient
- Stripe dashboard webhook configuration changes — `payment_intent.succeeded` is already configured
