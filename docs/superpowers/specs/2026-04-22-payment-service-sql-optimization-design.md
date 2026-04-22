# Payment Service + SQL Optimization Roadmap

**Date:** 2026-04-22
**Status:** Proposed
**Goal:** Strengthen portfolio coverage of Stripe payment processing, SQL optimization, and messaging patterns for backend Go developer roles.

## Overview

Two independent tracks that target the highest gaps in the current portfolio:

- **Track 1 — Payment Service + Saga Evolution:** Real Stripe integration with webhook handling, transactional outbox, and saga compensation with refunds.
- **Track 2 — SQL Optimization + Reporting:** Table partitioning, materialized views, CTE-based reporting queries, and a benchmark suite proving performance impact.

The tracks are independently buildable (separate feature branches) but share integration points through Kafka events and the order-service database.

---

## Track 1: Payment Service + Saga Evolution

### New Service: `go/payment-service`

A standalone Go microservice (REST :8098, gRPC :9098) that owns all payment logic. Follows existing service patterns — `cmd/server/`, `internal/`, migrations, shared `pkg/`.

**Responsibilities:**
- Create Stripe Checkout Sessions for order payment
- Receive and verify Stripe webhooks at `POST /webhooks/stripe`
- Store payment records in `paymentdb`
- Expose gRPC for the order-service saga (CreatePayment, GetPaymentStatus, RefundPayment)
- Idempotency keys on all Stripe API calls (derived from order ID to prevent double charges)

**Refund flow:**
- When a return is approved in order-service, it calls `payment-service.RefundPayment` via gRPC
- Payment-service calls Stripe's refund API, updates local record, publishes Kafka event

### Webhook Handling

**Endpoint:** `POST /webhooks/stripe` with Stripe signature verification (`stripe.ConstructEvent` with webhook signing secret).

**Idempotent processing:** Store Stripe event ID in a `processed_events` table with a unique constraint. If the insert conflicts, skip the event.

**Events handled:**
- `payment_intent.succeeded` — update payment record, publish RabbitMQ event to `saga.order.events`
- `payment_intent.payment_failed` — update status, publish failure event, orchestrator compensates
- `charge.refunded` — update payment record, publish Kafka event for analytics

### Transactional Outbox

Rather than publishing to RabbitMQ directly from the webhook handler (risking DB commit success + publish failure), write the outbound message to an `outbox` table in the same transaction as the payment status update. A background poller reads the outbox and publishes to RabbitMQ.

This guarantees at-least-once delivery without distributed transactions.

### Saga Evolution

**Current saga:** reserve items → validate stock → clear cart → complete

**New saga:** reserve items → validate stock → create payment → await webhook → clear cart → complete

New saga steps:
- `STOCK_VALIDATED` → publish command to payment-service (create Stripe Checkout Session / payment intent)
- `PAYMENT_CREATED` → wait for webhook confirmation event via RabbitMQ
- `PAYMENT_CONFIRMED` → proceed to clear cart
- `PAYMENT_FAILED` → compensate (release reserved items, no charge)

**Compensation with refunds:** If the saga fails after payment succeeds (e.g., cart clear fails), the orchestrator calls `RefundPayment` via gRPC before releasing items.

### Data Model (`paymentdb`)

```sql
-- payments: one payment per order
CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL UNIQUE,
    stripe_payment_intent_id TEXT UNIQUE,
    stripe_checkout_session_id TEXT,
    amount_cents    INTEGER NOT NULL CHECK (amount_cents > 0),
    currency        TEXT NOT NULL DEFAULT 'usd',
    status          TEXT NOT NULL DEFAULT 'pending',
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- processed_events: webhook deduplication
CREATE TABLE processed_events (
    stripe_event_id TEXT PRIMARY KEY,
    event_type      TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- outbox: transactional outbox for reliable messaging
CREATE TABLE outbox (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange    TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload     JSONB NOT NULL,
    published   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**Indexes:**
- `payments.order_id` — unique, lookup by order
- `payments.stripe_payment_intent_id` — unique, webhook lookup
- `outbox` partial index: `CREATE INDEX idx_outbox_unpublished ON outbox (created_at) WHERE published = false`

**Migrations:** 3 files using `golang-migrate` (create tables, add indexes, add constraints).

### Proto Definition

```protobuf
// go/proto/payment/v1/payment.proto
service PaymentService {
    rpc CreatePayment(CreatePaymentRequest) returns (CreatePaymentResponse);
    rpc GetPaymentStatus(GetPaymentStatusRequest) returns (GetPaymentStatusResponse);
    rpc RefundPayment(RefundPaymentRequest) returns (RefundPaymentResponse);
}
```

### Infrastructure

- New `paymentdb` on the shared Postgres instance
- K8s deployment, service, configmap, migration job, HPA, PDB in `go/k8s/`
- QA database (`paymentdb_qa`) and Kustomize overlay
- CI matrix updates (lint, test, build, Hadolint)
- Ingress path: `/go-payments/*`
- Stripe secrets (API key, webhook signing secret) via K8s Secrets
- New Kafka topic: `ecommerce.payments`

---

## Track 2: SQL Optimization + Reporting

### Table Partitioning

Range-partition the `orders` table by `created_at` (monthly partitions).

**Migration strategy:** PostgreSQL doesn't support in-place conversion to partitioned tables. The migration:
1. Rename `orders` to `orders_old`
2. Create `orders` as a partitioned table (same schema, `PARTITION BY RANGE (created_at)`)
3. Create monthly partition tables (`orders_2026_01`, `orders_2026_02`, etc.)
4. Copy data from `orders_old` into the partitioned table
5. Re-create indexes and constraints on the partitioned table
6. Drop `orders_old`

**Partition maintenance:** A goroutine in order-service that runs on startup and periodically (daily) creates partitions 3 months ahead so inserts never fail.

**Existing queries are unchanged** — PostgreSQL routes automatically. Queries with `WHERE created_at BETWEEN ...` benefit from partition pruning, provable via `EXPLAIN ANALYZE`.

### Materialized Views

Pre-computed reporting views refreshed on a schedule:

**`mv_daily_revenue`** — daily revenue by product and category:
```sql
SELECT
    date_trunc('day', o.created_at) AS day,
    oi.product_id,
    p.name AS product_name,
    p.category,
    SUM(oi.quantity * oi.price_at_purchase) AS revenue_cents,
    SUM(oi.quantity) AS units_sold,
    COUNT(DISTINCT o.id) AS order_count
FROM orders o
JOIN order_items oi ON oi.order_id = o.id
JOIN products p ON p.id = oi.product_id
WHERE o.status = 'COMPLETED'
GROUP BY 1, 2, 3, 4;
```

**`mv_product_performance`** — units sold, revenue, return rate, average order value per product.

**`mv_customer_summary`** — order count, total spend, first/last order date per user (customer lifetime value proxy).

**Refresh strategy:** A goroutine in order-service runs `REFRESH MATERIALIZED VIEW CONCURRENTLY` every 15 minutes. `CONCURRENTLY` requires a unique index on each view — prevents blocking reads during refresh.

### Complex Reporting Queries

REST endpoints on order-service for reporting:

- **Sales trends** — rolling 7-day and 30-day revenue using window functions (`SUM() OVER (ORDER BY date ROWS BETWEEN ...)`)
- **Inventory turnover** — units sold vs. current stock per product over a time range, ranked by turnover rate
- **Top customers** — CTE joining orders → order_items, computing total spend, order frequency, average order value, ranked with `DENSE_RANK()`. If Track 1 is complete, extends to include payment status dimensions (refund ratios, payment method breakdown).

These query the partitioned tables and materialized views, demonstrating the optimization stack working together. Track 2 is fully functional without Track 1 — payment dimensions are additive enhancements, not dependencies.

### Benchmark Suite

A `_bench_test.go` file using `go/pkg/dbtest` with testcontainers:

- Seeds 10k+ orders across multiple partitions with realistic distribution
- Runs each reporting query with `EXPLAIN ANALYZE` captured in test output
- Benchmarks the same queries with and without partitioning (non-partitioned copy of same data)
- Benchmarks materialized view reads vs. live aggregation queries
- Outputs wall-clock times and row scan counts (e.g., "partitioning reduced scan from 50k rows to 3k rows")

---

## Integration Points

- **Kafka:** Payment-service publishes `payment.succeeded`, `payment.failed`, `payment.refunded` to `ecommerce.payments`. Analytics-service gets a new consumer for that topic.
- **Materialized views** include payment dimensions once Track 1 data flows (revenue by payment status, refund ratios).
- **Order-service** is the shared touchpoint: Track 1 extends its saga, Track 2 adds partitioning and views to its database. Separate migrations, separate code paths — no conflicts during parallel development.

## Out of Scope

- No frontend payment UI — Stripe Checkout Sessions redirect to Stripe-hosted page and back
- No subscription/recurring billing — one-time payments only
- No read replicas or multi-region
- No new Kafka topics beyond `ecommerce.payments`
- No changes to auth-service, product-service, or cart-service
- No frontend changes beyond wiring up the Stripe redirect flow

## New Service Checklist (Track 1)

Per the CLAUDE.md checklist for adding a new Go service:

1. Service code: `go/payment-service/` with cmd/server, internal/, go.mod, Dockerfile
2. Proto: `go/proto/payment/v1/payment.proto`, `buf generate`
3. Cross-module imports: order-service needs payment proto — add replace directive + COPY in Dockerfile
4. K8s manifests: deployment, service, configmap, migration job, HPA, PDB
5. QA database: `paymentdb_qa` created manually on Debian server
6. QA Kustomize overlay: ConfigMap patch in `k8s/overlays/qa-go/kustomization.yaml`
7. CI matrices: Add to `go-lint`, `go-tests`, `build-images`, `security-hadolint`
8. CI deploy steps: Migration job delete+apply+wait in QA and prod deploy
9. deploy.sh: `kubectl wait` for payment-service deployment
10. Ingress: `/go-payments/*` path
11. Frontend: `NEXT_PUBLIC_GO_PAYMENT_URL` env var to Vercel
12. Smoke tests: Update if needed
13. Makefile: Add to `preflight-go`
14. Stripe secrets: K8s Secret for `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET`
