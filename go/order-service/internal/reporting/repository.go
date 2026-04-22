package reporting

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

var errNoPool = errors.New("database pool is nil")

type Repository struct {
	pool    *pgxpool.Pool
	breaker *gobreaker.CircuitBreaker[any]
}

func NewRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *Repository {
	return &Repository{pool: pool, breaker: breaker}
}

func (r *Repository) checkPool() error {
	if r.pool == nil {
		return errNoPool
	}
	return nil
}

// SalesTrends returns daily revenue with rolling 7-day and 30-day windows.
func (r *Repository) SalesTrends(ctx context.Context, days int) ([]SalesTrend, error) {
	if err := r.checkPool(); err != nil {
		return nil, err
	}

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

	return resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func(ctx context.Context) ([]SalesTrend, error) {
		rows, err := r.pool.Query(ctx, query, days)
		if err != nil {
			return nil, err
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
}

// InventoryTurnover returns products ranked by turnover rate over a time window.
func (r *Repository) InventoryTurnover(ctx context.Context, days, limit int) ([]InventoryTurnover, error) {
	if err := r.checkPool(); err != nil {
		return nil, err
	}

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

	return resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func(ctx context.Context) ([]InventoryTurnover, error) {
		rows, err := r.pool.Query(ctx, query, days, limit)
		if err != nil {
			return nil, err
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
}

// TopCustomers returns customers ranked by total spend.
func (r *Repository) TopCustomers(ctx context.Context, limit int) ([]CustomerSummary, error) {
	if err := r.checkPool(); err != nil {
		return nil, err
	}

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

	return resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func(ctx context.Context) ([]CustomerSummary, error) {
		rows, err := r.pool.Query(ctx, query, limit)
		if err != nil {
			return nil, err
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
}

// ProductPerformance returns all products with aggregated metrics from the materialized view.
func (r *Repository) ProductPerformance(ctx context.Context) ([]ProductPerformance, error) {
	if err := r.checkPool(); err != nil {
		return nil, err
	}

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

	return resilience.Call(ctx, r.breaker, resilience.DefaultRetryConfig(), func(ctx context.Context) ([]ProductPerformance, error) {
		rows, err := r.pool.Query(ctx, query)
		if err != nil {
			return nil, err
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
}
