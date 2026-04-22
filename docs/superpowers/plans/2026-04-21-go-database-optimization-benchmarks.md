# Go Database Optimization & Benchmarks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real-PostgreSQL benchmark infrastructure, harden schemas, and optimize queries across product-service, order-service, and cart-service — producing measurable before/after evidence for the portfolio.

**Architecture:** Shared `go/pkg/dbtest/` testcontainers helper provides a real PostgreSQL container for benchmarks. Each service's `internal/repository/` gets `*_bench_test.go` files that seed data and measure queries. Schema hardening adds CHECK constraints and targeted indexes via new migrations. Query optimizations (batch inserts, window functions, CTE rewrites, prepared statement cache) are applied and re-benchmarked.

**Tech Stack:** Go 1.26, pgx/v5, testcontainers-go, PostgreSQL 16, golang-migrate

---

### Task 1: Testcontainers Helper Package (`go/pkg/dbtest/`)

**Files:**
- Create: `go/pkg/dbtest/dbtest.go`

This package provides `SetupPostgres` which spins up a real PostgreSQL container, runs migrations, and returns a connected `pgxpool.Pool`. Services call this from `TestMain` so the container is created once per test binary.

- [ ] **Step 1: Add testcontainers dependency to `go/pkg/`**

```bash
cd go/pkg && go get github.com/testcontainers/testcontainers-go@latest && go get github.com/testcontainers/testcontainers-go/modules/postgres@latest && go mod tidy
```

- [ ] **Step 2: Create `go/pkg/dbtest/dbtest.go`**

```go
package dbtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestDB holds a running PostgreSQL container and its connection pool.
type TestDB struct {
	Pool      *pgxpool.Pool
	container testcontainers.Container
}

// SetupPostgres starts a PostgreSQL container, runs migrations from
// migrationsDir, and returns a connected pool. Call Teardown when done.
func SetupPostgres(ctx context.Context, migrationsDir string) (*TestDB, error) {
	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve migrations dir: %w", err)
	}

	// Collect .up.sql files for init scripts (run in lexicographic order).
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var initScripts []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" && contains(e.Name(), ".up.") {
			initScripts = append(initScripts, filepath.Join(absDir, e.Name()))
		}
	}

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		postgres.WithInitScripts(initScripts...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return &TestDB{Pool: pool, container: container}, nil
}

// Teardown closes the pool and terminates the container.
func (tdb *TestDB) Teardown(ctx context.Context) {
	if tdb.Pool != nil {
		tdb.Pool.Close()
	}
	if tdb.container != nil {
		_ = tdb.container.Terminate(ctx)
	}
}

// SeedSQL executes a SQL file against the pool.
func (tdb *TestDB) SeedSQL(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read seed file: %w", err)
	}
	_, err = tdb.Pool.Exec(ctx, string(data))
	return err
}

// CaptureExplain runs EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) for a query
// and writes the JSON output to outPath.
func CaptureExplain(ctx context.Context, pool *pgxpool.Pool, outPath, query string, args ...any) error {
	explainQuery := "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) " + query
	var planJSON []byte
	err := pool.QueryRow(ctx, explainQuery, args...).Scan(&planJSON)
	if err != nil {
		return fmt.Errorf("explain analyze: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create benchdata dir: %w", err)
	}
	return os.WriteFile(outPath, planJSON, 0o644)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Run `go mod tidy` in `go/pkg/`**

```bash
cd go/pkg && go mod tidy
```

Expected: no errors, `go.mod` updated with testcontainers dependencies.

- [ ] **Step 4: Verify the package compiles**

```bash
cd go/pkg && go build ./dbtest/
```

Expected: clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add go/pkg/dbtest/dbtest.go go/pkg/go.mod go/pkg/go.sum
git commit -m "feat(pkg): add testcontainers-based dbtest helper for benchmarks"
```

---

### Task 2: Product Service Baseline Benchmarks

**Files:**
- Create: `go/product-service/internal/repository/product_bench_test.go`

Benchmarks run against a real PostgreSQL container with 1000 seeded products. Covers cursor pagination, offset pagination, FindByID, DecrementStock (parallel), and Categories.

- [ ] **Step 1: Add testcontainers dependency to product-service**

```bash
cd go/product-service && go get github.com/testcontainers/testcontainers-go@latest && go get github.com/testcontainers/testcontainers-go/modules/postgres@latest && go get github.com/jackc/pgx/v5@latest && go mod tidy
```

- [ ] **Step 2: Create `go/product-service/internal/repository/product_bench_test.go`**

```go
package repository

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/pagination"
)

var (
	benchPool *pgxpool.Pool
	benchTDB  *dbtest.TestDB
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	benchTDB, err = dbtest.SetupPostgres(ctx, "../../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup postgres: %v\n", err)
		os.Exit(1)
	}

	benchPool = benchTDB.Pool
	seedBenchData(ctx, benchPool)

	code := m.Run()
	benchTDB.Teardown(ctx)
	os.Exit(code)
}

func seedBenchData(ctx context.Context, pool *pgxpool.Pool) {
	categories := []string{"Electronics", "Clothing", "Home", "Books", "Sports"}
	for i := 0; i < 1000; i++ {
		_, err := pool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, '', $6, NOW() - ($7 || ' hours')::interval, NOW())`,
			uuid.New(),
			fmt.Sprintf("Bench Product %04d", i),
			fmt.Sprintf("Description for product %d", i),
			1000+rand.Intn(99000), // price 1000-99999 cents
			categories[i%len(categories)],
			50+rand.Intn(200), // stock 50-249
			i,                 // stagger created_at so ordering is deterministic
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed product %d: %v\n", i, err)
			os.Exit(1)
		}
	}
}

func newBenchRepo() *ProductRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench"})
	return NewProductRepository(benchPool, breaker)
}

func BenchmarkProductList_Cursor(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	// Get a cursor by fetching the first page.
	products, _, _ := repo.List(ctx, model.ProductListParams{Limit: 20, Sort: "price_asc"})
	if len(products) == 0 {
		b.Fatal("no products seeded")
	}
	last := products[len(products)-1]
	cursor := pagination.EncodeCursor(fmt.Sprintf("%d", last.Price), last.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit:  20,
			Sort:   "price_asc",
			Cursor: cursor,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_Offset(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit: 20,
			Page:  5, // offset=80, forces COUNT + data query
			Sort:  "price_asc",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_CategoryFilter(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit:    20,
			Page:     1,
			Category: "Electronics",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductList_SearchQuery(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := repo.List(ctx, model.ProductListParams{
			Limit: 20,
			Page:  1,
			Query: "Bench Product 05",
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductFindByID(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	// Grab a real product ID.
	var id uuid.UUID
	err := benchPool.QueryRow(ctx, "SELECT id FROM products LIMIT 1").Scan(&id)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FindByID(ctx, id)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProductDecrementStock(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	// Create products with enough stock for parallel decrements.
	ids := make([]uuid.UUID, 100)
	for i := range ids {
		id := uuid.New()
		ids[i] = id
		_, err := benchPool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock)
			 VALUES ($1, $2, 'bench', 1000, 'Electronics', '', $3)`,
			id, fmt.Sprintf("Stock Bench %d", i), 1_000_000,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = repo.DecrementStock(ctx, ids[i%len(ids)], 1)
			i++
		}
	})
}

func BenchmarkProductCategories(b *testing.B) {
	repo := newBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.Categories(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 3: Verify the `pagination` package has `EncodeCursor` and `DecodeCursor` available**

```bash
cd go/product-service && grep -n 'func EncodeCursor\|func DecodeCursor' internal/pagination/*.go
```

Expected: both functions exist. If `EncodeCursor` doesn't exist, we need to check what the pagination package exports and adjust the benchmark accordingly.

- [ ] **Step 4: Run `go mod tidy` and verify benchmarks compile**

```bash
cd go/product-service && go mod tidy && go test -run='^$' -bench=. -benchtime=1x -count=1 ./internal/repository/ 2>&1 | head -30
```

Expected: benchmarks run at least once. This validates the testcontainers setup works. The first run will be slow (container pull).

- [ ] **Step 5: Capture EXPLAIN ANALYZE baseline for key queries**

Add a test function at the bottom of `product_bench_test.go`:

```go
func TestCaptureExplainBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()

	basedir := "../../../benchdata/product-service/baseline"

	// Offset list with COUNT
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/list_offset_count.json",
		"SELECT COUNT(*) FROM products")
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/list_offset_data.json",
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products ORDER BY price ASC, id ASC LIMIT $1 OFFSET $2",
		20, 80)

	// FindByID
	var id uuid.UUID
	_ = benchPool.QueryRow(ctx, "SELECT id FROM products LIMIT 1").Scan(&id)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id.json",
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products WHERE id = $1",
		id)

	// Categories
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/categories.json",
		"SELECT DISTINCT category FROM products ORDER BY category")

	// DecrementStock FOR UPDATE
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/decrement_stock_select.json",
		"SELECT stock FROM products WHERE id = $1 FOR UPDATE",
		id)
}
```

- [ ] **Step 6: Run explain capture**

```bash
cd go/product-service && go test -run=TestCaptureExplainBaseline -count=1 ./internal/repository/
```

Expected: JSON files created under `go/benchdata/product-service/baseline/`.

- [ ] **Step 7: Commit**

```bash
git add go/product-service/internal/repository/product_bench_test.go go/product-service/go.mod go/product-service/go.sum go/benchdata/
git commit -m "feat(product-service): add real-PostgreSQL benchmark suite with baseline EXPLAIN captures"
```

---

### Task 3: Order Service Baseline Benchmarks

**Files:**
- Create: `go/order-service/internal/repository/order_bench_test.go`

Benchmarks cover order creation (1, 5, 20 items to expose N+1), FindByID with JOIN, ListByUser, and FindIncompleteSagas.

- [ ] **Step 1: Add testcontainers dependency to order-service**

```bash
cd go/order-service && go get github.com/testcontainers/testcontainers-go@latest && go get github.com/testcontainers/testcontainers-go/modules/postgres@latest && go mod tidy
```

- [ ] **Step 2: Create `go/order-service/internal/repository/order_bench_test.go`**

```go
package repository

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var (
	benchPool *pgxpool.Pool
	benchTDB  *dbtest.TestDB
)

var benchUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
var benchProductIDs []uuid.UUID

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	benchTDB, err = dbtest.SetupPostgres(ctx, "../../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup postgres: %v\n", err)
		os.Exit(1)
	}

	benchPool = benchTDB.Pool
	seedOrderBenchData(ctx, benchPool)

	code := m.Run()
	benchTDB.Teardown(ctx)
	os.Exit(code)
}

func seedOrderBenchData(ctx context.Context, pool *pgxpool.Pool) {
	// Seed 100 products (order-service has its own products table).
	benchProductIDs = make([]uuid.UUID, 100)
	categories := []string{"Electronics", "Clothing", "Home", "Books", "Sports"}
	for i := range benchProductIDs {
		id := uuid.New()
		benchProductIDs[i] = id
		_, err := pool.Exec(ctx,
			`INSERT INTO products (id, name, description, price, category, image_url, stock)
			 VALUES ($1, $2, $3, $4, $5, '', $6)`,
			id,
			fmt.Sprintf("Order Bench Product %03d", i),
			fmt.Sprintf("Description %d", i),
			1000+rand.Intn(50000),
			categories[i%len(categories)],
			100000,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed product %d: %v\n", i, err)
			os.Exit(1)
		}
	}

	// Seed 200 orders with 1-5 items each for benchUserID.
	for i := 0; i < 200; i++ {
		orderID := uuid.New()
		itemCount := 1 + rand.Intn(5)
		total := 0
		items := make([]struct{ pid uuid.UUID; qty, price int }, itemCount)
		for j := 0; j < itemCount; j++ {
			price := 1000 + rand.Intn(50000)
			qty := 1 + rand.Intn(3)
			items[j] = struct{ pid uuid.UUID; qty, price int }{
				pid:   benchProductIDs[rand.Intn(len(benchProductIDs))],
				qty:   qty,
				price: price,
			}
			total += price * qty
		}

		_, err := pool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, saga_step, total, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, NOW() - ($6 || ' hours')::interval, NOW())`,
			orderID, benchUserID,
			[]string{"pending", "completed", "processing"}[i%3],
			[]string{"CREATED", "COMPLETED", "STOCK_RESERVED", "PAYMENT_CONFIRMED"}[i%4],
			total, i,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "seed order %d: %v\n", i, err)
			os.Exit(1)
		}

		for _, item := range items {
			_, err := pool.Exec(ctx,
				`INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), orderID, item.pid, item.qty, item.price,
			)
			if err != nil {
				fmt.Fprintf(os.Stderr, "seed order item: %v\n", err)
				os.Exit(1)
			}
		}
	}
}

func newOrderBenchRepo() *OrderRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench-order"})
	return NewOrderRepository(benchPool, breaker)
}

func benchOrderCreate(b *testing.B, itemCount int) {
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		items := make([]model.OrderItem, itemCount)
		for j := range items {
			items[j] = model.OrderItem{
				ProductID:       benchProductIDs[j%len(benchProductIDs)],
				Quantity:        1,
				PriceAtPurchase: 2500,
			}
		}
		_, err := repo.Create(ctx, uuid.New(), 2500*itemCount, items)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOrderCreate_1Item(b *testing.B)   { benchOrderCreate(b, 1) }
func BenchmarkOrderCreate_5Items(b *testing.B)  { benchOrderCreate(b, 5) }
func BenchmarkOrderCreate_20Items(b *testing.B) { benchOrderCreate(b, 20) }

func BenchmarkOrderFindByID(b *testing.B) {
	repo := newOrderBenchRepo()
	ctx := context.Background()

	var id uuid.UUID
	err := benchPool.QueryRow(ctx, "SELECT id FROM orders LIMIT 1").Scan(&id)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FindByID(ctx, id)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOrderListByUser_Simple(b *testing.B) {
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.ListByUser(ctx, benchUserID, model.OrderListParams{Limit: 20})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOrderFindIncompleteSagas(b *testing.B) {
	repo := newOrderBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.FindIncompleteSagas(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCaptureOrderExplainBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()
	basedir := "../../../benchdata/order-service/baseline"

	// N+1 item insert (can't EXPLAIN an INSERT in a transaction meaningfully,
	// but we can capture the saga scan).
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_incomplete_sagas.json",
		"SELECT id FROM orders WHERE saga_step NOT IN ($1, $2, $3)",
		"COMPLETED", "COMPENSATION_COMPLETE", "FAILED")

	var orderID uuid.UUID
	_ = benchPool.QueryRow(ctx, "SELECT id FROM orders LIMIT 1").Scan(&orderID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id_order.json",
		"SELECT id, user_id, status, saga_step, total, created_at, updated_at FROM orders WHERE id = $1",
		orderID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/find_by_id_items.json",
		"SELECT oi.id, oi.order_id, oi.product_id, oi.quantity, oi.price_at_purchase, p.name FROM order_items oi JOIN products p ON p.id = oi.product_id WHERE oi.order_id = $1",
		orderID)
}
```

- [ ] **Step 3: Run `go mod tidy` and verify benchmarks compile**

```bash
cd go/order-service && go mod tidy && go test -run='^$' -bench=. -benchtime=1x -count=1 ./internal/repository/ 2>&1 | head -30
```

Expected: benchmarks run at least once.

- [ ] **Step 4: Capture EXPLAIN baselines**

```bash
cd go/order-service && go test -run=TestCaptureOrderExplainBaseline -count=1 ./internal/repository/
```

- [ ] **Step 5: Commit**

```bash
git add go/order-service/internal/repository/order_bench_test.go go/order-service/go.mod go/order-service/go.sum go/benchdata/
git commit -m "feat(order-service): add real-PostgreSQL benchmark suite with baseline EXPLAIN captures"
```

---

### Task 4: Cart Service Baseline Benchmarks

**Files:**
- Create: `go/cart-service/internal/repository/cart_bench_test.go`

Benchmarks cover GetByUser (5 and 50 items), AddItem (new and upsert), Reserve, and UpdateQuantity (success and reserved conflict).

- [ ] **Step 1: Add testcontainers dependency to cart-service**

```bash
cd go/cart-service && go get github.com/testcontainers/testcontainers-go@latest && go get github.com/testcontainers/testcontainers-go/modules/postgres@latest && go mod tidy
```

- [ ] **Step 2: Create `go/cart-service/internal/repository/cart_bench_test.go`**

```go
package repository

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/dbtest"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var (
	benchPool *pgxpool.Pool
	benchTDB  *dbtest.TestDB
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	benchTDB, err = dbtest.SetupPostgres(ctx, "../../../migrations")
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup postgres: %v\n", err)
		os.Exit(1)
	}

	benchPool = benchTDB.Pool

	code := m.Run()
	benchTDB.Teardown(ctx)
	os.Exit(code)
}

func newCartBenchRepo() *CartRepository {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "bench-cart"})
	return NewCartRepository(benchPool, breaker)
}

// seedCartForUser creates `count` cart items for a user and returns the user ID
// and a list of item IDs.
func seedCartForUser(ctx context.Context, pool *pgxpool.Pool, count int) (uuid.UUID, []uuid.UUID) {
	userID := uuid.New()
	itemIDs := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		itemID := uuid.New()
		itemIDs[i] = itemID
		_, err := pool.Exec(ctx,
			`INSERT INTO cart_items (id, user_id, product_id, quantity, created_at)
			 VALUES ($1, $2, $3, $4, NOW())`,
			itemID, userID, uuid.New(), 1+i%5,
		)
		if err != nil {
			panic(fmt.Sprintf("seed cart item %d: %v", i, err))
		}
	}
	return userID, itemIDs
}

func BenchmarkCartGetByUser_5Items(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, _ := seedCartForUser(ctx, benchPool, 5)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetByUser(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartGetByUser_50Items(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, _ := seedCartForUser(ctx, benchPool, 50)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.GetByUser(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartAddItem_New(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each iteration uses a unique user+product so it's always a fresh insert.
		_, err := repo.AddItem(ctx, uuid.New(), uuid.New(), 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartAddItem_Upsert(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	// Pre-create a cart item so every iteration hits the ON CONFLICT path.
	userID := uuid.New()
	productID := uuid.New()
	_, err := repo.AddItem(ctx, userID, productID, 1)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := repo.AddItem(ctx, userID, productID, 1)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartReserve(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Seed fresh unreserved items each iteration.
		userID, _ := seedCartForUser(ctx, benchPool, 10)
		b.StartTimer()

		err := repo.Reserve(ctx, userID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartUpdateQuantity_Success(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()
	userID, itemIDs := seedCartForUser(ctx, benchPool, 10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := repo.UpdateQuantity(ctx, itemIDs[i%len(itemIDs)], userID, 1+(i%10))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCartUpdateQuantity_ReservedConflict(b *testing.B) {
	repo := newCartBenchRepo()
	ctx := context.Background()

	// Seed items and reserve them.
	userID, itemIDs := seedCartForUser(ctx, benchPool, 10)
	_ = repo.Reserve(ctx, userID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will hit the two-query conflict path.
		_ = repo.UpdateQuantity(ctx, itemIDs[i%len(itemIDs)], userID, 5)
	}
}

func TestCaptureCartExplainBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping explain capture in short mode")
	}
	ctx := context.Background()
	basedir := "../../../benchdata/cart-service/baseline"

	userID, _ := seedCartForUser(ctx, benchPool, 20)

	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/get_by_user.json",
		"SELECT id, user_id, product_id, quantity, created_at FROM cart_items WHERE user_id = $1 ORDER BY created_at DESC",
		userID)
	_ = dbtest.CaptureExplain(ctx, benchPool, basedir+"/reserve.json",
		"UPDATE cart_items SET reserved = true WHERE user_id = $1 AND reserved = false",
		userID)
}
```

- [ ] **Step 3: Run `go mod tidy` and verify benchmarks compile**

```bash
cd go/cart-service && go mod tidy && go test -run='^$' -bench=. -benchtime=1x -count=1 ./internal/repository/ 2>&1 | head -30
```

Expected: benchmarks run at least once.

- [ ] **Step 4: Capture EXPLAIN baselines**

```bash
cd go/cart-service && go test -run=TestCaptureCartExplainBaseline -count=1 ./internal/repository/
```

- [ ] **Step 5: Commit**

```bash
git add go/cart-service/internal/repository/cart_bench_test.go go/cart-service/go.mod go/cart-service/go.sum go/benchdata/
git commit -m "feat(cart-service): add real-PostgreSQL benchmark suite with baseline EXPLAIN captures"
```

---

### Task 5: Run Full Baseline and Record Numbers

**Files:**
- Create: `go/benchdata/baseline-results.txt`

Run all three benchmark suites and capture the results to a file for later comparison.

- [ ] **Step 1: Run product-service benchmarks**

```bash
cd go/product-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/product-bench.txt
```

- [ ] **Step 2: Run order-service benchmarks**

```bash
cd go/order-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/order-bench.txt
```

- [ ] **Step 3: Run cart-service benchmarks**

```bash
cd go/cart-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/cart-bench.txt
```

- [ ] **Step 4: Combine into `go/benchdata/baseline-results.txt`**

```bash
mkdir -p go/benchdata
echo "=== BASELINE BENCHMARK RESULTS ===" > go/benchdata/baseline-results.txt
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> go/benchdata/baseline-results.txt
echo "" >> go/benchdata/baseline-results.txt
echo "--- Product Service ---" >> go/benchdata/baseline-results.txt
cat /tmp/product-bench.txt >> go/benchdata/baseline-results.txt
echo "" >> go/benchdata/baseline-results.txt
echo "--- Order Service ---" >> go/benchdata/baseline-results.txt
cat /tmp/order-bench.txt >> go/benchdata/baseline-results.txt
echo "" >> go/benchdata/baseline-results.txt
echo "--- Cart Service ---" >> go/benchdata/baseline-results.txt
cat /tmp/cart-bench.txt >> go/benchdata/baseline-results.txt
```

- [ ] **Step 5: Commit**

```bash
git add go/benchdata/baseline-results.txt
git commit -m "docs: record baseline benchmark results before optimization"
```

---

### Task 6: Schema Hardening — Product Service Migration

**Files:**
- Create: `go/product-service/migrations/003_add_constraints_and_indexes.up.sql`
- Create: `go/product-service/migrations/003_add_constraints_and_indexes.down.sql`

- [ ] **Step 1: Verify seed data won't violate new constraints**

```bash
cd go/product-service && grep -c 'price.*0\b' seed.sql
```

Check that no seed product has price=0 or stock<0. The seed data has prices from 100 to 999999 and stock from 20 to 999999, so constraints are safe.

- [ ] **Step 2: Create up migration**

Create `go/product-service/migrations/003_add_constraints_and_indexes.up.sql`:

```sql
-- Data integrity: prevent invalid prices and stock at the database level.
ALTER TABLE products ADD CONSTRAINT chk_products_price CHECK (price > 0);
ALTER TABLE products ADD CONSTRAINT chk_products_stock CHECK (stock >= 0);

-- Partial index for low-stock inventory alerting queries.
-- Only indexes rows with stock < 10, keeping the index small.
CREATE INDEX idx_products_low_stock ON products (stock) WHERE stock < 10;
```

- [ ] **Step 3: Create down migration**

Create `go/product-service/migrations/003_add_constraints_and_indexes.down.sql`:

```sql
DROP INDEX IF EXISTS idx_products_low_stock;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_stock;
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_price;
```

- [ ] **Step 4: Verify migration runs against testcontainers**

```bash
cd go/product-service && go test -run=TestMain -count=1 ./internal/repository/ -v 2>&1 | head -10
```

Expected: TestMain succeeds (testcontainers runs all .up.sql migrations including the new one).

- [ ] **Step 5: Commit**

```bash
git add go/product-service/migrations/003_add_constraints_and_indexes.up.sql go/product-service/migrations/003_add_constraints_and_indexes.down.sql
git commit -m "feat(product-service): add CHECK constraints and partial low-stock index"
```

---

### Task 7: Schema Hardening — Order Service Migration

**Files:**
- Create: `go/order-service/migrations/007_add_constraints_and_indexes.up.sql`
- Create: `go/order-service/migrations/007_add_constraints_and_indexes.down.sql`

- [ ] **Step 1: Create up migration**

Create `go/order-service/migrations/007_add_constraints_and_indexes.up.sql`:

```sql
-- Data integrity: prevent invalid totals, quantities, and prices.
ALTER TABLE orders ADD CONSTRAINT chk_orders_total CHECK (total > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_quantity CHECK (quantity > 0);
ALTER TABLE order_items ADD CONSTRAINT chk_order_items_price CHECK (price_at_purchase > 0);

-- Index for FindIncompleteSagas which filters by saga_step.
-- Without this, the query does a sequential scan on the orders table.
CREATE INDEX idx_orders_saga_step ON orders (saga_step);

-- Index for querying returns by status (common access pattern).
CREATE INDEX idx_returns_status ON returns (status);
```

- [ ] **Step 2: Create down migration**

Create `go/order-service/migrations/007_add_constraints_and_indexes.down.sql`:

```sql
DROP INDEX IF EXISTS idx_returns_status;
DROP INDEX IF EXISTS idx_orders_saga_step;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS chk_order_items_price;
ALTER TABLE order_items DROP CONSTRAINT IF EXISTS chk_order_items_quantity;
ALTER TABLE orders DROP CONSTRAINT IF EXISTS chk_orders_total;
```

- [ ] **Step 3: Verify migration runs**

```bash
cd go/order-service && go test -run=TestMain -count=1 ./internal/repository/ -v 2>&1 | head -10
```

- [ ] **Step 4: Commit**

```bash
git add go/order-service/migrations/007_add_constraints_and_indexes.up.sql go/order-service/migrations/007_add_constraints_and_indexes.down.sql
git commit -m "feat(order-service): add CHECK constraints, saga_step and returns status indexes"
```

---

### Task 8: Schema Hardening — Cart Service Migration

**Files:**
- Create: `go/cart-service/migrations/003_add_constraints_and_indexes.up.sql`
- Create: `go/cart-service/migrations/003_add_constraints_and_indexes.down.sql`

- [ ] **Step 1: Check existing cart constraint**

The cart-service migration `001_create_cart_items.up.sql` already has `CHECK (quantity > 0)`. Verify:

```bash
grep 'CHECK' go/cart-service/migrations/001_create_cart_items.up.sql
```

Expected: `quantity INTEGER NOT NULL CHECK (quantity > 0)`. If already present, skip the quantity constraint in the new migration and only add the composite index.

- [ ] **Step 2: Create up migration**

Create `go/cart-service/migrations/003_add_constraints_and_indexes.up.sql`:

```sql
-- Composite index for Reserve/Release queries that filter on (user_id, reserved).
-- Without this, the UPDATE scans all cart_items for the user then filters by reserved.
CREATE INDEX idx_cart_items_user_reserved ON cart_items (user_id, reserved);
```

Note: `CHECK (quantity > 0)` already exists in the base schema, so we skip it.

- [ ] **Step 3: Create down migration**

Create `go/cart-service/migrations/003_add_constraints_and_indexes.down.sql`:

```sql
DROP INDEX IF EXISTS idx_cart_items_user_reserved;
```

- [ ] **Step 4: Verify migration runs**

```bash
cd go/cart-service && go test -run=TestMain -count=1 ./internal/repository/ -v 2>&1 | head -10
```

- [ ] **Step 5: Commit**

```bash
git add go/cart-service/migrations/003_add_constraints_and_indexes.up.sql go/cart-service/migrations/003_add_constraints_and_indexes.down.sql
git commit -m "feat(cart-service): add composite index for reserve/release queries"
```

---

### Task 9: Error Handling Consistency Fixes

**Files:**
- Modify: `go/product-service/internal/repository/product.go:246`
- Modify: `go/order-service/internal/repository/order.go:80`
- Modify: `go/auth-service/internal/repository/user.go:40`

Replace fragile string-based error detection with typed pgx error checking.

- [ ] **Step 1: Fix product-service FindByID (line 246)**

In `go/product-service/internal/repository/product.go`, replace:

```go
if strings.Contains(err.Error(), "no rows") {
```

with:

```go
if errors.Is(err, pgx.ErrNoRows) {
```

Also add imports at the top of the file:

```go
"errors"
"github.com/jackc/pgx/v5"
```

And remove `"strings"` from the import block since it is still used by `buildWhereClause` — actually check first:

```bash
cd go/product-service && grep -n 'strings\.' internal/repository/product.go
```

If `strings` is used elsewhere (it is — `strings.Join` in buildWhereClause), keep the import. Just add `"errors"` and `"github.com/jackc/pgx/v5"`.

- [ ] **Step 2: Fix order-service FindByID (line 80)**

In `go/order-service/internal/repository/order.go`, replace:

```go
if strings.Contains(err.Error(), "no rows") {
```

with:

```go
if errors.Is(err, pgx.ErrNoRows) {
```

Add imports: `"errors"` and `"github.com/jackc/pgx/v5"`. Check if `"strings"` is still needed:

```bash
cd go/order-service && grep -n 'strings\.' internal/repository/order.go
```

If `strings` is only used for the `"no rows"` check, remove it.

- [ ] **Step 3: Fix auth-service Create (line 40)**

In `go/auth-service/internal/repository/user.go`, replace:

```go
if strings.Contains(err.Error(), "duplicate key") {
    return nil, ErrEmailExists
}
```

with:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" {
    return nil, ErrEmailExists
}
```

Add import: `"github.com/jackc/pgx/v5/pgconn"`. Remove `"strings"` if no longer used:

```bash
cd go/auth-service && grep -n 'strings\.' internal/repository/user.go
```

- [ ] **Step 4: Verify all tests still pass**

```bash
cd go/product-service && go test ./internal/repository/ -v
cd go/order-service && go test ./internal/repository/ -v
cd go/auth-service && go test ./internal/repository/ -v
```

- [ ] **Step 5: Commit**

```bash
git add go/product-service/internal/repository/product.go go/order-service/internal/repository/order.go go/auth-service/internal/repository/user.go
git commit -m "fix: replace string-based error detection with typed pgx error checks"
```

---

### Task 10: Run Post-Schema Benchmarks and Capture EXPLAIN

**Files:**
- Create: `go/benchdata/post-schema-results.txt`

Re-run benchmarks to measure the impact of schema changes alone (indexes, constraints).

- [ ] **Step 1: Run all three benchmark suites**

```bash
cd go/product-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/product-bench-post.txt
cd go/order-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/order-bench-post.txt
cd go/cart-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/cart-bench-post.txt
```

- [ ] **Step 2: Capture post-schema EXPLAIN artifacts**

```bash
cd go/product-service && go test -run=TestCaptureExplainBaseline -count=1 ./internal/repository/
cd go/order-service && go test -run=TestCaptureOrderExplainBaseline -count=1 ./internal/repository/
cd go/cart-service && go test -run=TestCaptureCartExplainBaseline -count=1 ./internal/repository/
```

Note: rename the output directories from `baseline` to `post-schema` in the EXPLAIN capture tests, or add separate capture functions for the post-schema phase. The simplest approach: update the `basedir` variable in each test to use an env var, or just copy the results:

```bash
for svc in product-service order-service cart-service; do
  cp -r go/benchdata/$svc/baseline go/benchdata/$svc/post-schema 2>/dev/null || true
done
```

Then re-run the capture tests — they'll overwrite the `baseline` dir. Move the originals first if desired.

- [ ] **Step 3: Combine results**

```bash
echo "=== POST-SCHEMA BENCHMARK RESULTS ===" > go/benchdata/post-schema-results.txt
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> go/benchdata/post-schema-results.txt
echo "" >> go/benchdata/post-schema-results.txt
echo "--- Product Service ---" >> go/benchdata/post-schema-results.txt
cat /tmp/product-bench-post.txt >> go/benchdata/post-schema-results.txt
echo "" >> go/benchdata/post-schema-results.txt
echo "--- Order Service ---" >> go/benchdata/post-schema-results.txt
cat /tmp/order-bench-post.txt >> go/benchdata/post-schema-results.txt
echo "" >> go/benchdata/post-schema-results.txt
echo "--- Cart Service ---" >> go/benchdata/post-schema-results.txt
cat /tmp/cart-bench-post.txt >> go/benchdata/post-schema-results.txt
```

- [ ] **Step 4: Commit**

```bash
git add go/benchdata/
git commit -m "docs: record post-schema benchmark results"
```

---

### Task 11: Query Optimization — Batch Insert for Order Items

**Files:**
- Modify: `go/order-service/internal/repository/order.go:51-60`

Replace the N+1 loop with a single multi-row INSERT.

- [ ] **Step 1: Modify `Create` method in `go/order-service/internal/repository/order.go`**

Replace lines 51-60 (the for loop):

```go
		for _, item := range items {
			_, err = tx.Exec(ctx,
				`INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase)
				 VALUES ($1, $2, $3, $4, $5)`,
				uuid.New(), order.ID, item.ProductID, item.Quantity, item.PriceAtPurchase,
			)
			if err != nil {
				return nil, fmt.Errorf("insert order item: %w", err)
			}
		}
```

with a batch insert:

```go
		if len(items) > 0 {
			query := "INSERT INTO order_items (id, order_id, product_id, quantity, price_at_purchase) VALUES "
			args := make([]any, 0, len(items)*5)
			for i, item := range items {
				if i > 0 {
					query += ", "
				}
				base := i*5 + 1
				query += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d)", base, base+1, base+2, base+3, base+4)
				args = append(args, uuid.New(), order.ID, item.ProductID, item.Quantity, item.PriceAtPurchase)
			}
			if _, err = tx.Exec(ctx, query, args...); err != nil {
				return nil, fmt.Errorf("insert order items: %w", err)
			}
		}
```

- [ ] **Step 2: Verify existing tests pass**

```bash
cd go/order-service && go test ./internal/repository/ -v
```

- [ ] **Step 3: Run order benchmarks to measure improvement**

```bash
cd go/order-service && go test -run='^$' -bench=BenchmarkOrderCreate -benchmem -count=3 ./internal/repository/
```

Expected: significant improvement on 5-item and 20-item benchmarks (3-5x).

- [ ] **Step 4: Commit**

```bash
git add go/order-service/internal/repository/order.go
git commit -m "perf(order-service): batch INSERT for order items eliminates N+1 pattern"
```

---

### Task 12: Query Optimization — Eliminate COUNT+Data Double Query

**Files:**
- Modify: `go/product-service/internal/repository/product.go:176-236`

Replace the separate COUNT query + data query with a single query using `COUNT(*) OVER()`.

- [ ] **Step 1: Modify `listByOffset` method**

Replace the entire `listByOffset` method body (lines 176-236) with:

```go
func (r *ProductRepository) listByOffset(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	type result struct {
		products []model.Product
		total    int
	}
	res, err := resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (result, error) {
		var args []any
		argIdx := 1
		whereParts := buildWhereClause(params, &args, &argIdx)

		whereClause := ""
		if len(whereParts) > 0 {
			whereClause = " WHERE " + strings.Join(whereParts, " AND ")
		}

		cfg := sortConfigForParam(params.Sort)

		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}
		page := params.Page
		if page <= 0 {
			page = 1
		}
		offset := (page - 1) * limit

		// Single query with window function replaces separate COUNT(*) + data query.
		query := fmt.Sprintf(
			`SELECT id, name, description, price, category, image_url, stock, created_at, updated_at,
			        COUNT(*) OVER() AS total_count
			 FROM products%s %s LIMIT $%d OFFSET $%d`,
			whereClause, cfg.orderClause, argIdx, argIdx+1,
		)
		args = append(args, limit, offset)

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return result{}, fmt.Errorf("list products: %w", err)
		}
		defer rows.Close()

		var products []model.Product
		var total int
		for rows.Next() {
			var p model.Product
			if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt, &total); err != nil {
				return result{}, fmt.Errorf("scan product: %w", err)
			}
			products = append(products, p)
		}

		return result{products: products, total: total}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return res.products, res.total, nil
}
```

- [ ] **Step 2: Verify existing tests pass**

```bash
cd go/product-service && go test ./... -v
```

- [ ] **Step 3: Run product benchmarks to measure improvement**

```bash
cd go/product-service && go test -run='^$' -bench=BenchmarkProductList_Offset -benchmem -count=3 ./internal/repository/
```

Expected: ~2x improvement (one query instead of two).

- [ ] **Step 4: Commit**

```bash
git add go/product-service/internal/repository/product.go
git commit -m "perf(product-service): eliminate COUNT+data double query with window function"
```

---

### Task 13: Query Optimization — CTE Cart Conflict Resolution

**Files:**
- Modify: `go/cart-service/internal/repository/cart.go:97-120`

Replace the two-query pattern in `UpdateQuantity` with a single CTE query.

- [ ] **Step 1: Modify `UpdateQuantity` method**

Replace lines 97-120 with:

```go
func (r *CartRepository) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		// Single CTE query replaces UPDATE + fallback SELECT EXISTS.
		var wasUpdated, isReserved bool
		err := r.pool.QueryRow(ctx,
			`WITH updated AS (
				UPDATE cart_items SET quantity = $1
				WHERE id = $2 AND user_id = $3 AND reserved = false
				RETURNING id
			)
			SELECT
				EXISTS(SELECT 1 FROM updated) AS was_updated,
				EXISTS(SELECT 1 FROM cart_items WHERE id = $2 AND user_id = $3 AND reserved = true) AS is_reserved`,
			quantity, itemID, userID,
		).Scan(&wasUpdated, &isReserved)
		if err != nil {
			return fmt.Errorf("update cart quantity: %w", err)
		}
		if !wasUpdated {
			if isReserved {
				return ErrItemReserved
			}
			return ErrCartItemNotFound
		}
		return nil
	})
}
```

- [ ] **Step 2: Verify existing tests pass**

```bash
cd go/cart-service && go test ./... -v
```

- [ ] **Step 3: Run cart benchmarks to measure improvement**

```bash
cd go/cart-service && go test -run='^$' -bench=BenchmarkCartUpdateQuantity -benchmem -count=3 ./internal/repository/
```

Expected: ~2x improvement on the reserved conflict path.

- [ ] **Step 4: Commit**

```bash
git add go/cart-service/internal/repository/cart.go
git commit -m "perf(cart-service): single CTE query for UpdateQuantity conflict resolution"
```

---

### Task 14: Query Optimization — Prepared Statement Cache

**Files:**
- Modify: `go/product-service/cmd/server/deps.go:17`
- Modify: `go/order-service/cmd/server/deps.go` (equivalent line)
- Modify: `go/cart-service/cmd/server/deps.go` (equivalent line)

Enable pgx's automatic prepared statement cache for all three services.

- [ ] **Step 1: Modify product-service `connectPostgres`**

In `go/product-service/cmd/server/deps.go`, after line 22 (`poolConfig.HealthCheckPeriod = 30 * time.Second`), add:

```go
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
```

Add import `"github.com/jackc/pgx/v5"` to the import block.

- [ ] **Step 2: Modify order-service `connectPostgres`**

Same change in `go/order-service/cmd/server/deps.go`. After the HealthCheckPeriod line, add:

```go
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
```

Add import `"github.com/jackc/pgx/v5"`.

- [ ] **Step 3: Modify cart-service `connectPostgres`**

Same change in `go/cart-service/cmd/server/deps.go`. After the HealthCheckPeriod line, add:

```go
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
```

Add import `"github.com/jackc/pgx/v5"`.

- [ ] **Step 4: Verify all services still compile**

```bash
cd go/product-service && go build ./cmd/server/
cd go/order-service && go build ./cmd/server/
cd go/cart-service && go build ./cmd/server/
```

- [ ] **Step 5: Commit**

```bash
git add go/product-service/cmd/server/deps.go go/order-service/cmd/server/deps.go go/cart-service/cmd/server/deps.go
git commit -m "perf: enable pgx prepared statement cache across all services"
```

---

### Task 15: Stock Decrement Locking Documentation

**Files:**
- Modify: `go/product-service/internal/repository/product.go:275-306`

Add documentation comments explaining the locking strategy trade-offs. No code change to the logic.

- [ ] **Step 1: Add locking strategy comment**

Above the `DecrementStock` method (before line 275), add:

```go
// DecrementStock atomically decreases a product's stock within a transaction.
//
// Locking strategy: pessimistic locking via SELECT ... FOR UPDATE.
// This acquires a row-level exclusive lock, preventing concurrent transactions
// from reading or modifying the same row until the lock holder commits/rollbacks.
//
// Trade-offs at different scales:
//   - Current approach (SELECT FOR UPDATE): Correct and simple. Holds the lock
//     for the duration of the transaction. At low contention (<100 QPS per SKU),
//     lock wait time is negligible. At high contention, transactions queue.
//   - pg_advisory_xact_lock(product_id): Application-level lock. Avoids row-lock
//     overhead but requires all writers to cooperate. Useful when the critical
//     section spans multiple tables or services.
//   - Optimistic locking (version column): UPDATE ... WHERE version = $expected.
//     No locks held during read. Retries on conflict. Better throughput under
//     moderate contention but wastes work on retry. Requires a version/updated_at
//     column and retry loop in application code.
//
// For this portfolio's scale, pessimistic locking is the right choice.
```

- [ ] **Step 2: Commit**

```bash
git add go/product-service/internal/repository/product.go
git commit -m "docs(product-service): document stock decrement locking strategy trade-offs"
```

---

### Task 16: Run Final Benchmarks and Record Optimized Results

**Files:**
- Create: `go/benchdata/optimized-results.txt`

Run all benchmarks one final time to capture the fully optimized state.

- [ ] **Step 1: Run all benchmark suites**

```bash
cd go/product-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/product-bench-final.txt
cd go/order-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/order-bench-final.txt
cd go/cart-service && go test -run='^$' -bench=. -benchmem -count=3 ./internal/repository/ 2>&1 | tee /tmp/cart-bench-final.txt
```

- [ ] **Step 2: Combine results**

```bash
echo "=== OPTIMIZED BENCHMARK RESULTS ===" > go/benchdata/optimized-results.txt
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> go/benchdata/optimized-results.txt
echo "" >> go/benchdata/optimized-results.txt
echo "--- Product Service ---" >> go/benchdata/optimized-results.txt
cat /tmp/product-bench-final.txt >> go/benchdata/optimized-results.txt
echo "" >> go/benchdata/optimized-results.txt
echo "--- Order Service ---" >> go/benchdata/optimized-results.txt
cat /tmp/order-bench-final.txt >> go/benchdata/optimized-results.txt
echo "" >> go/benchdata/optimized-results.txt
echo "--- Cart Service ---" >> go/benchdata/optimized-results.txt
cat /tmp/cart-bench-final.txt >> go/benchdata/optimized-results.txt
```

- [ ] **Step 3: Capture final EXPLAIN ANALYZE artifacts**

Re-run the EXPLAIN capture tests for all three services. The EXPLAIN output now reflects the new indexes and optimized queries.

```bash
cd go/product-service && go test -run=TestCaptureExplainBaseline -count=1 ./internal/repository/
cd go/order-service && go test -run=TestCaptureOrderExplainBaseline -count=1 ./internal/repository/
cd go/cart-service && go test -run=TestCaptureCartExplainBaseline -count=1 ./internal/repository/
```

- [ ] **Step 4: Commit**

```bash
git add go/benchdata/
git commit -m "docs: record final optimized benchmark results and EXPLAIN artifacts"
```

---

### Task 17: Preflight Checks

**Files:** None (validation only)

Run all preflight checks to ensure nothing is broken.

- [ ] **Step 1: Run Go preflight**

```bash
make preflight-go
```

Expected: lint passes, all tests pass (including new benchmark tests).

- [ ] **Step 2: Fix any lint issues**

If golangci-lint reports issues in new or modified files, fix them before proceeding.

- [ ] **Step 3: Commit any fixes**

```bash
git add -u && git commit -m "fix: address lint issues from benchmark and optimization work"
```

Only run this step if there were actual fixes needed.

---

### Task 18: Run `go mod tidy` Across All Services

**Files:** Various `go.mod` and `go.sum` files

Since we modified `go/pkg/` (added testcontainers), all services that depend on it need a tidy.

- [ ] **Step 1: Tidy all Go modules**

```bash
cd go/pkg && go mod tidy
cd go/product-service && go mod tidy
cd go/order-service && go mod tidy
cd go/cart-service && go mod tidy
cd go/auth-service && go mod tidy
cd go/ai-service && go mod tidy
cd go/analytics-service && go mod tidy
```

- [ ] **Step 2: Commit if any go.mod/go.sum changed**

```bash
git add go/*/go.mod go/*/go.sum go/pkg/go.mod go/pkg/go.sum
git diff --cached --quiet || git commit -m "chore: go mod tidy across all services"
```

---

### Task 19: Write ADR

**Files:**
- Create: `docs/adr/go-database-optimization.md`

Document the optimization journey — what was measured, what changed, and the results.

- [ ] **Step 1: Create the ADR**

Create `docs/adr/go-database-optimization.md` using the benchmark results from `go/benchdata/baseline-results.txt` and `go/benchdata/optimized-results.txt`. Structure:

```markdown
# ADR: Go Database Optimization

## Status
Accepted

## Context
The Go ecommerce services (product, order, cart) had functional but unoptimized database access. No real-database benchmarks existed — existing benchmarks used mocked repositories. Several anti-patterns were identified during audit:
- N+1 INSERT pattern in order creation
- COUNT + data double-query in product listing
- Two-query conflict resolution in cart updates
- Missing indexes on frequently filtered columns
- String-based error detection instead of typed pgx checks

## Decision
Apply a three-phase optimization approach: baseline benchmarks → schema hardening → query optimization, with measurements at each phase.

### Phase 1: Benchmark Infrastructure
- testcontainers-go for real PostgreSQL in tests
- Benchmarks measure ns/op, B/op, allocs/op
- EXPLAIN ANALYZE artifacts captured as JSON

### Phase 2: Schema Hardening
- CHECK constraints: `price > 0`, `stock >= 0`, `total > 0`, `quantity > 0`, `price_at_purchase > 0`
- New indexes: `idx_orders_saga_step`, `idx_returns_status`, `idx_cart_items_user_reserved`, `idx_products_low_stock` (partial)

### Phase 3: Query Optimizations
- Batch INSERT for order items (eliminates N+1)
- COUNT(*) OVER() window function (eliminates double query)
- CTE-based cart UpdateQuantity (eliminates fallback SELECT)
- pgx prepared statement cache (QueryExecModeCacheDescribe)
- Typed error handling (errors.Is, errors.As with pgconn.PgError)

## Results
[Insert actual benchmark comparison from baseline-results.txt vs optimized-results.txt]

## Consequences
- Benchmarks require Docker (testcontainers) — CI must have Docker available
- Prepared statement cache changes query execution mode for all connections
- Window function adds per-row overhead on offset pagination (acceptable trade-off vs. double query)
```

Fill in the Results section with actual numbers from the benchmark output files.

- [ ] **Step 2: Commit**

```bash
git add docs/adr/go-database-optimization.md
git commit -m "docs: add ADR documenting Go database optimization journey"
```
