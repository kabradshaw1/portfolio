# Fix: saga_step VARCHAR(20) Overflow Breaking Checkout

- **Date:** 2026-04-22
- **Status:** Accepted

## Context

Checkout on QA was failing with "Order failed. Please try again." The saga orchestrator was silently compensating every order. Investigation revealed two errors in Loki:

- `ERROR: value too long for type character varying(20) (SQLSTATE 22001)`
- `dependency temporarily unavailable: order-postgres` (circuit breaker tripping from repeated failures)

Migration `006_add_saga_step.up.sql` originally defined `saga_step` as `TEXT`. Migration `008_partition_orders.up.sql` recreated the orders table for partitioning but inadvertently narrowed `saga_step` to `VARCHAR(20)`. The saga step `COMPENSATION_COMPLETE` is 21 characters, overflowing the column. Every saga that reached compensation hit this error, tripped the circuit breaker, and cascaded into further failures.

The bug was latent until checkout with payment was enabled — prior to the payment-service integration, sagas rarely reached the compensation path.

## Decision

1. **New migration `011_fix_saga_step_type.up.sql`:** `ALTER TABLE orders ALTER COLUMN saga_step TYPE TEXT;` — restores the original column type. Applied directly to both QA and prod databases for immediate relief, with the migration file ensuring future environments are correct.

2. **Fixed the source migration `008`:** Changed `VARCHAR(20)` to `TEXT` in the partition migration so that replaying migrations from scratch produces the correct schema.

3. **Did not widen to a larger VARCHAR** (e.g., VARCHAR(50)). `TEXT` matches the original intent, avoids future length guessing, and has no performance difference in PostgreSQL (TEXT and VARCHAR are stored identically).

## Consequences

- **Positive:** Checkout works. The `COMPENSATION_COMPLETE` step (and any future longer step names) can be written without constraint errors.
- **Positive:** Circuit breaker stops tripping on saga step writes, eliminating cascading failures.
- **Trade-off:** Editing a previously-applied migration (008) has no effect on existing databases but prevents schema drift if someone stands up a fresh environment. The new migration 011 handles existing deployments.
- **Lesson:** Partition migrations that recreate tables must preserve the exact column types from prior migrations. A CI check that validates column types against a schema snapshot would catch this class of bug.
