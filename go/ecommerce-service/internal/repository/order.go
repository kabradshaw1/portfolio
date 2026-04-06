package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
)

var ErrOrderNotFound = errors.New("order not found")

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) Create(ctx context.Context, userID uuid.UUID, total int, items []model.OrderItem) (*model.Order, error) {
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
}

func (r *OrderRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
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
}

func (r *OrderRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]model.Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, status, total, created_at, updated_at
		 FROM orders WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
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
	return orders, nil
}

func (r *OrderRepository) UpdateStatus(ctx context.Context, orderID uuid.UUID, status model.OrderStatus) error {
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
}
