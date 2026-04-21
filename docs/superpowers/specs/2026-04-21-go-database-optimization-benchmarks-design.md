# Go Database Optimization & Benchmarks

**Date:** 2026-04-21
**Status:** Draft
**Services:** product-service, order-service, cart-service

## Goal

Demonstrate database engineering maturity across the Go ecommerce services through a three-phase approach: baseline benchmarks, schema hardening, and query optimization. Each phase produces measurable results, telling a systematic "before vs. after" story for the portfolio.

## Motivation

Job applications targeting roles that require highly optimized Go + PostgreSQL systems. This work signals:

- Schema design with proper constraints and indexing strategy
- Query optimization driven by EXPLAIN ANALYZE, not guesswork
- Benchmark discipline — real PostgreSQL (testcontainers), not mocks
- Systematic methodology: measure, harden, optimize, re-measure

## Phase 1: Baseline Benchmarks

### Testcontainers Infrastructure

Shared package at `go/pkg/dbtest/` providing:

- `SetupPostgres(t testing.TB, migrationsDir string) *pgxpool.Pool` — spins up a PostgreSQL container via testcontainers-go, runs migrations from the given directory, returns a connected pool
- Container is created once per test binary via `TestMain`, reused across all benchmarks in the package
- Seed data loaded once before benchmarks run (realistic volumes: 1K-10K rows depending on table)
- Automatic cleanup on test completion

### Benchmark Files

Each service gets a `*_bench_test.go` in its `internal/repository/` package:

**product-service** (`product_bench_test.go`):
- `BenchmarkProductList/cursor/1000rows` — cursor-based pagination
- `BenchmarkProductList/offset/1000rows` — offset-based pagination (includes COUNT query)
- `BenchmarkProductList/with_category_filter` — filtered by category
- `BenchmarkProductList/with_search_query` — ILIKE text search
- `BenchmarkProductFindByID`
- `BenchmarkProductDecrementStock` — pessimistic locking under `b.RunParallel()`
- `BenchmarkProductCategories` — DISTINCT category listing

**order-service** (`order_bench_test.go`):
- `BenchmarkOrderCreate/1_item` — single item order
- `BenchmarkOrderCreate/5_items` — multi-item order (exposes N+1)
- `BenchmarkOrderCreate/20_items` — large order (N+1 amplified)
- `BenchmarkOrderFindByID` — with JOIN to order_items + products
- `BenchmarkOrderListByUser/cursor`
- `BenchmarkOrderListByUser/simple`
- `BenchmarkOrderFindIncompleteSagas` — no index on saga_step

**cart-service** (`cart_bench_test.go`):
- `BenchmarkCartGetByUser/5_items` — small cart
- `BenchmarkCartGetByUser/50_items` — large cart
- `BenchmarkCartAddItem/new` — fresh insert
- `BenchmarkCartAddItem/existing` — upsert (quantity increment)
- `BenchmarkCartReserve` — reserve all items for user
- `BenchmarkCartUpdateQuantity/success` — normal update
- `BenchmarkCartUpdateQuantity/reserved_conflict` — triggers fallback query

### Measurements

- `ns/op` — query latency (built into `go test -bench`)
- `B/op`, `allocs/op` — memory pressure via `b.ReportAllocs()`
- `b.RunParallel()` — concurrent throughput for contention-sensitive operations (stock decrement, cart reservation)

### EXPLAIN ANALYZE Capture

Helper function `CaptureExplain(ctx, pool, query, args...) json.RawMessage` that runs `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` and writes output to `benchdata/<service>/<query_name>.json`. These artifacts are committed to the repo as interview discussion material.

## Phase 2: Schema Hardening

New migrations added to each service. Benchmarks re-run after to measure impact.

### Product Service

Migration `003_add_constraints_and_indexes.up.sql`:

```sql
ALTER TABLE products ADD CONSTRAINT chk_products_price CHECK (price > 0);
ALTER TABLE products ADD CONSTRAINT chk_products_stock CHECK (stock >= 0);
CREATE INDEX idx_products_low_stock ON products (stock) WHERE stock < 10;
```

- `CHECK (price > 0)` — prevents zero/negative prices at DB level
- `CHECK (stock >= 0)` — prevents negative stock (complements application-level check in DecrementStock)
- Partial index on low stock — demonstrates partial index knowledge, useful for inventory alerting queries

### Order Service

Migration `007_add_constraints_and_indexes.up.sql`:

```sql
ALTER TABLE orders ADD CONSTRAINT chk_orders_total CHECK (total > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_quantity CHECK (quantity > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price CHECK (price_at_purchase > 0);
CREATE INDEX idx_orders_saga_step ON orders (saga_step);
CREATE INDEX idx_returns_status ON returns (status);
```

- CHECK constraints on total, quantity, and price — defense in depth
- `idx_orders_saga_step` — `FindIncompleteSagas` currently does a sequential scan
- `idx_returns_status` — natural access pattern for querying returns by status

### Cart Service

Migration `003_add_constraints_and_indexes.up.sql`:

```sql
ALTER TABLE cart_items ADD CONSTRAINT chk_cart_items_quantity CHECK (quantity > 0);
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
```

- `CHECK (quantity > 0)` — missing from cart-service's migration (exists in order-service's copy)
- Composite index on `(user_id, reserved)` — `Reserve` and `Release` queries filter on both columns

### Error Handling Consistency

Not a migration, but fixed alongside schema work:

- Replace `strings.Contains(err.Error(), "no rows")` with `errors.Is(err, pgx.ErrNoRows)` in product-service repository
- Replace `strings.Contains(err.Error(), "duplicate key")` with pgx error code checking (`pgconn.PgError` code `23505`) in auth-service repository (drive-by fix — auth is out of scope for benchmarks but this is a one-line consistency improvement)

## Phase 3: Query Optimizations

### Batch Insert for Order Items (order-service)

**Current:** N+1 loop — one `INSERT` per order item inside a transaction.

**Optimized:** Single multi-row INSERT with dynamically built values clause:

```go
query := "INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase) VALUES "
// Build ($1,$2,$3,$4,$5), ($6,$7,$8,$9,$10), ... dynamically
```

Alternative: `pgx.CopyFrom` (COPY protocol) for maximum throughput. Decision made based on benchmark results — CopyFrom is faster for large batches but adds complexity.

**Expected improvement:** 3-5x for 5+ item orders.

### Eliminate COUNT+Data Double Query (product-service)

**Current:** Offset-based `List` runs `SELECT COUNT(*)` then `SELECT ... LIMIT/OFFSET` — two round trips.

**Optimized:** Window function approach:

```sql
SELECT *, COUNT(*) OVER() AS total_count
FROM products
WHERE ...
ORDER BY ...
LIMIT $1 OFFSET $2
```

One query instead of two. Document tradeoff: window function adds overhead per row, but eliminates a full table scan for the count.

**Expected improvement:** ~2x for offset-based listing.

### Prepared Statement Cache (all services)

**Current:** All queries go through parse/plan on every execution.

**Optimized:** Enable pgx's automatic prepared statement cache:

```go
config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
```

This caches the query plan after first execution. Zero code changes to query callsites.

**Expected improvement:** 10-20% on hot-path queries (measurable in repeated benchmark iterations).

### Single-Query Cart Conflict Resolution (cart-service)

**Current:** `UpdateQuantity` does UPDATE, then fallback SELECT EXISTS on conflict.

**Optimized:** CTE-based single query:

```sql
WITH updated AS (
    UPDATE cart_items SET quantity = $3
    WHERE id = $1 AND user_id = $2 AND reserved = false
    RETURNING id
)
SELECT
    EXISTS(SELECT 1 FROM updated) AS was_updated,
    EXISTS(SELECT 1 FROM cart_items WHERE id = $1 AND user_id = $2 AND reserved = true) AS is_reserved
```

One round trip regardless of conflict state.

**Expected improvement:** 2x on conflict path, neutral on happy path.

### Stock Decrement — Documentation Only

The current `SELECT ... FOR UPDATE` (pessimistic locking) approach in product-service is correct for this scale. No code change — instead, add comments documenting the tradeoff vs. `pg_advisory_xact_lock` and optimistic locking with version columns. This becomes an interview talking point about lock contention strategies at different scales.

## Deliverables

| Artifact | Path |
|----------|------|
| Testcontainers helper | `go/pkg/dbtest/` |
| Product benchmarks | `go/product-service/internal/repository/product_bench_test.go` |
| Order benchmarks | `go/order-service/internal/repository/order_bench_test.go` |
| Cart benchmarks | `go/cart-service/internal/repository/cart_bench_test.go` |
| EXPLAIN ANALYZE artifacts | `go/<service>/benchdata/*.json` |
| Product schema migration | `go/product-service/migrations/003_add_constraints_and_indexes.{up,down}.sql` |
| Order schema migration | `go/order-service/migrations/007_add_constraints_and_indexes.{up,down}.sql` |
| Cart schema migration | `go/cart-service/migrations/003_add_constraints_and_indexes.{up,down}.sql` |
| Optimized query implementations | Modified repository files in each service |
| ADR | `docs/adr/go-database-optimization.md` |

## Success Criteria

- All benchmarks run with `go test -bench ./... -benchmem` from any machine with Docker installed
- Measurable ns/op improvement: batch insert 3-5x, COUNT elimination ~2x, prepared statements 10-20%
- Zero regressions — existing unit and integration tests pass
- All CHECK constraints valid against existing seed data
- EXPLAIN ANALYZE artifacts show index usage (no unexpected sequential scans)
- Error handling uses typed checks (`errors.Is`, pgconn error codes) consistently

## Out of Scope

- Load testing (k6/hey) — future phase
- Auth service — too simple to showcase optimization work
- Redis cache optimization — already well-implemented with circuit breakers
- Cross-service query patterns — each service optimized independently
- Connection pool tuning under load — belongs in load test phase
- Foreign key changes across service boundaries — intentionally denormalized for decomposition
