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

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type ProductRepository struct {
	pool *pgxpool.Pool
}

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

func (r *ProductRepository) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	var args []any
	argIdx := 1

	whereClause := ""
	if params.Category != "" {
		whereClause = fmt.Sprintf(" WHERE category = $%d", argIdx)
		args = append(args, params.Category)
		argIdx++
	}

	// Count query
	countQuery := "SELECT COUNT(*) FROM products" + whereClause
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	// Sort
	orderClause := " ORDER BY created_at DESC"
	switch params.Sort {
	case "price_asc":
		orderClause = " ORDER BY price ASC"
	case "price_desc":
		orderClause = " ORDER BY price DESC"
	case "name_asc":
		orderClause = " ORDER BY name ASC"
	}

	// Pagination
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	query := fmt.Sprintf(
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products%s%s LIMIT $%d OFFSET $%d",
		whereClause, orderClause, argIdx, argIdx+1,
	)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []model.Product
	for rows.Next() {
		var p model.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, p)
	}

	return products, total, nil
}

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var p model.Product
	err := r.pool.QueryRow(ctx,
		"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products WHERE id = $1",
		id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("find product: %w", err)
	}
	return &p, nil
}

func (r *ProductRepository) Categories(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT DISTINCT category FROM products ORDER BY category")
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, cat)
	}
	return categories, nil
}

func (r *ProductRepository) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	result, err := r.pool.Exec(ctx,
		"UPDATE products SET stock = stock - $1, updated_at = NOW() WHERE id = $2 AND stock >= $1",
		qty, productID,
	)
	if err != nil {
		return fmt.Errorf("decrement stock: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrInsufficientStock
	}
	return nil
}
