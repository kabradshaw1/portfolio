package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var ErrOrderNotFound = apperror.NotFound("ORDER_NOT_FOUND", "order not found")

type OrderRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewOrderRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *OrderRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &OrderRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

func (r *OrderRepository) Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Order, error) {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var order model.Order
		err = tx.QueryRow(ctx,
			`INSERT INTO orders (id, user_id, status, total, created_at, updated_at)
			 VALUES ($1, $2, 'pending', $3, NOW(), NOW())
			 RETURNING id, user_id, status, total, created_at, updated_at`,
			uuid.New(), userID, total,
		).Scan(&order.ID, &order.UserID, &order.Status, &order.Total, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("insert order: %w", err)
		}

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

		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}

		order.Items = items
		return &order, nil
	})
}

func (r *OrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Order, error) {
		var order model.Order
		err := r.pool.QueryRow(ctx,
			`SELECT id, user_id, status, total, created_at, updated_at
			 FROM orders WHERE id = $1`,
			id,
		).Scan(&order.ID, &order.UserID, &order.Status, &order.Total, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			if strings.Contains(err.Error(), "no rows") {
				return nil, ErrOrderNotFound
			}
			return nil, fmt.Errorf("find order: %w", err)
		}

		rows, err := r.pool.Query(ctx,
			`SELECT oi.id, oi.order_id, oi.product_id, oi.quantity, oi.price_at_purchase, p.name
			 FROM order_items oi
			 JOIN products p ON p.id = oi.product_id
			 WHERE oi.order_id = $1`,
			id,
		)
		if err != nil {
			return nil, fmt.Errorf("find order items: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var item model.OrderItem
			if err := rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &item.PriceAtPurchase, &item.ProductName); err != nil {
				return nil, fmt.Errorf("scan order item: %w", err)
			}
			order.Items = append(order.Items, item)
		}

		return &order, nil
	})
}

func (r *OrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, params model.OrderListParams) ([]model.Order, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]model.Order, error) {
		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}

		var rows interface {
			Next() bool
			Scan(dest ...any) error
			Close()
			Err() error
		}
		var err error

		if params.Cursor != "" {
			cursorTime, cursorID, decErr := pagination.DecodeCursor(params.Cursor)
			if decErr != nil {
				return nil, fmt.Errorf("invalid cursor: %w", decErr)
			}
			t, parseErr := time.Parse(time.RFC3339Nano, cursorTime)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid cursor time: %w", parseErr)
			}
			rows, err = r.pool.Query(ctx,
				`SELECT id, user_id, status, total, created_at, updated_at
				 FROM orders
				 WHERE user_id = $1 AND (created_at, id) < ($2, $3)
				 ORDER BY created_at DESC, id DESC
				 LIMIT $4`,
				userID, t, cursorID, limit+1,
			)
		} else {
			rows, err = r.pool.Query(ctx,
				`SELECT id, user_id, status, total, created_at, updated_at
				 FROM orders WHERE user_id = $1
				 ORDER BY created_at DESC, id DESC
				 LIMIT $2`,
				userID, limit+1,
			)
		}
		if err != nil {
			return nil, fmt.Errorf("list orders: %w", err)
		}
		defer rows.Close()

		var orders []model.Order
		for rows.Next() {
			var o model.Order
			if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.Total, &o.CreatedAt, &o.UpdatedAt); err != nil {
				return nil, fmt.Errorf("scan order: %w", err)
			}
			orders = append(orders, o)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows error: %w", err)
		}
		return orders, nil
	})
}

func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		result, err := r.pool.Exec(ctx,
			"UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
			status, orderID,
		)
		if err != nil {
			return fmt.Errorf("update order status: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrOrderNotFound
		}
		return nil
	})
}
