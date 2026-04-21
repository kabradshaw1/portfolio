package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var ErrCartItemNotFound = apperror.NotFound("CART_ITEM_NOT_FOUND", "cart item not found")

type CartRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewCartRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *CartRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &CartRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

func (r *CartRepository) GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]model.CartItem, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT id, user_id, product_id, quantity, created_at
			 FROM cart_items
			 WHERE user_id = $1
			 ORDER BY created_at DESC`,
			userID,
		)
		if err != nil {
			return nil, fmt.Errorf("get cart: %w", err)
		}
		defer rows.Close()

		var items []model.CartItem
		for rows.Next() {
			var item model.CartItem
			if err := rows.Scan(
				&item.ID, &item.UserID, &item.ProductID, &item.Quantity, &item.CreatedAt,
			); err != nil {
				return nil, fmt.Errorf("scan cart item: %w", err)
			}
			items = append(items, item)
		}
		return items, nil
	})
}

func (r *CartRepository) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.CartItem, error) {
		var item model.CartItem
		err := r.pool.QueryRow(ctx,
			`INSERT INTO cart_items (id, user_id, product_id, quantity, created_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (user_id, product_id)
			 DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity
			 RETURNING id, user_id, product_id, quantity, created_at`,
			uuid.New(), userID, productID, quantity,
		).Scan(&item.ID, &item.UserID, &item.ProductID, &item.Quantity, &item.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("add cart item: %w", err)
		}
		return &item, nil
	})
}

func (r *CartRepository) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		result, err := r.pool.Exec(ctx,
			"UPDATE cart_items SET quantity = $1 WHERE id = $2 AND user_id = $3",
			quantity, itemID, userID,
		)
		if err != nil {
			return fmt.Errorf("update cart quantity: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrCartItemNotFound
		}
		return nil
	})
}

func (r *CartRepository) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		result, err := r.pool.Exec(ctx,
			"DELETE FROM cart_items WHERE id = $1 AND user_id = $2",
			itemID, userID,
		)
		if err != nil {
			return fmt.Errorf("remove cart item: %w", err)
		}
		if result.RowsAffected() == 0 {
			return ErrCartItemNotFound
		}
		return nil
	})
}

func (r *CartRepository) ClearCart(ctx context.Context, userID uuid.UUID) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx, "DELETE FROM cart_items WHERE user_id = $1", userID)
		if err != nil {
			return fmt.Errorf("clear cart: %w", err)
		}
		return nil
	})
}
