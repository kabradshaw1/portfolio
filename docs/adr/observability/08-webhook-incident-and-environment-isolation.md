# Webhook Incident: Silent Failures and Environment Isolation

- **Date:** 2026-04-23
- **Status:** Accepted

## Context

A production bug surfaced where Stripe checkout completed successfully but orders stayed stuck at `PAYMENT_CREATED` — the cart was never cleared and orders never completed. The root cause was straightforward (the `stripe_payment_intent_id` was never stored, so the webhook handler couldn't find the payment record), but discovering it took far longer than it should have.

This was the second major debugging incident in two days (following the mTLS handshake failure documented in ADR 07). Where ADR 07 exposed instrumentation gaps in the decomposed services, this incident exposed gaps in the middleware itself and in the QA environment's isolation from production.

### The debugging timeline

1. **Initial symptom:** Customer completes Stripe checkout, redirected back to the store, cart still full, no order confirmation.
2. **Loki query for errors:** No ERROR logs found across the entire `go-ecommerce` namespace for 24 hours. The webhook handler returned a 500, but the `apperror.ErrorHandler()` middleware only logged unknown errors — `AppError` instances were silently converted to JSON responses.
3. **Traced the order through saga logs:** Found the saga advanced to `STOCK_VALIDATED`, payment-service called Stripe, webhook received `payment_intent.succeeded` — but the saga never advanced past that point.
4. **Checked the payment database:** `stripe_payment_intent_id` was empty on all payment records. The Stripe Go SDK returns nil for `sess.PaymentIntent` at checkout session creation time.
5. **Identified the lookup failure:** `HandlePaymentSucceeded` called `FindByStripeIntentID(intentID)` — no match because the ID was never stored. The handler errored, no outbox message was written, the saga stalled.
6. **Fixed the webhook handler:** Extracted `order_id` from PaymentIntent metadata, used `FindByOrderID` instead, backfilled the intent ID for future refund lookups.
7. **Deployed fix, tested — cart still not cleared:** The fix worked (saga completed in QA), but the `clear.cart` command was consumed by the production cart-service instead of QA. Both environments shared the same RabbitMQ instance and identical queue names.
8. **Fixed environment isolation:** Created a `/qa` vhost on RabbitMQ, updated QA service configs to use it.
9. **Tested again — checkout returned 500:** `order_items` table had a foreign key to a `products` table in `ecommercedb` — a monolith relic. Products had been extracted to `productdb`, so the FK referenced stale data. Dropped the constraint.
10. **Tested again — checkout worked, orders page crashed:** Frontend called `.map()` on the API response object instead of `response.orders`. Fixed the destructuring.

Four distinct bugs, discovered sequentially because each one masked the next. Total debugging time: ~3 hours.

## Decision

### 1. Log 5xx AppErrors in middleware

The `apperror.ErrorHandler()` middleware in `go/pkg/apperror/middleware.go` now logs all AppErrors with `HTTPStatus >= 500` via `slog.Error` before sending the JSON response. Fields logged: error code, message, HTTP status, request ID.

4xx errors remain silent — they represent client mistakes, not server failures.

**Impact:** This single change would have reduced the initial investigation from ~45 minutes to under 5 minutes. The webhook handler's 500 would have appeared immediately in Loki with the exact error message.

### 2. Fix payment-service OTEL endpoint

Payment-service config pointed to `jaeger-collector.monitoring.svc.cluster.local:4317` while all other services used `jaeger.monitoring.svc.cluster.local:4317`. This caused continuous "traces export: exporter export timeout" log spam that drowned out real signals.

Fixed by correcting the endpoint to match other services.

### 3. Add saga-order-stalled alert

New Grafana alert rule fires when `saga_steps_total{step="PAYMENT_CREATED"}` increases but neither `COMPLETED` nor `COMPENSATION_COMPLETE` increases within 30 minutes. Routes to Telegram.

This would have caught the stuck orders within 30 minutes of the first failed checkout, instead of waiting for a manual report.

### 4. Webhook event type dashboard breakdown

Updated the "Payment Webhooks" Grafana panel to group by `event_type` and `outcome` instead of just `outcome`. Now shows separate lines for `payment_intent.succeeded`, `payment_intent.payment_failed`, and `charge.refunded`.

### 5. QA environment RabbitMQ isolation via vhost

QA and production shared the same RabbitMQ instance (`rabbitmq.java-tasks.svc.cluster.local:5672`) with identical queue names. Both environments' consumers competed for the same messages — a QA saga command could be consumed by a production service (and vice versa).

Created a `/qa` vhost on RabbitMQ with full permissions. Updated all QA Kustomize overlay RABBITMQ_URL entries (order-service, cart-service, payment-service) to `amqp://...5672/qa`. Services auto-declare their exchanges and queues on the new vhost at startup.

**Alternative considered:** Separate queue names via environment variable suffix. Rejected because it would require code changes in every service's topology declaration, consumer setup, and publisher. The vhost approach is a config-only change.

### 6. Drop stale monolith foreign key

The `order_items` table had `REFERENCES products(id)` — a constraint from when orders and products shared `ecommercedb`. After the product-service extraction to `productdb`, this FK pointed at a stale `products` table that didn't contain the current catalog. New migration `012_drop_product_fk` removes the constraint.

## Consequences

**Positive:**
- Server errors are now always visible in Loki — silent 500s are impossible
- Payment-service traces export cleanly to Jaeger — no more log noise
- Stuck orders trigger alerts within 30 minutes
- QA saga flow is fully isolated from production — no more cross-environment message consumption
- Webhook dashboard shows per-event-type health at a glance
- Order creation no longer blocked by stale FK constraint

**Trade-offs:**
- Slightly more log volume from 5xx AppError logging (bounded by error rate, not request rate)
- RabbitMQ vhost requires manual creation on fresh clusters (documented in CLAUDE.md)

**Lessons:**
- The middleware is the last line of defense for error visibility. If it doesn't log, errors vanish regardless of how well individual services are instrumented.
- Shared infrastructure between environments (databases, message brokers, caches) is a class of bug that's invisible until it manifests as "the fix didn't work." Each shared resource needs explicit environment isolation.
- Monolith-era constraints (FKs, shared tables) linger long after decomposition. Every migration should audit cross-service references.
- Sequential bugs — where fixing one reveals the next — are the most expensive to debug. Each layer of the stack needs independent testability.
