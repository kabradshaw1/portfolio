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

var ErrCartItemNotFound = errors.New("cart item not found")

type CartRepository struct {
	pool *pgxpool.Pool
}

func NewCartRepository(pool *pgxpool.Pool) *CartRepository {
	return &CartRepository{pool: pool}
}

func (r *CartRepository) GetByUser(ctx context.Context, userID uuid.UUID) ([]model.CartItem, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ci.id, ci.user_id, ci.product_id, ci.quantity, ci.created_at,
		        p.name, p.price, p.image_url
		 FROM cart_items ci
		 JOIN products p ON p.id = ci.product_id
		 WHERE ci.user_id = $1
		 ORDER BY ci.created_at DESC`,
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
			&item.ProductName, &item.ProductPrice, &item.ProductImage,
		); err != nil {
			return nil, fmt.Errorf("scan cart item: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *CartRepository) AddItem(ctx context.Context, userID, productID uuid.UUID, quantity int) (*model.CartItem, error) {
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
		if isDuplicateKeyError(err) {
			return nil, fmt.Errorf("add cart item: %w", err)
		}
		return nil, fmt.Errorf("add cart item: %w", err)
	}
	return &item, nil
}

func (r *CartRepository) UpdateQuantity(ctx context.Context, itemID, userID uuid.UUID, quantity int) error {
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
}

func (r *CartRepository) RemoveItem(ctx context.Context, itemID, userID uuid.UUID) error {
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
}

func (r *CartRepository) ClearCart(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM cart_items WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("clear cart: %w", err)
	}
	return nil
}

func isDuplicateKeyError(err error) bool {
	return strings.Contains(err.Error(), "duplicate key")
}
