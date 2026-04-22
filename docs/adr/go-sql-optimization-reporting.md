# ADR: SQL Optimization — Partitioning, Materialized Views, and Reporting

- **Date:** 2026-04-22
- **Status:** Accepted
- **Complements:** [Go Database Optimization](go-database-optimization.md) (Phase 1: benchmarks + schema hardening)

## Context

The order-service had optimized individual queries (see prior ADR) but lacked techniques needed for production-scale analytics workloads:

- **Table growth** — the `orders` table grows monotonically with time. Sequential scans become expensive as the table scales, and most queries filter on recent date ranges that could benefit from partition pruning.
- **Reporting queries** — the ecommerce frontend and AI agent need pre-computed business metrics (revenue trends, product performance, customer lifetime value). Computing these live on every request is wasteful when the underlying data changes infrequently.
- **Portfolio demonstration** — a Gen AI Engineer interview should show fluency with professional SQL patterns: range partitioning, materialized views with concurrent refresh, CTEs, and window functions.

### Constraints

- All changes target `go/order-service/` — no new service, no new database.
- Existing application code uses lowercase status strings (`"completed"`, not `"COMPLETED"`), which the queries must match.
- PostgreSQL partitioned tables have a fundamental constraint: unique indexes must include the partition key. This affects foreign key relationships.
- The ecommerce service uses the `resilience.Call[T]` generic wrapper for all database access, which requires `func(ctx context.Context) (T, error)` — queries must be adapted to this signature.

## Decision

### 1. Range Partitioning on `orders.created_at`

Partition the `orders` table by month using PostgreSQL range partitioning:

```sql
CREATE TABLE orders (
    id         UUID NOT NULL,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    saga_step  VARCHAR(20),
    total      INTEGER NOT NULL CHECK (total > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)  -- partition key must be in PK
) PARTITION BY RANGE (created_at);
```

**Why monthly?** Monthly granularity balances partition count against pruning effectiveness. Daily partitions would create 365+ tables per year (management overhead); quarterly would give less granular pruning for date-range queries. Monthly gives 12 partitions/year — manageable, and most reporting queries use 30/90-day windows that prune to 1-3 partitions.

**Why `created_at` as the partition key?** Almost every reporting query filters on order date. User-based partitioning (by `user_id`) wouldn't help because reporting aggregates across all users. Status-based partitioning has too few values and would create hotspots on `"pending"`.

**The FK trade-off.** The composite PK `(id, created_at)` means there's no unique constraint on `id` alone. PostgreSQL requires FK target columns to have a unique constraint, so the existing FKs from `order_items` and `returns` to `orders(id)` cannot be re-created on the partitioned table. Referential integrity is enforced at the application layer — the order-service creates orders and their items in a single transaction, and the saga orchestrator manages lifecycle transitions. This is a well-known trade-off with PostgreSQL partitioning, documented in the migration file.

**Automatic partition maintenance.** A background goroutine runs daily and creates partitions 3 months ahead using `CREATE TABLE IF NOT EXISTS`:

```go
func RunMaintenance(ctx context.Context, pool *pgxpool.Pool) {
    if err := EnsureFuturePartitions(ctx, pool); err != nil {
        slog.ErrorContext(ctx, "initial partition maintenance failed", "error", err)
    }
    go func() {
        ticker := time.NewTicker(24 * time.Hour)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                if err := EnsureFuturePartitions(ctx, pool); err != nil {
                    slog.ErrorContext(ctx, "partition maintenance failed", "error", err)
                }
            }
        }
    }()
}
```

A default partition catches any rows that fall outside defined ranges, preventing INSERT failures.

### 2. Materialized Views for Pre-computed Metrics

Three materialized views serve different reporting needs:

| View | Purpose | Refresh Strategy |
|------|---------|-----------------|
| `mv_daily_revenue` | Revenue by product/day — feeds sales trend reports | Concurrent, 15-min interval |
| `mv_product_performance` | Units, revenue, AOV, return rate per product | Concurrent, 15-min interval |
| `mv_customer_summary` | CLV proxy — order count, total spend, first/last order per user | Concurrent, 15-min interval |

**Why materialized views over live queries?** The reporting queries join `orders`, `order_items`, `products`, and `returns` with GROUP BY and aggregation. On a table with partition pruning these are already fast, but the materialized view approach has two advantages: (1) read latency is constant regardless of table size — it's a simple scan of the pre-aggregated result, and (2) the refresh cost is amortized across all readers.

**Why `REFRESH MATERIALIZED VIEW CONCURRENTLY`?** Standard refresh takes an exclusive lock, blocking reads. `CONCURRENTLY` allows reads during refresh but requires a unique index on the view. Each view has one:

```sql
CREATE UNIQUE INDEX idx_mv_daily_revenue_pk ON mv_daily_revenue (day, product_id);
CREATE UNIQUE INDEX idx_mv_product_performance_pk ON mv_product_performance (product_id);
CREATE UNIQUE INDEX idx_mv_customer_summary_pk ON mv_customer_summary (user_id);
```

**Why 15 minutes?** Reporting data doesn't need to be real-time — a 15-minute staleness window is acceptable for dashboards and analytics. Shorter intervals waste CPU on refreshes that no one observes; longer intervals make the data feel stale when browsing.

### 3. CTE-based Reporting Queries with Window Functions

The reporting repository uses CTEs and window functions rather than subqueries:

```sql
-- Sales trends with rolling averages
WITH daily AS (
    SELECT day, SUM(revenue_cents) AS daily_revenue
    FROM mv_daily_revenue
    WHERE day >= CURRENT_DATE - $1::int
    GROUP BY day ORDER BY day
)
SELECT day, daily_revenue,
    SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS rolling_7day,
    SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 29 PRECEDING AND CURRENT ROW) AS rolling_30day
FROM daily ORDER BY day
```

**Why CTEs over subqueries?** Readability — the CTE separates the "get daily totals" step from the "compute rolling windows" step. PostgreSQL 12+ inlines CTEs when possible, so there's no performance penalty.

**Why `DENSE_RANK()` over `ROW_NUMBER()`?** For inventory turnover and top customers, tied values should share the same rank. `DENSE_RANK()` gives `1, 1, 2` for ties; `ROW_NUMBER()` gives `1, 2, 3` which misrepresents the data.

### 4. Reporting Handler with Interface-based Injection

The handler defines a `ReportingRepo` interface rather than depending on the concrete `*reporting.Repository`:

```go
type ReportingRepo interface {
    SalesTrends(ctx context.Context, days int) ([]reporting.SalesTrend, error)
    InventoryTurnover(ctx context.Context, days, limit int) ([]reporting.InventoryTurnover, error)
    TopCustomers(ctx context.Context, limit int) ([]reporting.CustomerSummary, error)
    ProductPerformance(ctx context.Context) ([]reporting.ProductPerformance, error)
}
```

This allows handler tests to use a mock without a database, keeping unit tests fast and Docker-free.

### 5. Benchmark Suite

Integration benchmarks (build-tagged `//go:build integration`) use testcontainers to:
- Seed 10,000 orders across 50 products and 10 users over 6 months
- Refresh materialized views
- Benchmark each reporting query
- Compare live aggregation vs materialized view reads
- Capture `EXPLAIN ANALYZE` output for partition pruning verification

The benchmarks skip gracefully when Docker is unavailable, so `go test ./...` always passes.

## Migration Lessons Learned

The partitioning migration revealed three issues that were caught in CI before reaching production:

1. **Index name collision.** `ALTER TABLE orders RENAME TO orders_old` keeps the original index names. Creating `CREATE INDEX idx_orders_user ON orders(...)` fails because `idx_orders_user` already exists on `orders_old`. Fix: explicitly `DROP INDEX IF EXISTS` after the rename.

2. **FK on partitioned tables.** The composite PK `(id, created_at)` doesn't satisfy `REFERENCES orders(id)` because there's no unique constraint on `id` alone. Both `order_items` and `returns` FKs must be dropped and cannot be re-created.

3. **Forgotten FK.** The `returns` table also has a FK to `orders` — not just `order_items`. Both must be dropped before the table can be dropped with `DROP TABLE orders_old CASCADE`.

These failures led to adding `make preflight-go-migrations` — a local Makefile target that spins up Postgres in Docker, runs all migrations, and verifies tables exist. This catches migration errors before pushing to CI.

## Consequences

### Positive

- **Partition pruning** reduces scan scope for date-filtered queries from the full table to 1-3 monthly partitions
- **Materialized views** give constant-time reads for reporting regardless of table size
- **Window functions** provide rolling averages and ranked results without application-level computation
- **Local migration testing** via `make preflight-go-migrations` prevents the class of errors that caused 3 CI failures
- **Benchmark suite** provides reproducible performance measurements for interview discussion

### Trade-offs

- **No FK enforcement** on `order_items` and `returns` — referential integrity relies on application logic (saga orchestrator, transactional order creation)
- **15-minute data staleness** on materialized views — acceptable for analytics, not suitable for real-time inventory decisions
- **Partition maintenance overhead** — the daily goroutine is lightweight but adds a background process that must be monitored
- **Migration complexity** — partitioning an existing table requires a rename-copy-drop dance that is error-prone (as the CI failures demonstrated)

## File Map

```
go/order-service/
├── migrations/
│   ├── 008_partition_orders.up.sql          # Range partition + index rebuild
│   ├── 008_partition_orders.down.sql        # Revert to flat table + restore FKs
│   ├── 009_create_materialized_views.up.sql # 3 materialized views + unique indexes
│   └── 009_create_materialized_views.down.sql
├── internal/
│   ├── partition/
│   │   ├── manager.go                       # Name(), CreateSQL(), RunMaintenance()
│   │   └── manager_test.go
│   ├── reporting/
│   │   ├── model.go                         # SalesTrend, ProductPerformance, etc.
│   │   ├── refresher.go                     # MaterializedViews(), Refresher.Run()
│   │   ├── refresher_test.go
│   │   ├── repository.go                    # CTE queries with resilience.Call[T]
│   │   ├── repository_test.go               # Nil-pool error tests
│   │   └── reporting_bench_test.go          # Integration benchmarks (build-tagged)
│   └── handler/
│       ├── reporting.go                     # ReportingRepo interface, 4 endpoints
│       └── reporting_test.go                # Mock-based handler tests
└── cmd/server/
    ├── main.go                              # Wires partition + refresher + handler
    └── routes.go                            # /reporting/* route group
```
