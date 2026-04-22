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
	db.Teardown(ctx)
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
