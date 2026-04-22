package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var (
	ErrProductNotFound   = apperror.NotFound("PRODUCT_NOT_FOUND", "product not found")
	ErrInsufficientStock = apperror.Conflict("INSUFFICIENT_STOCK", "insufficient stock")
)

// sortConfig describes how to order product queries and, for cursor pagination,
// how to parse the cursor's sort value and build the comparator condition.
type sortConfig struct {
	orderClause string
	comparator  string
	sortCol     string
	parseValue  func(string) (any, error)
}

// sortConfigForParam returns the sort configuration for a given sort parameter.
func sortConfigForParam(sort string) sortConfig {
	switch sort {
	case "price_asc":
		return sortConfig{
			orderClause: "ORDER BY price ASC, id ASC",
			comparator:  ">",
			sortCol:     "price",
			parseValue: func(v string) (any, error) {
				var n int
				_, err := fmt.Sscanf(v, "%d", &n)
				return n, err
			},
		}
	case "price_desc":
		return sortConfig{
			orderClause: "ORDER BY price DESC, id DESC",
			comparator:  "<",
			sortCol:     "price",
			parseValue: func(v string) (any, error) {
				var n int
				_, err := fmt.Sscanf(v, "%d", &n)
				return n, err
			},
		}
	case "name_asc":
		return sortConfig{
			orderClause: "ORDER BY name ASC, id ASC",
			comparator:  ">",
			sortCol:     "name",
			parseValue:  func(v string) (any, error) { return v, nil },
		}
	default:
		return sortConfig{
			orderClause: "ORDER BY created_at DESC, id DESC",
			comparator:  "<",
			sortCol:     "created_at",
			parseValue: func(v string) (any, error) {
				return time.Parse(time.RFC3339Nano, v)
			},
		}
	}
}

// buildWhereClause appends category/query filter conditions and returns
// the assembled WHERE parts.
func buildWhereClause(params model.ProductListParams, args *[]any, argIdx *int) []string {
	var whereParts []string
	if params.Category != "" {
		whereParts = append(whereParts, fmt.Sprintf("category = $%d", *argIdx))
		*args = append(*args, params.Category)
		(*argIdx)++
	}
	if params.Query != "" {
		whereParts = append(whereParts, fmt.Sprintf("name ILIKE '%%' || $%d || '%%'", *argIdx))
		*args = append(*args, params.Query)
		(*argIdx)++
	}
	return whereParts
}

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
	if params.Cursor != "" {
		return r.listByCursor(ctx, params)
	}
	return r.listByOffset(ctx, params)
}

func (r *ProductRepository) listByCursor(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	type result struct {
		products []model.Product
	}
	res, err := resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (result, error) {
		sortValue, cursorID, err := pagination.DecodeCursor(params.Cursor)
		if err != nil {
			return result{}, apperror.BadRequest("INVALID_CURSOR", "invalid cursor")
		}

		var args []any
		argIdx := 1
		whereParts := buildWhereClause(params, &args, &argIdx)

		cfg := sortConfigForParam(params.Sort)
		parsedSortValue, err := cfg.parseValue(sortValue)
		if err != nil {
			return result{}, apperror.BadRequest("INVALID_CURSOR", "invalid cursor sort value")
		}

		// Add cursor condition: (sort_col, id) <comparator> ($val, $id)
		whereParts = append(whereParts, fmt.Sprintf("(%s, id) %s ($%d, $%d)", cfg.sortCol, cfg.comparator, argIdx, argIdx+1))
		args = append(args, parsedSortValue, cursorID)
		argIdx += 2

		whereClause := ""
		if len(whereParts) > 0 {
			whereClause = " WHERE " + strings.Join(whereParts, " AND ")
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 20
		}
		// Fetch limit+1 to determine hasMore
		query := fmt.Sprintf(
			"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products%s %s LIMIT $%d",
			whereClause, cfg.orderClause, argIdx,
		)
		args = append(args, limit+1)

		rows, err := r.pool.Query(ctx, query, args...)
		if err != nil {
			return result{}, fmt.Errorf("list products cursor: %w", err)
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

		return result{products: products}, nil
	})
	if err != nil {
		return nil, 0, err
	}
	return res.products, 0, nil
}

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

func (r *ProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*model.Product, error) {
		var p model.Product
		err := r.pool.QueryRow(ctx,
			"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products WHERE id = $1",
			id,
		).Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Category, &p.ImageURL, &p.Stock, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrProductNotFound
			}
			return nil, fmt.Errorf("find product: %w", err)
		}
		return &p, nil
	})
}

func (r *ProductRepository) Categories(ctx context.Context) ([]string, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]string, error) {
		rows, err := r.pool.Query(ctx, "SELECT DISTINCT category FROM products ORDER BY category LIMIT 100")
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
