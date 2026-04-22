# SQL Optimization + Reporting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add table partitioning, materialized views, CTE-based reporting queries, and a benchmark suite to the order-service to demonstrate professional SQL optimization skills.

**Architecture:** New migrations partition the `orders` table by `created_at` (range, monthly). Materialized views pre-compute revenue, product performance, and customer summary metrics. A background goroutine refreshes views concurrently. REST endpoints expose reporting queries using CTEs and window functions. A benchmark suite proves optimization impact with before/after metrics.

**Tech Stack:** Go, PostgreSQL (range partitioning, materialized views, CTEs, window functions), pgx, testcontainers

**Spec:** `docs/superpowers/specs/2026-04-22-payment-service-sql-optimization-design.md` (Track 2)

---

### Task 1: Table Partitioning Migration

**Files:**
- Create: `go/order-service/migrations/008_partition_orders.up.sql`
- Create: `go/order-service/migrations/008_partition_orders.down.sql`

- [ ] **Step 1: Create the partitioning up migration**

Create `go/order-service/migrations/008_partition_orders.up.sql`:

```sql
-- Step 1: Rename existing orders table
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;
ALTER TABLE orders RENAME TO orders_old;

-- Step 2: Create partitioned orders table with same schema
CREATE TABLE orders (
    id         UUID NOT NULL,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    saga_step  VARCHAR(20),
    total      INTEGER NOT NULL CHECK (total > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Step 3: Create partitions (monthly, 2026-01 through 2027-06)
CREATE TABLE orders_2026_01 PARTITION OF orders FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE orders_2026_02 PARTITION OF orders FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
CREATE TABLE orders_2026_03 PARTITION OF orders FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
CREATE TABLE orders_2026_04 PARTITION OF orders FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE orders_2026_05 PARTITION OF orders FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE orders_2026_06 PARTITION OF orders FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE orders_2026_07 PARTITION OF orders FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE orders_2026_08 PARTITION OF orders FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE orders_2026_09 PARTITION OF orders FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE orders_2026_10 PARTITION OF orders FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE orders_2026_11 PARTITION OF orders FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE orders_2026_12 PARTITION OF orders FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE orders_2027_01 PARTITION OF orders FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
CREATE TABLE orders_2027_02 PARTITION OF orders FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
CREATE TABLE orders_2027_03 PARTITION OF orders FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');
CREATE TABLE orders_2027_04 PARTITION OF orders FOR VALUES FROM ('2027-04-01') TO ('2027-05-01');
CREATE TABLE orders_2027_05 PARTITION OF orders FOR VALUES FROM ('2027-05-01') TO ('2027-06-01');
CREATE TABLE orders_2027_06 PARTITION OF orders FOR VALUES FROM ('2027-06-01') TO ('2027-07-01');

-- Default partition for anything outside the defined ranges
CREATE TABLE orders_default PARTITION OF orders DEFAULT;

-- Step 4: Copy data from old table
INSERT INTO orders (id, user_id, status, saga_step, total, created_at, updated_at)
SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders_old;

-- Step 5: Re-create indexes on partitioned table
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_saga_step ON orders(saga_step);
CREATE INDEX idx_orders_created_at ON orders(created_at);
-- Composite index for cursor pagination (keyset)
CREATE INDEX idx_orders_user_cursor ON orders(user_id, created_at DESC, id DESC);

-- Step 6: Re-create FK from order_items
ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;

-- Step 7: Drop old table
DROP TABLE orders_old;
```

- [ ] **Step 2: Create the partitioning down migration**

Create `go/order-service/migrations/008_partition_orders.down.sql`:

```sql
-- Reverse: re-create non-partitioned table and copy data back
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS order_items_order_id_fkey;

CREATE TABLE orders_flat (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'pending',
    saga_step  VARCHAR(20),
    total      INTEGER NOT NULL CHECK (total > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO orders_flat (id, user_id, status, saga_step, total, created_at, updated_at)
SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders;

DROP TABLE orders;
ALTER TABLE orders_flat RENAME TO orders;

CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_saga_step ON orders(saga_step);

ALTER TABLE order_items ADD CONSTRAINT order_items_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE;
```

- [ ] **Step 3: Commit**

```bash
git add go/order-service/migrations/008_partition_orders.up.sql go/order-service/migrations/008_partition_orders.down.sql
git commit -m "feat(order): add migration to range-partition orders table by created_at"
```

---

### Task 2: Partition Maintenance Goroutine

**Files:**
- Create: `go/order-service/internal/partition/manager.go`
- Create: `go/order-service/internal/partition/manager_test.go`

- [ ] **Step 1: Write failing test**

Create `go/order-service/internal/partition/manager_test.go`:

```go
package partition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/partition"
)

func TestPartitionName(t *testing.T) {
	tests := []struct {
		date     time.Time
		expected string
	}{
		{time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), "orders_2026_04"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "orders_2027_01"},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "orders_2026_12"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			name := partition.Name(tt.date)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestCreatePartitionSQL(t *testing.T) {
	sql := partition.CreateSQL(time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC))
	assert.Contains(t, sql, "orders_2027_07")
	assert.Contains(t, sql, "2027-07-01")
	assert.Contains(t, sql, "2027-08-01")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/order-service && go test ./internal/partition/... -v
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement partition manager**

Create `go/order-service/internal/partition/manager.go`:

```go
package partition

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const monthsAhead = 3 // Create partitions 3 months in advance

// Name returns the partition table name for a given date.
func Name(t time.Time) string {
	return fmt.Sprintf("orders_%04d_%02d", t.Year(), t.Month())
}

// CreateSQL returns the DDL to create a monthly partition for the given date.
func CreateSQL(t time.Time) string {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	name := Name(start)

	return fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF orders FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
}

// EnsureFuturePartitions creates partitions for the next N months from now.
func EnsureFuturePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	now := time.Now().UTC()
	for i := 0; i <= monthsAhead; i++ {
		target := now.AddDate(0, i, 0)
		sql := CreateSQL(target)
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("create partition %s: %w", Name(target), err)
		}
		slog.InfoContext(ctx, "ensured partition exists", "partition", Name(target))
	}
	return nil
}

// RunMaintenance starts a background goroutine that creates future partitions daily.
func RunMaintenance(ctx context.Context, pool *pgxpool.Pool) {
	// Create partitions on startup
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

- [ ] **Step 4: Run tests**

```bash
cd go/order-service && go test ./internal/partition/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/order-service/internal/partition/
git commit -m "feat(order): add partition maintenance goroutine for monthly orders partitions"
```

---

### Task 3: Materialized Views Migration

**Files:**
- Create: `go/order-service/migrations/009_create_materialized_views.up.sql`
- Create: `go/order-service/migrations/009_create_materialized_views.down.sql`

- [ ] **Step 1: Create materialized views migration**

Create `go/order-service/migrations/009_create_materialized_views.up.sql`:

```sql
-- Daily revenue by product and category
CREATE MATERIALIZED VIEW mv_daily_revenue AS
SELECT
    date_trunc('day', o.created_at)::date AS day,
    oi.product_id,
    p.name AS product_name,
    p.category,
    SUM(oi.quantity * oi.price_at_purchase) AS revenue_cents,
    SUM(oi.quantity) AS units_sold,
    COUNT(DISTINCT o.id) AS order_count
FROM orders o
JOIN order_items oi ON oi.order_id = o.id
JOIN products p ON p.id = oi.product_id
WHERE o.status = 'completed'
GROUP BY 1, 2, 3, 4;

-- Unique index required for REFRESH MATERIALIZED VIEW CONCURRENTLY
CREATE UNIQUE INDEX idx_mv_daily_revenue_pk ON mv_daily_revenue (day, product_id);

-- Product performance: units, revenue, return rate, AOV
CREATE MATERIALIZED VIEW mv_product_performance AS
SELECT
    p.id AS product_id,
    p.name AS product_name,
    p.category,
    p.stock AS current_stock,
    COALESCE(SUM(oi.quantity), 0) AS total_units_sold,
    COALESCE(SUM(oi.quantity * oi.price_at_purchase), 0) AS total_revenue_cents,
    COUNT(DISTINCT o.id) AS total_orders,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN COALESCE(SUM(oi.quantity * oi.price_at_purchase), 0) / COUNT(DISTINCT o.id)
        ELSE 0
    END AS avg_order_value_cents,
    COALESCE(r.return_count, 0) AS return_count,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN ROUND(COALESCE(r.return_count, 0)::numeric / COUNT(DISTINCT o.id) * 100, 2)
        ELSE 0
    END AS return_rate_pct
FROM products p
LEFT JOIN order_items oi ON oi.product_id = p.id
LEFT JOIN orders o ON o.id = oi.order_id AND o.status = 'completed'
LEFT JOIN (
    SELECT oi2.product_id, COUNT(*) AS return_count
    FROM returns ret
    JOIN order_items oi2 ON oi2.order_id = ret.order_id
    WHERE ret.status = 'approved'
    GROUP BY oi2.product_id
) r ON r.product_id = p.id
GROUP BY p.id, p.name, p.category, p.stock, r.return_count;

CREATE UNIQUE INDEX idx_mv_product_performance_pk ON mv_product_performance (product_id);

-- Customer summary: CLV proxy
CREATE MATERIALIZED VIEW mv_customer_summary AS
SELECT
    o.user_id,
    COUNT(DISTINCT o.id) AS order_count,
    COALESCE(SUM(o.total), 0) AS total_spend_cents,
    MIN(o.created_at) AS first_order_at,
    MAX(o.created_at) AS last_order_at,
    CASE
        WHEN COUNT(DISTINCT o.id) > 0
        THEN COALESCE(SUM(o.total), 0) / COUNT(DISTINCT o.id)
        ELSE 0
    END AS avg_order_value_cents
FROM orders o
WHERE o.status = 'completed'
GROUP BY o.user_id;

CREATE UNIQUE INDEX idx_mv_customer_summary_pk ON mv_customer_summary (user_id);
```

- [ ] **Step 2: Create down migration**

Create `go/order-service/migrations/009_create_materialized_views.down.sql`:

```sql
DROP MATERIALIZED VIEW IF EXISTS mv_customer_summary;
DROP MATERIALIZED VIEW IF EXISTS mv_product_performance;
DROP MATERIALIZED VIEW IF EXISTS mv_daily_revenue;
```

- [ ] **Step 3: Commit**

```bash
git add go/order-service/migrations/009_create_materialized_views.up.sql go/order-service/migrations/009_create_materialized_views.down.sql
git commit -m "feat(order): add materialized views for revenue, product performance, and customer summary"
```

---

### Task 4: Materialized View Refresher

**Files:**
- Create: `go/order-service/internal/reporting/refresher.go`
- Create: `go/order-service/internal/reporting/refresher_test.go`

- [ ] **Step 1: Write failing test**

Create `go/order-service/internal/reporting/refresher_test.go`:

```go
package reporting_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
)

func TestNewRefresher(t *testing.T) {
	r := reporting.NewRefresher(nil, 15*time.Minute)
	assert.NotNil(t, r)
}

func TestMaterializedViewNames(t *testing.T) {
	views := reporting.MaterializedViews()
	assert.Contains(t, views, "mv_daily_revenue")
	assert.Contains(t, views, "mv_product_performance")
	assert.Contains(t, views, "mv_customer_summary")
	assert.Len(t, views, 3)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/order-service && go test ./internal/reporting/... -v
```

Expected: FAIL

- [ ] **Step 3: Implement refresher**

Create `go/order-service/internal/reporting/refresher.go`:

```go
package reporting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var views = []string{
	"mv_daily_revenue",
	"mv_product_performance",
	"mv_customer_summary",
}

// MaterializedViews returns the list of managed materialized view names.
func MaterializedViews() []string {
	return views
}

// Refresher periodically refreshes materialized views concurrently.
type Refresher struct {
	pool     *pgxpool.Pool
	interval time.Duration
}

func NewRefresher(pool *pgxpool.Pool, interval time.Duration) *Refresher {
	return &Refresher{pool: pool, interval: interval}
}

// Run starts the refresh loop. Blocks until context is cancelled.
func (r *Refresher) Run(ctx context.Context) {
	// Initial refresh on startup
	r.refreshAll(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshAll(ctx)
		}
	}
}

func (r *Refresher) refreshAll(ctx context.Context) {
	for _, view := range views {
		start := time.Now()
		sql := fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", view)
		if _, err := r.pool.Exec(ctx, sql); err != nil {
			slog.ErrorContext(ctx, "refresh materialized view failed", "view", view, "error", err)
			continue
		}
		slog.InfoContext(ctx, "refreshed materialized view", "view", view, "duration", time.Since(start).String())
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/order-service && go test ./internal/reporting/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/order-service/internal/reporting/
git commit -m "feat(order): add materialized view refresher with concurrent refresh support"
```

---

### Task 5: Reporting Repository — CTE Queries

**Files:**
- Create: `go/order-service/internal/reporting/model.go`
- Create: `go/order-service/internal/reporting/repository.go`
- Create: `go/order-service/internal/reporting/repository_test.go`

- [ ] **Step 1: Create reporting models**

Create `go/order-service/internal/reporting/model.go`:

```go
package reporting

import "time"

type DailyRevenue struct {
	Day         time.Time `json:"day"`
	ProductID   string    `json:"productId"`
	ProductName string    `json:"productName"`
	Category    string    `json:"category"`
	RevenueCents int      `json:"revenueCents"`
	UnitsSold   int       `json:"unitsSold"`
	OrderCount  int       `json:"orderCount"`
}

type SalesTrend struct {
	Day            time.Time `json:"day"`
	DailyRevenue   int       `json:"dailyRevenue"`
	Rolling7Day    int       `json:"rolling7Day"`
	Rolling30Day   int       `json:"rolling30Day"`
}

type ProductPerformance struct {
	ProductID          string  `json:"productId"`
	ProductName        string  `json:"productName"`
	Category           string  `json:"category"`
	CurrentStock       int     `json:"currentStock"`
	TotalUnitsSold     int     `json:"totalUnitsSold"`
	TotalRevenueCents  int     `json:"totalRevenueCents"`
	TotalOrders        int     `json:"totalOrders"`
	AvgOrderValueCents int     `json:"avgOrderValueCents"`
	ReturnCount        int     `json:"returnCount"`
	ReturnRatePct      float64 `json:"returnRatePct"`
}

type InventoryTurnover struct {
	ProductID    string  `json:"productId"`
	ProductName  string  `json:"productName"`
	UnitsSold    int     `json:"unitsSold"`
	CurrentStock int     `json:"currentStock"`
	TurnoverRate float64 `json:"turnoverRate"`
	Rank         int     `json:"rank"`
}

type CustomerSummary struct {
	UserID             string    `json:"userId"`
	OrderCount         int       `json:"orderCount"`
	TotalSpendCents    int       `json:"totalSpendCents"`
	FirstOrderAt       time.Time `json:"firstOrderAt"`
	LastOrderAt        time.Time `json:"lastOrderAt"`
	AvgOrderValueCents int       `json:"avgOrderValueCents"`
	Rank               int       `json:"rank"`
}
```

- [ ] **Step 2: Write failing tests**

Create `go/order-service/internal/reporting/repository_test.go`:

```go
package reporting_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestReportingRepository_SalesTrends_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.SalesTrends(context.Background(), 30)
	assert.Error(t, err)
}

func TestReportingRepository_InventoryTurnover_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.InventoryTurnover(context.Background(), 30, 20)
	assert.Error(t, err)
}

func TestReportingRepository_TopCustomers_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.TopCustomers(context.Background(), 10)
	assert.Error(t, err)
}

func TestReportingRepository_ProductPerformance_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.ProductPerformance(context.Background())
	assert.Error(t, err)
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd go/order-service && go test ./internal/reporting/... -v -run TestReportingRepository
```

Expected: FAIL — `Repository` not defined.

- [ ] **Step 4: Implement reporting repository**

Create `go/order-service/internal/reporting/repository.go`:

```go
package reporting

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

type Repository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func NewRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *Repository {
	return &Repository{pool: pool, breaker: breaker}
}

// SalesTrends returns daily revenue with rolling 7-day and 30-day windows.
func (r *Repository) SalesTrends(ctx context.Context, days int) ([]SalesTrend, error) {
	query := `
		WITH daily AS (
			SELECT day, SUM(revenue_cents) AS daily_revenue
			FROM mv_daily_revenue
			WHERE day >= CURRENT_DATE - $1::int
			GROUP BY day
			ORDER BY day
		)
		SELECT
			day,
			daily_revenue,
			SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW) AS rolling_7day,
			SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 29 PRECEDING AND CURRENT ROW) AS rolling_30day
		FROM daily
		ORDER BY day`

	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		rows, queryErr := r.pool.Query(ctx, query, days)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()

		var trends []SalesTrend
		for rows.Next() {
			var t SalesTrend
			if scanErr := rows.Scan(&t.Day, &t.DailyRevenue, &t.Rolling7Day, &t.Rolling30Day); scanErr != nil {
				return nil, scanErr
			}
			trends = append(trends, t)
		}
		return trends, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SalesTrend), nil
}

// InventoryTurnover returns products ranked by turnover rate over a time window.
func (r *Repository) InventoryTurnover(ctx context.Context, days, limit int) ([]InventoryTurnover, error) {
	query := `
		WITH sales AS (
			SELECT
				oi.product_id,
				p.name AS product_name,
				SUM(oi.quantity) AS units_sold,
				p.stock AS current_stock
			FROM order_items oi
			JOIN orders o ON o.id = oi.order_id
			JOIN products p ON p.id = oi.product_id
			WHERE o.status = 'completed'
			  AND o.created_at >= CURRENT_DATE - $1::int
			GROUP BY oi.product_id, p.name, p.stock
		)
		SELECT
			product_id,
			product_name,
			units_sold,
			current_stock,
			CASE WHEN current_stock > 0
				THEN ROUND(units_sold::numeric / current_stock, 2)
				ELSE 0
			END AS turnover_rate,
			DENSE_RANK() OVER (ORDER BY
				CASE WHEN current_stock > 0
					THEN units_sold::numeric / current_stock
					ELSE 0
				END DESC
			) AS rank
		FROM sales
		ORDER BY turnover_rate DESC
		LIMIT $2`

	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		rows, queryErr := r.pool.Query(ctx, query, days, limit)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()

		var items []InventoryTurnover
		for rows.Next() {
			var it InventoryTurnover
			if scanErr := rows.Scan(&it.ProductID, &it.ProductName, &it.UnitsSold, &it.CurrentStock, &it.TurnoverRate, &it.Rank); scanErr != nil {
				return nil, scanErr
			}
			items = append(items, it)
		}
		return items, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]InventoryTurnover), nil
}

// TopCustomers returns customers ranked by total spend.
func (r *Repository) TopCustomers(ctx context.Context, limit int) ([]CustomerSummary, error) {
	query := `
		SELECT
			user_id::text,
			order_count,
			total_spend_cents,
			first_order_at,
			last_order_at,
			avg_order_value_cents,
			DENSE_RANK() OVER (ORDER BY total_spend_cents DESC) AS rank
		FROM mv_customer_summary
		ORDER BY total_spend_cents DESC
		LIMIT $1`

	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		rows, queryErr := r.pool.Query(ctx, query, limit)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()

		var customers []CustomerSummary
		for rows.Next() {
			var c CustomerSummary
			if scanErr := rows.Scan(&c.UserID, &c.OrderCount, &c.TotalSpendCents, &c.FirstOrderAt, &c.LastOrderAt, &c.AvgOrderValueCents, &c.Rank); scanErr != nil {
				return nil, scanErr
			}
			customers = append(customers, c)
		}
		return customers, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]CustomerSummary), nil
}

// ProductPerformance returns all products with aggregated metrics from the materialized view.
func (r *Repository) ProductPerformance(ctx context.Context) ([]ProductPerformance, error) {
	query := `
		SELECT
			product_id::text,
			product_name,
			category,
			current_stock,
			total_units_sold,
			total_revenue_cents,
			total_orders,
			avg_order_value_cents,
			return_count,
			return_rate_pct
		FROM mv_product_performance
		ORDER BY total_revenue_cents DESC`

	result, err := resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func() (any, error) {
		rows, queryErr := r.pool.Query(ctx, query)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()

		var products []ProductPerformance
		for rows.Next() {
			var p ProductPerformance
			if scanErr := rows.Scan(&p.ProductID, &p.ProductName, &p.Category, &p.CurrentStock, &p.TotalUnitsSold, &p.TotalRevenueCents, &p.TotalOrders, &p.AvgOrderValueCents, &p.ReturnCount, &p.ReturnRatePct); scanErr != nil {
				return nil, scanErr
			}
			products = append(products, p)
		}
		return products, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ProductPerformance), nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd go/order-service && go test ./internal/reporting/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go/order-service/internal/reporting/
git commit -m "feat(order): add reporting repository with CTE queries and window functions"
```

---

### Task 6: Reporting REST Handler

**Files:**
- Create: `go/order-service/internal/handler/reporting.go`
- Create: `go/order-service/internal/handler/reporting_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/order-service/internal/handler/reporting_test.go`:

```go
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type mockReportingRepo struct {
	salesTrends  []reporting.SalesTrend
	inventory    []reporting.InventoryTurnover
	customers    []reporting.CustomerSummary
	products     []reporting.ProductPerformance
	err          error
}

func (m *mockReportingRepo) SalesTrends(_ context.Context, _ int) ([]reporting.SalesTrend, error) {
	return m.salesTrends, m.err
}

func (m *mockReportingRepo) InventoryTurnover(_ context.Context, _, _ int) ([]reporting.InventoryTurnover, error) {
	return m.inventory, m.err
}

func (m *mockReportingRepo) TopCustomers(_ context.Context, _ int) ([]reporting.CustomerSummary, error) {
	return m.customers, m.err
}

func (m *mockReportingRepo) ProductPerformance(_ context.Context) ([]reporting.ProductPerformance, error) {
	return m.products, m.err
}

func setupReportingRouter(h *handler.ReportingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.GET("/reporting/sales-trends", h.SalesTrends)
	r.GET("/reporting/inventory-turnover", h.InventoryTurnover)
	r.GET("/reporting/top-customers", h.TopCustomers)
	r.GET("/reporting/product-performance", h.ProductPerformance)
	return r
}

func TestReportingHandler_SalesTrends(t *testing.T) {
	h := handler.NewReportingHandler(&mockReportingRepo{
		salesTrends: []reporting.SalesTrend{},
	})
	router := setupReportingRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reporting/sales-trends?days=30", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReportingHandler_SalesTrends_DefaultDays(t *testing.T) {
	h := handler.NewReportingHandler(&mockReportingRepo{
		salesTrends: []reporting.SalesTrend{},
	})
	router := setupReportingRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reporting/sales-trends", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/order-service && go test ./internal/handler/... -v -run TestReportingHandler
```

Expected: FAIL

- [ ] **Step 3: Implement reporting handler**

Create `go/order-service/internal/handler/reporting.go`:

```go
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// ReportingRepo abstracts reporting queries.
type ReportingRepo interface {
	SalesTrends(ctx context.Context, days int) ([]reporting.SalesTrend, error)
	InventoryTurnover(ctx context.Context, days, limit int) ([]reporting.InventoryTurnover, error)
	TopCustomers(ctx context.Context, limit int) ([]reporting.CustomerSummary, error)
	ProductPerformance(ctx context.Context) ([]reporting.ProductPerformance, error)
}

type ReportingHandler struct {
	repo ReportingRepo
}

func NewReportingHandler(repo ReportingRepo) *ReportingHandler {
	return &ReportingHandler{repo: repo}
}

func (h *ReportingHandler) SalesTrends(c *gin.Context) {
	days := intQueryParam(c, "days", 30)

	trends, err := h.repo.SalesTrends(c.Request.Context(), days)
	if err != nil {
		c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch sales trends"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"trends": trends})
}

func (h *ReportingHandler) InventoryTurnover(c *gin.Context) {
	days := intQueryParam(c, "days", 30)
	limit := intQueryParam(c, "limit", 20)

	items, err := h.repo.InventoryTurnover(c.Request.Context(), days, limit)
	if err != nil {
		c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch inventory turnover"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"inventory": items})
}

func (h *ReportingHandler) TopCustomers(c *gin.Context) {
	limit := intQueryParam(c, "limit", 10)

	customers, err := h.repo.TopCustomers(c.Request.Context(), limit)
	if err != nil {
		c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch top customers"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"customers": customers})
}

func (h *ReportingHandler) ProductPerformance(c *gin.Context) {
	products, err := h.repo.ProductPerformance(c.Request.Context())
	if err != nil {
		c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch product performance"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": products})
}

func intQueryParam(c *gin.Context, key string, defaultVal int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/order-service && go test ./internal/handler/... -v -run TestReportingHandler
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/order-service/internal/handler/reporting.go go/order-service/internal/handler/reporting_test.go
git commit -m "feat(order): add reporting REST handler for sales trends, inventory, and customers"
```

---

### Task 7: Wire Reporting into Routes + Main

**Files:**
- Modify: `go/order-service/cmd/server/routes.go`
- Modify: `go/order-service/cmd/server/main.go`

- [ ] **Step 1: Add reporting routes**

Add to `go/order-service/cmd/server/routes.go` — add `reportingHandler *handler.ReportingHandler` parameter to `setupRouter`, then add reporting group:

```go
// Reporting routes — no auth, read-only analytics
report := router.Group("/reporting")
{
	report.GET("/sales-trends", reportingHandler.SalesTrends)
	report.GET("/inventory-turnover", reportingHandler.InventoryTurnover)
	report.GET("/top-customers", reportingHandler.TopCustomers)
	report.GET("/product-performance", reportingHandler.ProductPerformance)
}
```

- [ ] **Step 2: Wire up in main.go**

Add to `go/order-service/cmd/server/main.go`:

```go
import (
	"github.com/kabradshaw1/portfolio/go/order-service/internal/partition"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
)
```

After creating the order repository, add:

```go
// Start partition maintenance
partition.RunMaintenance(ctx, pool)

// Start materialized view refresher
refresher := reporting.NewRefresher(pool, 15*time.Minute)
go refresher.Run(ctx)

// Create reporting repository and handler
reportingRepo := reporting.NewRepository(pool, pgBreaker)
reportingHandler := handler.NewReportingHandler(reportingRepo)
```

Pass `reportingHandler` to `setupRouter`.

- [ ] **Step 3: Run all order-service tests**

```bash
cd go/order-service && go test ./... -v -race
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/order-service/cmd/server/routes.go go/order-service/cmd/server/main.go
git commit -m "feat(order): wire reporting handler, partition manager, and view refresher into main"
```

---

### Task 8: Benchmark Suite

**Files:**
- Create: `go/order-service/internal/reporting/reporting_bench_test.go`

- [ ] **Step 1: Create the benchmark test file**

Create `go/order-service/internal/reporting/reporting_bench_test.go`:

```go
//go:build integration

package reporting_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var (
	benchPool *pgxpool.Pool
	benchRepo *reporting.Repository
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	db, err := dbtest.SetupPostgres(ctx, "../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "skip benchmarks: %v\n", err)
		os.Exit(0)
	}

	benchPool = db.Pool
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench"})
	benchRepo = reporting.NewRepository(benchPool, breaker)

	seedBenchData(ctx, benchPool)

	// Refresh materialized views so queries have data
	for _, view := range reporting.MaterializedViews() {
		if _, err := benchPool.Exec(ctx, fmt.Sprintf("REFRESH MATERIALIZED VIEW %s", view)); err != nil {
			fmt.Fprintf(os.Stderr, "refresh %s: %v\n", view, err)
		}
	}

	code := m.Run()
	db.Teardown()
	os.Exit(code)
}

func seedBenchData(ctx context.Context, pool *pgxpool.Pool) {
	rng := rand.New(rand.NewSource(42))

	// Create 50 products
	categories := []string{"electronics", "clothing", "books", "home", "sports"}
	productIDs := make([]uuid.UUID, 50)
	for i := range productIDs {
		id := uuid.New()
		productIDs[i] = id
		cat := categories[rng.Intn(len(categories))]
		price := 500 + rng.Intn(10000)
		pool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, stock, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, now())
			 ON CONFLICT DO NOTHING`,
			id, fmt.Sprintf("Product %d", i), "Benchmark product", price, cat, 100+rng.Intn(500),
		)
	}

	// Create 10 users and 10000 orders spread across 6 months
	userIDs := make([]uuid.UUID, 10)
	for i := range userIDs {
		userIDs[i] = uuid.New()
	}

	baseDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10000; i++ {
		orderID := uuid.New()
		userID := userIDs[rng.Intn(len(userIDs))]
		daysOffset := rng.Intn(180)
		createdAt := baseDate.Add(time.Duration(daysOffset) * 24 * time.Hour)
		numItems := 1 + rng.Intn(5)
		total := 0

		for j := 0; j < numItems; j++ {
			prodID := productIDs[rng.Intn(len(productIDs))]
			qty := 1 + rng.Intn(3)
			price := 500 + rng.Intn(5000)
			total += qty * price
			pool.Exec(ctx,
				`INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT DO NOTHING`,
				uuid.New(), orderID, prodID, qty, price,
			)
		}

		status := "completed"
		if rng.Float32() < 0.1 {
			status = "failed"
		}
		pool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, saga_step, total, created_at, updated_at)
			 VALUES ($1, $2, $3, 'COMPLETED', $4, $5, $5)
			 ON CONFLICT DO NOTHING`,
			orderID, userID, status, total, createdAt,
		)
	}
}

func BenchmarkSalesTrends_30Days(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchRepo.SalesTrends(ctx, 30)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSalesTrends_90Days(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchRepo.SalesTrends(ctx, 90)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInventoryTurnover(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchRepo.InventoryTurnover(ctx, 30, 20)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTopCustomers(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchRepo.TopCustomers(ctx, 10)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductPerformance(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := benchRepo.ProductPerformance(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLiveAggregation compares materialized view read vs live query
func BenchmarkLiveAggregation_DailyRevenue(b *testing.B) {
	ctx := context.Background()
	query := `
		SELECT date_trunc('day', o.created_at)::date AS day,
			SUM(oi.quantity * oi.price_at_purchase) AS revenue
		FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE o.status = 'completed' AND o.created_at >= CURRENT_DATE - 30
		GROUP BY 1 ORDER BY 1`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := benchPool.Query(ctx, query)
		if err != nil {
			b.Fatal(err)
		}
		rows.Close()
	}
}

func BenchmarkMaterializedView_DailyRevenue(b *testing.B) {
	ctx := context.Background()
	query := `
		SELECT day, SUM(revenue_cents) AS revenue
		FROM mv_daily_revenue
		WHERE day >= CURRENT_DATE - 30
		GROUP BY day ORDER BY day`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := benchPool.Query(ctx, query)
		if err != nil {
			b.Fatal(err)
		}
		rows.Close()
	}
}

// TestExplainPlans captures EXPLAIN ANALYZE output for documentation
func TestExplainPlans(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping explain plans in short mode")
	}

	ctx := context.Background()
	queries := map[string]string{
		"sales_trends_30d": `
			EXPLAIN ANALYZE
			WITH daily AS (
				SELECT day, SUM(revenue_cents) AS daily_revenue
				FROM mv_daily_revenue WHERE day >= CURRENT_DATE - 30 GROUP BY day ORDER BY day
			)
			SELECT day, daily_revenue,
				SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 6 PRECEDING AND CURRENT ROW),
				SUM(daily_revenue) OVER (ORDER BY day ROWS BETWEEN 29 PRECEDING AND CURRENT ROW)
			FROM daily ORDER BY day`,
		"inventory_turnover": `
			EXPLAIN ANALYZE
			SELECT oi.product_id, p.name, SUM(oi.quantity), p.stock
			FROM order_items oi
			JOIN orders o ON o.id = oi.order_id
			JOIN products p ON p.id = oi.product_id
			WHERE o.status = 'completed' AND o.created_at >= CURRENT_DATE - 30
			GROUP BY oi.product_id, p.name, p.stock
			ORDER BY SUM(oi.quantity)::numeric / GREATEST(p.stock, 1) DESC
			LIMIT 20`,
		"partition_pruning": `
			EXPLAIN ANALYZE
			SELECT COUNT(*) FROM orders
			WHERE created_at BETWEEN '2026-03-01' AND '2026-03-31'`,
		"full_scan_no_pruning": `
			EXPLAIN ANALYZE
			SELECT COUNT(*) FROM orders`,
	}

	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			rows, err := benchPool.Query(ctx, query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()

			t.Logf("=== EXPLAIN ANALYZE: %s ===", name)
			for rows.Next() {
				var line string
				if err := rows.Scan(&line); err != nil {
					t.Fatal(err)
				}
				t.Log(line)
			}
		})
	}
}
```

- [ ] **Step 2: Verify tests compile**

```bash
cd go/order-service && go test -tags=integration -c ./internal/reporting/ -o /dev/null 2>&1 || echo "Expected: compilation check"
```

Expected: Compiles (may warn about Docker if not available).

- [ ] **Step 3: Run benchmarks locally (requires Docker/colima)**

```bash
cd go/order-service && DOCKER_HOST=unix://$HOME/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -bench=. -benchtime=10s -timeout 300s ./internal/reporting/ -v
```

Expected: Benchmark results showing ns/op for each query. If Docker unavailable, tests skip gracefully.

- [ ] **Step 4: Commit**

```bash
git add go/order-service/internal/reporting/reporting_bench_test.go
git commit -m "feat(order): add benchmark suite comparing partitioned queries, matviews, and live aggregation"
```

---

### Task 9: Final Integration — Run All Tests

**Files:** None (verification only)

- [ ] **Step 1: Run order-service unit tests**

```bash
cd go/order-service && go test ./... -v -race
```

Expected: All pass.

- [ ] **Step 2: Run order-service lint**

```bash
cd go/order-service && golangci-lint run ./...
```

Expected: No errors.

- [ ] **Step 3: Run full preflight**

```bash
make preflight-go
```

Expected: All Go services pass lint + tests.

- [ ] **Step 4: Commit any remaining fixes**

If any lint or test fixes were needed, commit them:

```bash
git add -A && git commit -m "fix(order): address lint and test issues from reporting integration"
```
