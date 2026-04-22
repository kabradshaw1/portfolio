# ADR: Go Database Optimization

## Status
Accepted

## Context
The Go ecommerce services (product, order, cart) had functional but unoptimized database access. No real-database benchmarks existed — existing benchmarks used mocked repositories. An audit identified several anti-patterns:

- **N+1 INSERT pattern** in order creation (one INSERT per order item in a loop)
- **COUNT + data double-query** in product listing (separate COUNT(*) before the data query)
- **Two-query conflict resolution** in cart updates (UPDATE, then fallback SELECT EXISTS)
- **Missing indexes** on frequently filtered columns (saga_step, returns status, cart reserved)
- **String-based error detection** (`strings.Contains(err.Error(), "no rows")`) instead of typed pgx checks
- **No data integrity constraints** at the database level (no CHECK on price, stock, quantity)

## Decision
Apply a three-phase optimization approach: baseline benchmarks → schema hardening → query optimization, with measurements at each phase.

### Phase 1: Benchmark Infrastructure
- **testcontainers-go** for real PostgreSQL 16 in Go test files — any machine with Docker can run `go test -bench`
- Benchmarks measure `ns/op`, `B/op`, `allocs/op` via standard `testing.B`
- `b.RunParallel()` for contention-sensitive operations (stock decrement)
- **EXPLAIN ANALYZE** artifacts captured as JSON to `go/benchdata/` for interview discussion
- Graceful degradation: benchmarks skip when Docker unavailable, non-Docker tests still run

### Phase 2: Schema Hardening
**CHECK constraints:**
- `products.price > 0`, `products.stock >= 0`
- `orders.total > 0`, `order_items.quantity > 0`, `order_items.price_at_purchase > 0`

**New indexes:**
- `idx_orders_saga_step` — FindIncompleteSagas was doing a sequential scan
- `idx_returns_status` — natural access pattern for querying returns by status
- `idx_cart_items_user_reserved` — composite index for Reserve/Release queries
- `idx_products_low_stock` — partial index (`WHERE stock < 10`) for inventory alerting

**Error handling:**
- Replaced `strings.Contains(err.Error(), "no rows")` with `errors.Is(err, pgx.ErrNoRows)`
- Replaced `strings.Contains(err.Error(), "duplicate key")` with `errors.As(err, &pgconn.PgError)` + code `23505`

### Phase 3: Query Optimizations

**Batch INSERT for order items** (order-service):
Replaced N+1 loop with single multi-row INSERT using dynamically built `VALUES ($1..$5), ($6..$10), ...`. Eliminates N round trips for N-item orders.

**COUNT(*) OVER() window function** (product-service):
Replaced separate `SELECT COUNT(*)` + `SELECT ... LIMIT/OFFSET` with single query using `COUNT(*) OVER() AS total_count`. Trades slight per-row overhead for eliminating a full table scan.

**CTE-based cart conflict resolution** (cart-service):
Replaced two-query pattern (UPDATE, then fallback SELECT EXISTS) with single CTE:
```sql
WITH updated AS (UPDATE ... RETURNING id)
SELECT EXISTS(SELECT 1 FROM updated), EXISTS(SELECT 1 FROM cart_items WHERE ... AND reserved = true)
```

**pgx prepared statement cache** (all services):
Enabled `pgx.QueryExecModeCacheDescribe` on all connection pools. Caches query plans after first execution — zero code changes to query callsites.

**Stock decrement locking documentation** (product-service):
Documented the trade-offs between `SELECT ... FOR UPDATE` (current), `pg_advisory_xact_lock`, and optimistic locking with version columns. Current approach is correct for this scale.

## Results
Benchmark results will be captured to `go/benchdata/` when running with Docker:
- `baseline-results.txt` — before any changes
- `post-schema-results.txt` — after schema hardening only
- `optimized-results.txt` — after all query optimizations

Expected improvements based on the nature of the changes:
- Batch INSERT: 3-5x for multi-item orders (N round trips → 1)
- COUNT elimination: ~2x for offset pagination (2 queries → 1)
- CTE cart conflict: ~2x on conflict path (2 queries → 1)
- Prepared statement cache: 10-20% on hot-path queries

## Consequences
- **Docker required for benchmarks** — testcontainers spins up PostgreSQL containers. CI must have Docker. Benchmarks gracefully skip without Docker.
- **Prepared statement cache** changes the default query execution mode for all connections. This is the recommended pgx mode for production workloads.
- **Window function** adds per-row overhead on offset pagination — acceptable trade-off vs. the double query, and cursor pagination (already implemented) avoids this entirely.
- **CHECK constraints** are defense-in-depth — application code already validates, but the DB now enforces as a safety net.
