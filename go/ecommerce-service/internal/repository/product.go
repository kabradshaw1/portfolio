package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var (
	ErrProductNotFound   = apperror.NotFound("PRODUCT_NOT_FOUND", "product not found")
	ErrInsufficientStock = apperror.Conflict("INSUFFICIENT_STOCK", "insufficient stock")
)

type ProductRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewProductRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *ProductRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &ProductRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

func (r *ProductRepository) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	type result struct {
		products []model.Product
		total    int
	}
	res, err := resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (result, error) {
		var args []any
		argIdx := 1

		var whereParts []string
		if params.Category != "" {
			whereParts = append(whereParts, fmt.Sprintf("category = $%d", argIdx))
			args = append(args, params.Category)
			argIdx++
		}
		if params.Query != "" {
			whereParts = append(whereParts, fmt.Sprintf("name ILIKE '%%' || $%d || '%%'", argIdx))
			args = append(args, params.Query)
			argIdx++
		}
		whereClause := ""
		if len(whereParts) > 0 {
			whereClause = " WHERE " + strings.Join(whereParts, " AND ")
		}

		countQuery := "SELECT COUNT(*) FROM products" + whereClause
		var total int
		if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return result{}, fmt.Errorf("count products: %w", err)
		}

		orderClause := " ORDER BY created_at DESC"
		switch params.Sort {
		case "price_asc":
			orderClause = " ORDER BY price ASC"
		case "price_desc":
			orderClause = " ORDER BY price DESC"
		case "name_asc":
			orderClause = " ORDER BY name ASC"
		}

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
			return result{}, fmt.Errorf("list products: %w", err)
		}
		defer rows.Close()

		var products []model.Product
		for rows.Next() {
			var p model.Product
			if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt); err != nil {
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

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Product, error) {
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
	})
}

func (r *ProductRepository) Categories(ctx context.Context) ([]string, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]string, error) {
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
	})
}

func (r *ProductRepository) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		tx, err := r.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		var stock int
		err = tx.QueryRow(ctx,
			"SELECT stock FROM products WHERE id = $1 FOR UPDATE",
			productID,
		).Scan(&stock)
		if err != nil {
			return fmt.Errorf("select for update: %w", err)
		}

		if stock < qty {
			return ErrInsufficientStock
		}

		_, err = tx.Exec(ctx,
			"UPDATE products SET stock = stock - $1, updated_at = NOW() WHERE id = $2",
			qty, productID,
		)
		if err != nil {
			return fmt.Errorf("update stock: %w", err)
		}

		return tx.Commit(ctx)
	})
}
