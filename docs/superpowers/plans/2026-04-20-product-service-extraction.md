# Product Service Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract product/category functionality from ecommerce-service into a standalone product-service with REST + gRPC dual server, establishing the proto toolchain and gRPC patterns for Phases 2 and 3.

**Architecture:** Product-service runs REST (:8095) and gRPC (:9095) servers sharing a single service layer. The REST API serves frontend traffic unchanged. The gRPC API exposes product data and stock management for inter-service calls. During this phase, the products table exists in both `productdb` (authoritative) and `ecommercedb` (read-only for cart/order JOINs).

**Tech Stack:** Go 1.26, Gin, gRPC, protobuf/buf, pgxpool, go-redis, OTel, Prometheus, Kubernetes

---

### Task 1: Scaffold product-service module and config

**Files:**
- Create: `go/product-service/go.mod`
- Create: `go/product-service/cmd/server/config.go`
- Create: `go/product-service/cmd/server/deps.go`

- [ ] **Step 1: Initialize the Go module**

```bash
cd go/product-service && go mod init github.com/kabradshaw1/portfolio/go/product-service
```

Then edit `go.mod` to:

```go
module github.com/kabradshaw1/portfolio/go/product-service

go 1.26.1

require (
	github.com/kabradshaw1/portfolio/go/pkg v0.0.0
)

replace github.com/kabradshaw1/portfolio/go/pkg => ../pkg
```

- [ ] **Step 2: Create config.go**

```go
package main

import (
	"log"
	"os"
)

type Config struct {
	DatabaseURL    string
	AllowedOrigins string
	Port           string // REST port, default "8095"
	GRPCPort       string // gRPC port, default "9095"
	RedisURL       string
	OTELEndpoint   string
}

func loadConfig() Config {
	cfg := Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
		Port:           os.Getenv("PORT"),
		GRPCPort:       os.Getenv("GRPC_PORT"),
		RedisURL:       os.Getenv("REDIS_URL"),
		OTELEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if cfg.AllowedOrigins == "" {
		cfg.AllowedOrigins = "http://localhost:3000"
	}
	if cfg.Port == "" {
		cfg.Port = "8095"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9095"
	}

	return cfg
}
```

- [ ] **Step 3: Create deps.go**

Copy connection helpers from `go/ecommerce-service/cmd/server/deps.go` but only include `connectPostgres` and `connectRedis` (no RabbitMQ or Kafka):

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database URL: %v", err)
	}
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")
	return pool
}

func connectRedis(ctx context.Context, redisURL string) *redis.Client {
	if redisURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("failed to parse REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("redis not available, continuing without cache", "error", err)
		return nil
	}
	slog.Info("connected to redis")
	return client
}
```

- [ ] **Step 4: Run `go mod tidy`**

```bash
cd go/product-service && go get github.com/jackc/pgx/v5@v5.9.1 github.com/redis/go-redis/v9@v9.18.0 && go mod tidy
```

- [ ] **Step 5: Commit**

```bash
git add go/product-service/go.mod go/product-service/go.sum go/product-service/cmd/server/config.go go/product-service/cmd/server/deps.go
git commit -m "feat(product-service): scaffold module, config, and connection helpers"
```

---

### Task 2: Copy product models, validation, pagination, and metrics

**Files:**
- Create: `go/product-service/internal/model/product.go`
- Create: `go/product-service/internal/validate/validate.go`
- Create: `go/product-service/internal/pagination/cursor.go`
- Create: `go/product-service/internal/metrics/metrics.go`

- [ ] **Step 1: Create model/product.go**

Copy from `go/ecommerce-service/internal/model/product.go` — change only the module path in imports:

```go
package model

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       int       `json:"price"`
	Category    string    `json:"category"`
	ImageURL    string    `json:"imageUrl"`
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ProductListParams struct {
	Category string
	Query    string
	Sort     string
	Page     int
	Limit    int
	Cursor   string
}

type ProductListResponse struct {
	Products   []Product `json:"products"`
	Total      int       `json:"total,omitempty"`
	Page       int       `json:"page,omitempty"`
	Limit      int       `json:"limit"`
	NextCursor string    `json:"nextCursor,omitempty"`
	HasMore    bool      `json:"hasMore"`
}
```

- [ ] **Step 2: Create validate/validate.go**

Only include product-related validation (not cart/order/return):

```go
package validate

import "github.com/kabradshaw1/portfolio/go/pkg/apperror"

const (
	minPage  = 1
	minLimit = 1
	maxLimit = 100
)

var validSortValues = map[string]bool{
	"created_at_desc": true,
	"price_asc":       true,
	"price_desc":      true,
	"name_asc":        true,
}

func ProductListParams(sort string, page, limit int) []apperror.FieldError {
	var errs []apperror.FieldError

	if sort != "" && !validSortValues[sort] {
		errs = append(errs, apperror.FieldError{
			Field:   "sort",
			Message: "must be one of: created_at_desc, price_asc, price_desc, name_asc",
		})
	}

	if page < minPage {
		errs = append(errs, apperror.FieldError{Field: "page", Message: "must be at least 1"})
	}

	if limit < minLimit || limit > maxLimit {
		errs = append(errs, apperror.FieldError{Field: "limit", Message: "must be between 1 and 100"})
	}

	return errs
}
```

- [ ] **Step 3: Create pagination/cursor.go**

Copy from `go/ecommerce-service/internal/pagination/cursor.go` unchanged:

```go
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type cursorPayload struct {
	Value string `json:"v"`
	ID    string `json:"id"`
}

func EncodeCursor(sortValue string, id uuid.UUID) string {
	payload := cursorPayload{Value: sortValue, ID: id.String()}
	data, _ := json.Marshal(payload)
	return base64.URLEncoding.EncodeToString(data)
}

func DecodeCursor(cursor string) (string, uuid.UUID, error) {
	if cursor == "" {
		return "", uuid.Nil, fmt.Errorf("cursor is empty")
	}
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor format: %w", err)
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("invalid cursor ID: %w", err)
	}
	return payload.Value, id, nil
}
```

- [ ] **Step 4: Create metrics/metrics.go**

Only product-relevant metrics:

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ProductViews = promauto.NewCounter(prometheus.CounterOpts{
		Name: "product_views_total",
		Help: "Total individual product page views.",
	})

	CacheOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "product_cache_operations_total",
		Help: "Redis cache operations.",
	}, []string{"operation", "result"})
)
```

- [ ] **Step 5: Run `go mod tidy`**

```bash
cd go/product-service && go mod tidy
```

- [ ] **Step 6: Commit**

```bash
git add go/product-service/internal/
git commit -m "feat(product-service): add models, validation, pagination, and metrics"
```

---

### Task 3: Copy repository layer with tests

**Files:**
- Create: `go/product-service/internal/repository/product.go`
- Create: `go/product-service/internal/repository/product_test.go`

- [ ] **Step 1: Create repository/product.go**

Copy from `go/ecommerce-service/internal/repository/product.go` — update import paths from `github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/` to `github.com/kabradshaw1/portfolio/go/product-service/internal/`:

```go
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

type sortConfig struct {
	orderClause string
	comparator  string
	sortCol     string
	parseValue  func(string) (any, error)
}

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

		countQuery := "SELECT COUNT(*) FROM products" + whereClause
		var total int
		if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return result{}, fmt.Errorf("count products: %w", err)
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

		query := fmt.Sprintf(
			"SELECT id, name, description, price, category, image_url, stock, created_at, updated_at FROM products%s %s LIMIT $%d OFFSET $%d",
			whereClause, cfg.orderClause, argIdx, argIdx+1,
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
```

- [ ] **Step 2: Write a unit test for repository construction**

This is a minimal compile-check test — full repository tests require a database (integration tests, deferred to a future issue). Create `go/product-service/internal/repository/product_test.go`:

```go
package repository

import (
	"testing"

	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestNewProductRepository(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := NewProductRepository(nil, breaker)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}
```

- [ ] **Step 3: Run test to verify it passes**

```bash
cd go/product-service && go mod tidy && go test ./internal/repository/ -v -race
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/product-service/internal/repository/
git commit -m "feat(product-service): add product repository with resilience wrappers"
```

---

### Task 4: Copy service layer with tests

**Files:**
- Create: `go/product-service/internal/service/product.go`
- Create: `go/product-service/internal/service/product_test.go`

- [ ] **Step 1: Create service/product.go**

Copy from `go/ecommerce-service/internal/service/product.go` — update import paths:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
)

func getFromCache[T any](ctx context.Context, r *redis.Client, key string) (T, bool) {
	var zero T
	if r == nil {
		return zero, false
	}
	cached, err := r.Get(ctx, key).Result()
	if err != nil {
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
		return zero, false
	}
	var val T
	if json.Unmarshal([]byte(cached), &val) != nil {
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
		return zero, false
	}
	metrics.CacheOps.WithLabelValues("get", "hit").Inc()
	return val, true
}

func setInCache[T any](ctx context.Context, r *redis.Client, key string, val T, ttl time.Duration) {
	if r == nil {
		return
	}
	if data, err := json.Marshal(val); err == nil {
		r.Set(ctx, key, data, ttl)
		metrics.CacheOps.WithLabelValues("set", "success").Inc()
	}
}

type ProductRepo interface {
	List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Categories(ctx context.Context) ([]string, error)
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
}

type ProductService struct {
	repo  ProductRepo
	redis *redis.Client
}

func NewProductService(repo ProductRepo, redisClient *redis.Client) *ProductService {
	return &ProductService{repo: repo, redis: redisClient}
}

func (s *ProductService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	if params.Cursor != "" {
		return s.repo.List(ctx, params)
	}

	cacheKey := fmt.Sprintf("product:list:%s:%s:%d:%d", params.Category, params.Sort, params.Page, params.Limit)

	if resp, ok := getFromCache[model.ProductListResponse](ctx, s.redis, cacheKey); ok {
		return resp.Products, resp.Total, nil
	}

	products, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	setInCache(ctx, s.redis, cacheKey, model.ProductListResponse{
		Products: products, Total: total, Page: params.Page, Limit: params.Limit,
	}, 5*time.Minute)

	return products, total, nil
}

func (s *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	cacheKey := fmt.Sprintf("product:%s", id.String())

	if p, ok := getFromCache[model.Product](ctx, s.redis, cacheKey); ok {
		return &p, nil
	}

	product, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	setInCache(ctx, s.redis, cacheKey, product, 5*time.Minute)
	return product, nil
}

func (s *ProductService) Categories(ctx context.Context) ([]string, error) {
	cacheKey := "product:categories"

	if cats, ok := getFromCache[[]string](ctx, s.redis, cacheKey); ok {
		return cats, nil
	}

	cats, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, err
	}

	setInCache(ctx, s.redis, cacheKey, cats, 30*time.Minute)
	return cats, nil
}

func (s *ProductService) InvalidateCache(ctx context.Context) error {
	if s.redis == nil {
		return nil
	}

	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "product:*", 100).Result()
		if err != nil {
			return fmt.Errorf("scan product keys: %w", err)
		}
		if len(keys) > 0 {
			s.redis.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}
```

Note: Cache key prefix changed from `ecom:products:` to `product:` — this service owns its own cache namespace.

- [ ] **Step 2: Create service/product_test.go**

```go
package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/service"
)

type mockProductRepo struct {
	products []model.Product
}

func (m *mockProductRepo) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	var filtered []model.Product
	for _, p := range m.products {
		if params.Category != "" && p.Category != params.Category {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, len(filtered), nil
}

func (m *mockProductRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	for _, p := range m.products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("product not found")
}

func (m *mockProductRepo) Categories(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	var cats []string
	for _, p := range m.products {
		if !seen[p.Category] {
			seen[p.Category] = true
			cats = append(cats, p.Category)
		}
	}
	return cats, nil
}

func (m *mockProductRepo) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	for i, p := range m.products {
		if p.ID == productID {
			if p.Stock < qty {
				return fmt.Errorf("insufficient stock")
			}
			m.products[i].Stock -= qty
			return nil
		}
	}
	return fmt.Errorf("product not found")
}

func newMockProductRepo() *mockProductRepo {
	return &mockProductRepo{
		products: []model.Product{
			{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				Name:      "Laptop",
				Price:     99900,
				Category:  "Electronics",
				Stock:     10,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			{
				ID:        uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				Name:      "T-Shirt",
				Price:     2500,
				Category:  "Clothing",
				Stock:     50,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		},
	}
}

func TestListProducts(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	products, total, err := svc.List(context.Background(), model.ProductListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(products) != 2 {
		t.Errorf("expected 2 products, got %d", len(products))
	}
}

func TestListProductsByCategory(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	products, total, err := svc.List(context.Background(), model.ProductListParams{Category: "Electronics"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(products) != 1 {
		t.Errorf("expected 1 product, got %d", len(products))
	}
	if products[0].Name != "Laptop" {
		t.Errorf("expected Laptop, got %s", products[0].Name)
	}
}

func TestGetCategories(t *testing.T) {
	repo := newMockProductRepo()
	svc := service.NewProductService(repo, nil)

	cats, err := svc.Categories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d", len(cats))
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd go/product-service && go mod tidy && go test ./internal/service/ -v -race
```

Expected: 3 PASS

- [ ] **Step 4: Commit**

```bash
git add go/product-service/internal/service/
git commit -m "feat(product-service): add service layer with caching and unit tests"
```

---

### Task 5: Copy REST handler with tests

**Files:**
- Create: `go/product-service/internal/handler/product.go`
- Create: `go/product-service/internal/handler/product_test.go`
- Create: `go/product-service/internal/handler/health.go`

- [ ] **Step 1: Create handler/product.go**

Copy from `go/ecommerce-service/internal/handler/product.go` — update import paths:

```go
package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/pagination"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type ProductServiceInterface interface {
	List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Categories(ctx context.Context) ([]string, error)
}

type ProductHandler struct {
	svc ProductServiceInterface
}

func NewProductHandler(svc ProductServiceInterface) *ProductHandler {
	return &ProductHandler{svc: svc}
}

func (h *ProductHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	category := c.Query("category")
	query := c.Query("q")
	sort := c.DefaultQuery("sort", "created_at_desc")
	cursor := c.Query("cursor")

	if errs := validate.ProductListParams(sort, page, limit); len(errs) > 0 {
		_ = c.Error(apperror.Validation(errs))
		return
	}

	params := model.ProductListParams{
		Category: category,
		Query:    query,
		Sort:     sort,
		Page:     page,
		Limit:    limit,
		Cursor:   cursor,
	}

	products, total, err := h.svc.List(c.Request.Context(), params)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if cursor != "" {
		hasMore := len(products) > limit
		if hasMore {
			products = products[:limit]
		}
		var nextCursor string
		if hasMore && len(products) > 0 {
			nextCursor = buildProductCursor(products[len(products)-1], sort)
		}
		c.JSON(http.StatusOK, model.ProductListResponse{
			Products:   products,
			Limit:      limit,
			NextCursor: nextCursor,
			HasMore:    hasMore,
		})
		return
	}

	var nextCursor string
	hasMore := total > page*limit
	if hasMore && len(products) > 0 {
		nextCursor = buildProductCursor(products[len(products)-1], sort)
	}
	c.JSON(http.StatusOK, model.ProductListResponse{
		Products:   products,
		Total:      total,
		Page:       page,
		Limit:      limit,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

func buildProductCursor(p model.Product, sort string) string {
	switch sort {
	case "price_asc", "price_desc":
		return pagination.EncodeCursor(fmt.Sprintf("%d", p.Price), p.ID)
	case "name_asc":
		return pagination.EncodeCursor(p.Name, p.ID)
	default:
		return pagination.EncodeCursor(p.CreatedAt.Format(time.RFC3339Nano), p.ID)
	}
}

func (h *ProductHandler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_ID", "invalid product ID"))
		return
	}

	product, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}

	metrics.ProductViews.Inc()
	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) Categories(c *gin.Context) {
	categories, err := h.svc.Categories(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}
```

- [ ] **Step 2: Create handler/health.go**

```go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

func NewHealthHandler(pool *pgxpool.Pool, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{pool: pool, redis: redisClient}
}

func (h *HealthHandler) Health(c *gin.Context) {
	ctx := c.Request.Context()
	checks := gin.H{}
	if err := h.pool.Ping(ctx); err != nil {
		checks["postgres"] = "unhealthy"
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "checks": checks})
		return
	}
	checks["postgres"] = "healthy"
	if h.redis != nil {
		if err := h.redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unhealthy"
		} else {
			checks["redis"] = "healthy"
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "healthy", "checks": checks})
}
```

- [ ] **Step 3: Create handler/product_test.go**

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type fakeProductService struct {
	gotParams model.ProductListParams
}

func (f *fakeProductService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	f.gotParams = params
	return nil, 0, nil
}
func (f *fakeProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	return nil, nil
}
func (f *fakeProductService) Categories(ctx context.Context) ([]string, error) {
	return nil, nil
}

func productTestRouter() *gin.Engine {
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	return r
}

func TestProductHandler_List_ForwardsQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &fakeProductService{}
	h := NewProductHandler(svc)

	r := productTestRouter()
	r.GET("/products", h.List)

	req := httptest.NewRequest(http.MethodGet, "/products?q=jacket&limit=5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if svc.gotParams.Query != "jacket" {
		t.Errorf("expected Query=jacket, got %q", svc.gotParams.Query)
	}
	if svc.gotParams.Limit != 5 {
		t.Errorf("expected Limit=5, got %d", svc.gotParams.Limit)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if _, ok := body["products"]; !ok {
		t.Errorf("expected products key in body")
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd go/product-service && go mod tidy && go test ./internal/handler/ -v -race
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/product-service/internal/handler/
git commit -m "feat(product-service): add REST handlers with health check and tests"
```

---

### Task 6: Copy middleware (logging, metrics, CORS) and create routes

**Files:**
- Create: `go/product-service/internal/middleware/logging.go`
- Create: `go/product-service/internal/middleware/metrics.go`
- Create: `go/product-service/internal/middleware/cors.go`
- Create: `go/product-service/cmd/server/routes.go`

- [ ] **Step 1: Create middleware files**

Copy `logging.go`, `metrics.go`, and `cors.go` from `go/ecommerce-service/internal/middleware/` unchanged (no import path changes needed — they only use standard library and external packages).

`middleware/logging.go`:
```go
package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Set("requestId", requestID)
		c.Header("X-Request-ID", requestID)
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		attrs := []any{
			"requestId", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency", latency.String(),
			"ip", c.ClientIP(),
		}

		sc := trace.SpanContextFromContext(c.Request.Context())
		if sc.HasTraceID() {
			attrs = append(attrs, "traceID", sc.TraceID().String())
		}

		slog.InfoContext(c.Request.Context(), "request", attrs...)
	}
}
```

`middleware/metrics.go`:
```go
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}
```

`middleware/cors.go`:
```go
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func CORS(allowedOrigins string) gin.HandlerFunc {
	origins := strings.Split(allowedOrigins, ",")
	originSet := make(map[string]bool, len(origins))
	for _, o := range origins {
		originSet[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
```

- [ ] **Step 2: Create routes.go**

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

func setupRouter(
	cfg Config,
	productHandler *handler.ProductHandler,
	healthHandler *handler.HealthHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("product-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	router.GET("/products", productHandler.List)
	router.GET("/products/:id", productHandler.GetByID)
	router.GET("/categories", productHandler.Categories)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return router
}
```

- [ ] **Step 3: Run `go mod tidy`**

```bash
cd go/product-service && go mod tidy
```

- [ ] **Step 4: Commit**

```bash
git add go/product-service/internal/middleware/ go/product-service/cmd/server/routes.go
git commit -m "feat(product-service): add middleware and REST router setup"
```

---

### Task 7: Set up buf toolchain and proto definition

**Files:**
- Create: `go/buf.yaml`
- Create: `go/buf.gen.yaml`
- Create: `go/proto/product/v1/product.proto`

- [ ] **Step 1: Install buf locally (if not present)**

```bash
which buf || go install github.com/bufbuild/buf/cmd/buf@latest
```

- [ ] **Step 2: Create buf.yaml**

```yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
```

- [ ] **Step 3: Create buf.gen.yaml**

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: .
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: .
    opt: paths=source_relative
```

- [ ] **Step 4: Create product.proto**

```protobuf
syntax = "proto3";

package product.v1;

option go_package = "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1";

import "google/protobuf/timestamp.proto";

service ProductService {
  rpc GetProduct(GetProductRequest) returns (Product);
  rpc GetProducts(GetProductsRequest) returns (GetProductsResponse);
  rpc CheckAvailability(CheckAvailabilityRequest) returns (CheckAvailabilityResponse);
  rpc DecrementStock(DecrementStockRequest) returns (DecrementStockResponse);
  rpc InvalidateCache(InvalidateCacheRequest) returns (InvalidateCacheResponse);
}

message Product {
  string id = 1;
  string name = 2;
  string description = 3;
  int32 price = 4;
  string category = 5;
  string image_url = 6;
  int32 stock = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
}

message GetProductRequest {
  string id = 1;
}

message GetProductsRequest {
  string category = 1;
  string query = 2;
  string sort = 3;
  int32 page = 4;
  int32 limit = 5;
  string cursor = 6;
}

message GetProductsResponse {
  repeated Product products = 1;
  int32 total = 2;
  int32 page = 3;
  int32 limit = 4;
  string next_cursor = 5;
  bool has_more = 6;
}

message CheckAvailabilityRequest {
  string product_id = 1;
  int32 quantity = 2;
}

message CheckAvailabilityResponse {
  bool available = 1;
  int32 current_stock = 2;
  int32 price = 3;
}

message DecrementStockRequest {
  string product_id = 1;
  int32 quantity = 2;
}

message DecrementStockResponse {
  int32 remaining_stock = 1;
}

message InvalidateCacheRequest {}
message InvalidateCacheResponse {}
```

- [ ] **Step 5: Run buf lint**

```bash
cd go && buf lint
```

Expected: No errors

- [ ] **Step 6: Generate Go code**

```bash
cd go && buf generate
```

Expected: Generated files appear in `go/product-service/internal/pb/product/v1/`:
- `product.pb.go`
- `product_grpc.pb.go`

- [ ] **Step 7: Verify generated code compiles**

```bash
cd go/product-service && go mod tidy && go build ./internal/pb/...
```

- [ ] **Step 8: Commit**

```bash
git add go/buf.yaml go/buf.gen.yaml go/proto/ go/product-service/internal/pb/
git commit -m "feat(product-service): add buf toolchain and product.proto with generated Go code"
```

---

### Task 8: Implement gRPC server

**Files:**
- Create: `go/product-service/internal/grpc/server.go`
- Create: `go/product-service/internal/grpc/server_test.go`

- [ ] **Step 1: Write failing test**

Create `go/product-service/internal/grpc/server_test.go`:

```go
package grpc

import (
	"context"
	"testing"

	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

func TestGetProduct_NotFound(t *testing.T) {
	svc := &fakeService{}
	srv := NewProductGRPCServer(svc)

	_, err := srv.GetProduct(context.Background(), &pb.GetProductRequest{Id: "00000000-0000-0000-0000-000000000099"})
	if err == nil {
		t.Fatal("expected error for nonexistent product")
	}
}

func TestGetProducts_Empty(t *testing.T) {
	svc := &fakeService{}
	srv := NewProductGRPCServer(svc)

	resp, err := srv.GetProducts(context.Background(), &pb.GetProductsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Products) != 0 {
		t.Errorf("expected 0 products, got %d", len(resp.Products))
	}
}

func TestCheckAvailability_InStock(t *testing.T) {
	svc := &fakeService{
		stockProduct: &fakeProduct{stock: 10, price: 999},
	}
	srv := NewProductGRPCServer(svc)

	resp, err := srv.CheckAvailability(context.Background(), &pb.CheckAvailabilityRequest{
		ProductId: "00000000-0000-0000-0000-000000000001",
		Quantity:  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Available {
		t.Error("expected available=true")
	}
	if resp.CurrentStock != 10 {
		t.Errorf("expected stock 10, got %d", resp.CurrentStock)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd go/product-service && go test ./internal/grpc/ -v -race
```

Expected: FAIL (types not defined yet)

- [ ] **Step 3: Implement server.go**

```go
package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

type ProductServicer interface {
	List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	Categories(ctx context.Context) ([]string, error)
	InvalidateCache(ctx context.Context) error
}

type StockDecrementer interface {
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
}

type ProductGRPCServer struct {
	pb.UnimplementedProductServiceServer
	svc   ProductServicer
	stock StockDecrementer
}

func NewProductGRPCServer(svc ProductServicer, opts ...func(*ProductGRPCServer)) *ProductGRPCServer {
	s := &ProductGRPCServer{svc: svc}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithStockDecrementer(d StockDecrementer) func(*ProductGRPCServer) {
	return func(s *ProductGRPCServer) { s.stock = d }
}

func modelToProto(p *model.Product) *pb.Product {
	return &pb.Product{
		Id:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Price:       int32(p.Price),
		Category:    p.Category,
		ImageUrl:    p.ImageURL,
		Stock:       int32(p.Stock),
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
}

func (s *ProductGRPCServer) GetProduct(ctx context.Context, req *pb.GetProductRequest) (*pb.Product, error) {
	id, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
	}

	return modelToProto(product), nil
}

func (s *ProductGRPCServer) GetProducts(ctx context.Context, req *pb.GetProductsRequest) (*pb.GetProductsResponse, error) {
	params := model.ProductListParams{
		Category: req.Category,
		Query:    req.Query,
		Sort:     req.Sort,
		Page:     int(req.Page),
		Limit:    int(req.Limit),
		Cursor:   req.Cursor,
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Page <= 0 {
		params.Page = 1
	}

	products, total, err := s.svc.List(ctx, params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list products: %v", err)
	}

	resp := &pb.GetProductsResponse{
		Total:   int32(total),
		Page:    int32(params.Page),
		Limit:   int32(params.Limit),
		HasMore: len(products) > int(params.Limit),
	}

	for i := range products {
		resp.Products = append(resp.Products, modelToProto(&products[i]))
	}

	return resp, nil
}

func (s *ProductGRPCServer) CheckAvailability(ctx context.Context, req *pb.CheckAvailabilityRequest) (*pb.CheckAvailabilityResponse, error) {
	id, err := uuid.Parse(req.ProductId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "product not found: %v", err)
	}

	return &pb.CheckAvailabilityResponse{
		Available:    product.Stock >= int(req.Quantity),
		CurrentStock: int32(product.Stock),
		Price:        int32(product.Price),
	}, nil
}

func (s *ProductGRPCServer) DecrementStock(ctx context.Context, req *pb.DecrementStockRequest) (*pb.DecrementStockResponse, error) {
	if s.stock == nil {
		return nil, status.Errorf(codes.Unimplemented, "stock decrement not configured")
	}

	id, err := uuid.Parse(req.ProductId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid product ID: %v", err)
	}

	if err := s.stock.DecrementStock(ctx, id, int(req.Quantity)); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "decrement stock: %v", err)
	}

	product, err := s.svc.GetByID(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("fetch updated product: %v", err))
	}

	return &pb.DecrementStockResponse{
		RemainingStock: int32(product.Stock),
	}, nil
}

func (s *ProductGRPCServer) InvalidateCache(ctx context.Context, _ *pb.InvalidateCacheRequest) (*pb.InvalidateCacheResponse, error) {
	if err := s.svc.InvalidateCache(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "invalidate cache: %v", err)
	}
	return &pb.InvalidateCacheResponse{}, nil
}
```

- [ ] **Step 4: Add test helpers (fakes) to the test file**

Add to the top of `server_test.go`:

```go
package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/model"
	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

type fakeProduct struct {
	stock int
	price int
}

type fakeService struct {
	stockProduct *fakeProduct
}

func (f *fakeService) List(ctx context.Context, params model.ProductListParams) ([]model.Product, int, error) {
	return nil, 0, nil
}

func (f *fakeService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	if f.stockProduct != nil {
		return &model.Product{
			ID:    id,
			Name:  "Test Product",
			Stock: f.stockProduct.stock,
			Price: f.stockProduct.price,
		}, nil
	}
	return nil, fmt.Errorf("product not found")
}

func (f *fakeService) Categories(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakeService) InvalidateCache(ctx context.Context) error {
	return nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd go/product-service && go mod tidy && go test ./internal/grpc/ -v -race
```

Expected: 3 PASS

- [ ] **Step 6: Commit**

```bash
git add go/product-service/internal/grpc/
git commit -m "feat(product-service): implement gRPC server with product service methods"
```

---

### Task 9: Create main.go with dual REST + gRPC server

**Files:**
- Create: `go/product-service/cmd/server/main.go`

- [ ] **Step 1: Write main.go**

```go
package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	"github.com/kabradshaw1/portfolio/go/product-service/internal/handler"
	grpcsrv "github.com/kabradshaw1/portfolio/go/product-service/internal/grpc"
	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "product-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)
	defer pool.Close()

	redisClient := connectRedis(ctx, cfg.RedisURL)

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "product-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	productRepo := repository.NewProductRepository(pool, pgBreaker)
	productSvc := service.NewProductService(productRepo, redisClient)

	// REST server
	router := setupRouter(cfg,
		handler.NewProductHandler(productSvc),
		handler.NewHealthHandler(pool, redisClient),
	)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server failed: %v", err)
		}
	}()

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterProductServiceServer(grpcServer, grpcsrv.NewProductGRPCServer(
		productSvc,
		grpcsrv.WithStockDecrementer(productRepo),
	))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("product.v1.ProductService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen: %v", err)
	}

	go func() {
		slog.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down servers")

	cancel()
	grpcServer.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("REST server forced to shutdown: %v", err)
	}
	slog.Info("servers stopped")
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/product-service && go mod tidy && go build ./cmd/server/
```

Expected: Binary compiles without errors

- [ ] **Step 3: Commit**

```bash
git add go/product-service/cmd/server/main.go go/product-service/go.mod go/product-service/go.sum
git commit -m "feat(product-service): dual REST + gRPC server with OTel and health"
```

---

### Task 10: Add migrations, seed data, and Dockerfile

**Files:**
- Create: `go/product-service/migrations/001_create_products.up.sql`
- Create: `go/product-service/migrations/001_create_products.down.sql`
- Create: `go/product-service/migrations/002_add_pagination_indexes.up.sql`
- Create: `go/product-service/migrations/002_add_pagination_indexes.down.sql`
- Create: `go/product-service/seed.sql`
- Create: `go/product-service/Dockerfile`

- [ ] **Step 1: Create migrations**

`001_create_products.up.sql`:
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price INTEGER NOT NULL,
    category VARCHAR(100) NOT NULL,
    image_url VARCHAR(500),
    stock INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_category ON products (category);
CREATE INDEX idx_products_price ON products (price);
```

`001_create_products.down.sql`:
```sql
DROP TABLE IF EXISTS products;
```

`002_add_pagination_indexes.up.sql`:
```sql
CREATE INDEX IF NOT EXISTS idx_products_price_id ON products (price, id);
CREATE INDEX IF NOT EXISTS idx_products_name_id ON products (name, id);
CREATE INDEX IF NOT EXISTS idx_products_created_at_id ON products (created_at DESC, id DESC);
```

`002_add_pagination_indexes.down.sql`:
```sql
DROP INDEX IF EXISTS idx_products_price_id;
DROP INDEX IF EXISTS idx_products_name_id;
DROP INDEX IF EXISTS idx_products_created_at_id;
```

- [ ] **Step 2: Create seed.sql**

Copy product-related seed data from `go/ecommerce-service/seed.sql`. Keep the idempotent `WHERE NOT EXISTS` guards:

```sql
-- Product seed data for product-service
-- Must be idempotent: guard every INSERT with WHERE NOT EXISTS.

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440001', 'Wireless Headphones', 'Noise-cancelling Bluetooth headphones with 30-hour battery life.', 7999, 'Electronics', '', 50
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440001');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440002', 'USB-C Fast Charger', 'GaN 65W charger with dual ports.', 3499, 'Electronics', '', 120
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440002');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440003', 'Mechanical Keyboard', 'Cherry MX Brown switches with RGB backlighting.', 12999, 'Electronics', '', 35
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440003');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440004', 'Portable SSD 1TB', 'USB 3.2 Gen 2 external drive, 1050 MB/s reads.', 8999, 'Electronics', '', 60
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440004');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440005', 'Cotton T-Shirt', 'Soft-washed 100% cotton crew neck.', 2499, 'Clothing', '', 200
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440005');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440006', 'Slim Fit Chinos', 'Stretch-cotton twill chinos.', 4999, 'Clothing', '', 80
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440006');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440007', 'Waterproof Rain Jacket', 'Lightweight packable rain shell.', 6999, 'Clothing', '', 45
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440007');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440008', 'Knit Beanie', 'Merino-wool blend winter beanie.', 1999, 'Clothing', '', 150
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440008');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440009', 'Pour-Over Coffee Maker', 'Borosilicate glass dripper with reusable filter.', 3999, 'Home', '', 70
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440009');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440010', 'Cast Iron Skillet 12"', 'Pre-seasoned cast-iron pan.', 4499, 'Home', '', 40
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440010');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440011', 'LED Desk Lamp', 'Adjustable colour-temperature LED.', 5999, 'Home', '', 55
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440011');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440012', 'Ceramic Planters (Set of 3)', 'Matte-finish planters with drainage holes.', 2999, 'Home', '', 90
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440012');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440013', 'The Go Programming Language', 'Donovan & Kernighan.  Comprehensive Go reference.', 3499, 'Books', '', 100
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440013');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440014', 'Designing Data-Intensive Applications', 'Martin Kleppmann. Distributed-systems essentials.', 3999, 'Books', '', 85
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440014');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440015', 'Clean Architecture', 'Robert C. Martin.  Software design principles.', 2999, 'Books', '', 110
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440015');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440016', 'System Design Interview', 'Alex Xu. Step-by-step system design guide.', 3499, 'Books', '', 95
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440016');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440017', 'Yoga Mat', 'Non-slip TPE mat, 6mm thick.', 2999, 'Sports', '', 130
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440017');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440018', 'Adjustable Dumbbells (Pair)', 'Dial-adjustable 5-52.5 lb dumbbells.', 29999, 'Sports', '', 20
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440018');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440019', 'Resistance Bands Set', 'Five latex bands with handles and door anchor.', 1999, 'Sports', '', 160
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440019');

INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440020', 'Insulated Water Bottle', 'Stainless-steel, 32 oz, keeps drinks cold 24 h.', 2499, 'Sports', '', 140
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440020');

-- Smoke-test product: high stock, predictable name for E2E tests.
INSERT INTO products (id, name, description, price, category, image_url, stock)
SELECT '550e8400-e29b-41d4-a716-446655440099', 'Smoke Test Widget', 'Automated test fixture — do not remove.', 100, 'Electronics', '', 999999
WHERE NOT EXISTS (SELECT 1 FROM products WHERE id = '550e8400-e29b-41d4-a716-446655440099');
```

- [ ] **Step 3: Create Dockerfile**

```dockerfile
FROM migrate/migrate:v4.17.0 AS migrate

FROM golang:1.26-alpine AS builder

WORKDIR /app/product-service
COPY pkg/ /app/pkg/
COPY product-service/go.mod product-service/go.sum ./
RUN go mod download
COPY product-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /product-service ./cmd/server

FROM alpine:3.19

# hadolint ignore=DL3018
RUN apk add --no-cache postgresql-client \
    && adduser -D -u 1001 appuser

COPY --from=builder /product-service /product-service
COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY product-service/migrations/ /migrations/
COPY product-service/seed.sql /seed.sql

USER appuser

EXPOSE 8095 9095
ENTRYPOINT ["/product-service"]
```

- [ ] **Step 4: Commit**

```bash
git add go/product-service/migrations/ go/product-service/seed.sql go/product-service/Dockerfile
git commit -m "feat(product-service): add migrations, seed data, and Dockerfile"
```

---

### Task 11: Add Kubernetes manifests

**Files:**
- Create: `go/k8s/configmaps/product-service-config.yml`
- Create: `go/k8s/deployments/product-service.yml`
- Create: `go/k8s/services/product-service.yml`
- Create: `go/k8s/jobs/product-service-migrate.yml`
- Create: `go/k8s/hpa/product-hpa.yml`
- Create: `go/k8s/pdb/product-pdb.yml`
- Modify: `go/k8s/ingress.yml`
- Modify: `go/k8s/kustomization.yaml`

- [ ] **Step 1: Create configmap**

`go/k8s/configmaps/product-service-config.yml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: product-service-config
  namespace: go-ecommerce
data:
  DATABASE_URL: postgres://taskuser:taskpass@postgres.java-tasks.svc.cluster.local:5432/productdb?sslmode=disable
  REDIS_URL: redis://redis.java-tasks.svc.cluster.local:6379
  ALLOWED_ORIGINS: http://localhost:3000,https://kylebradshaw.dev
  PORT: "8095"
  GRPC_PORT: "9095"
  OTEL_EXPORTER_OTLP_ENDPOINT: "jaeger.monitoring.svc.cluster.local:4317"
```

- [ ] **Step 2: Create deployment**

`go/k8s/deployments/product-service.yml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-product-service
  namespace: go-ecommerce
spec:
  replicas: 1
  selector:
    matchLabels:
      app: go-product-service
  template:
    metadata:
      labels:
        app: go-product-service
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8095"
        prometheus.io/path: "/metrics"
    spec:
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: go-product-service
          image: ghcr.io/kabradshaw1/portfolio/go-product-service:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 8095
              name: http
            - containerPort: 9095
              name: grpc
          envFrom:
            - configMapRef:
                name: product-service-config
          securityContext:
            runAsNonRoot: true
            runAsUser: 1001
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          readinessProbe:
            httpGet:
              path: /health
              port: 8095
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /health
              port: 8095
            initialDelaySeconds: 15
            periodSeconds: 30
```

- [ ] **Step 3: Create service**

`go/k8s/services/product-service.yml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: go-product-service
  namespace: go-ecommerce
spec:
  selector:
    app: go-product-service
  ports:
    - name: http
      port: 8095
      targetPort: 8095
    - name: grpc
      port: 9095
      targetPort: 9095
```

- [ ] **Step 4: Create migration job**

`go/k8s/jobs/product-service-migrate.yml`:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: go-product-migrate
  namespace: go-ecommerce
spec:
  backoffLimit: 2
  activeDeadlineSeconds: 120
  ttlSecondsAfterFinished: 600
  template:
    spec:
      restartPolicy: Never
      imagePullSecrets:
        - name: ghcr-secret
      containers:
        - name: migrate-and-seed
          image: ghcr.io/kabradshaw1/portfolio/go-product-service:latest
          imagePullPolicy: Always
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e
              echo "Running product-service migrations..."
              /usr/local/bin/migrate -path=/migrations -database="${DATABASE_URL}&x-migrations-table=product_schema_migrations" up
              echo "Applying seed data..."
              psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f /seed.sql
              echo "Done."
          env:
            - name: DATABASE_URL
              valueFrom:
                configMapKeyRef:
                  name: product-service-config
                  key: DATABASE_URL
          resources:
            requests:
              memory: "32Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
```

- [ ] **Step 5: Create HPA and PDB**

`go/k8s/hpa/product-hpa.yml`:
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: go-product-hpa
  namespace: go-ecommerce
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-product-service
  minReplicas: 1
  maxReplicas: 3
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
```

`go/k8s/pdb/product-pdb.yml`:
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: go-product-pdb
  namespace: go-ecommerce
spec:
  maxUnavailable: 1
  selector:
    matchLabels:
      app: go-product-service
```

- [ ] **Step 6: Update ingress.yml — add product-service path**

Add this path before the existing `/go-api` path in `go/k8s/ingress.yml`:

```yaml
          - path: /go-products(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: go-product-service
                port:
                  number: 8095
```

- [ ] **Step 7: Update kustomization.yaml — add product-service resources**

Add these lines to the `resources:` list in `go/k8s/kustomization.yaml`:

```yaml
  - configmaps/product-service-config.yml
  - deployments/product-service.yml
  - services/product-service.yml
  - jobs/product-service-migrate.yml
  - hpa/product-hpa.yml
  - pdb/product-pdb.yml
```

- [ ] **Step 8: Commit**

```bash
git add go/k8s/
git commit -m "feat(product-service): add Kubernetes manifests with gRPC port"
```

---

### Task 12: Update CI pipeline

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `Makefile`

- [ ] **Step 1: Add product-service to Go lint and test matrices**

In `.github/workflows/ci.yml`, update the `go-lint` and `go-tests` jobs' matrix:

```yaml
        service: [auth-service, ecommerce-service, ai-service, analytics-service, product-service]
```

- [ ] **Step 2: Add product-service to Docker build matrix**

Add this entry to the `build-images` job's `matrix.include`:

```yaml
          - service: go-product-service
            context: go
            file: go/product-service/Dockerfile
            image: go-product-service
            paths: go/product-service go/pkg
```

- [ ] **Step 3: Add productdb creation and product-service migrations to migration test**

In the `go-migration-test` job, after the "Create ecommercedb" step, add:

```yaml
      - name: Create productdb
        env:
          PGPASSWORD: taskpass
        run: |
          psql -h localhost -U taskuser -d taskdb -c "CREATE DATABASE productdb;"

      - name: Run product-service migrations
        run: |
          migrate -path go/product-service/migrations -database "postgres://taskuser:taskpass@localhost:5432/productdb?sslmode=disable&x-migrations-table=product_schema_migrations" up

      - name: Apply product seed data
        run: |
          psql "postgres://taskuser:taskpass@localhost:5432/productdb?sslmode=disable" -v ON_ERROR_STOP=1 -f go/product-service/seed.sql
```

- [ ] **Step 4: Update Makefile preflight-go target**

Add product-service to lint and test commands in `Makefile`:

```makefile
preflight-go:
	@echo "\n=== Go: linting ==="
	cd go/auth-service && golangci-lint run ./...
	cd go/ecommerce-service && golangci-lint run ./...
	cd go/ai-service && golangci-lint run ./...
	cd go/product-service && golangci-lint run ./...
	@echo "\n=== Go: tests ==="
	cd go/auth-service && go test ./... -v -race
	cd go/ecommerce-service && go test ./... -v -race
	cd go/ai-service && go test ./... -v -race
	cd go/product-service && go test ./... -v -race
```

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml Makefile
git commit -m "ci: add product-service to lint, test, build, and migration pipeline"
```

---

### Task 13: Update ecommerce-service order worker to use gRPC

**Files:**
- Modify: `go/ecommerce-service/internal/worker/order_processor.go`
- Modify: `go/ecommerce-service/cmd/server/main.go`
- Modify: `go/ecommerce-service/cmd/server/config.go`

- [ ] **Step 1: Add PRODUCT_GRPC_ADDR to ecommerce config**

Add to `go/ecommerce-service/cmd/server/config.go`:

```go
type Config struct {
	// ... existing fields ...
	ProductGRPCAddr string // address of product-service gRPC
}
```

And in `loadConfig()`:
```go
cfg.ProductGRPCAddr = os.Getenv("PRODUCT_GRPC_ADDR")
```

- [ ] **Step 2: Update order_processor.go interfaces**

Replace the `ProductRepoForWorker` and `CacheInvalidator` interfaces with a single gRPC-backed interface. Update `go/ecommerce-service/internal/worker/order_processor.go`:

Replace `ProductRepoForWorker` and `CacheInvalidator` with:

```go
type ProductClient interface {
	DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error
	InvalidateCache(ctx context.Context) error
}
```

Update `OrderProcessor` struct:

```go
type OrderProcessor struct {
	orderRepo      OrderRepoForWorker
	productClient  ProductClient
	kafkaPublisher kafka.Producer
}

func NewOrderProcessor(orderRepo OrderRepoForWorker, productClient ProductClient, kafkaPub kafka.Producer) *OrderProcessor {
	return &OrderProcessor{
		orderRepo:      orderRepo,
		productClient:  productClient,
		kafkaPublisher: kafkaPub,
	}
}
```

Update `ProcessOrder` to use `productClient`:

```go
func (p *OrderProcessor) ProcessOrder(ctx context.Context, orderIDStr string) error {
	// ... parse orderID, find order, set processing (unchanged) ...

	for _, item := range order.Items {
		if err := p.productClient.DecrementStock(ctx, item.ProductID, item.Quantity); err != nil {
			_ = p.orderRepo.UpdateStatus(ctx, orderID, model.OrderStatusFailed)
			ordersTotal.WithLabelValues("failed").Inc()
			kafka.SafePublish(ctx, p.kafkaPublisher, "ecommerce.orders", orderIDStr, kafka.Event{
				Type: "order.failed",
				Data: map[string]any{"orderID": orderIDStr, "userID": order.UserID.String()},
			})
			return fmt.Errorf("decrement stock for product %s: %w", item.ProductID, err)
		}
	}

	// ... set completed, invalidate cache, publish kafka (unchanged) ...
	if err := p.productClient.InvalidateCache(ctx); err != nil {
		log.Printf("WARN: failed to invalidate cache: %v", err)
	}
	// ... rest unchanged ...
}
```

- [ ] **Step 3: Create gRPC client adapter in ecommerce-service**

First, add a replace directive to `go/ecommerce-service/go.mod` so it can import the product-service's generated protobuf code:

```
require github.com/kabradshaw1/portfolio/go/product-service v0.0.0
replace github.com/kabradshaw1/portfolio/go/product-service => ../product-service
```

Then create `go/ecommerce-service/internal/productclient/client.go`:

```go
package productclient

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/kabradshaw1/portfolio/go/product-service/internal/pb/product/v1"
)

type GRPCClient struct {
	client pb.ProductServiceClient
	conn   *grpc.ClientConn
}

func New(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to product-service: %w", err)
	}
	return &GRPCClient{
		client: pb.NewProductServiceClient(conn),
		conn:   conn,
	}, nil
}

func (c *GRPCClient) DecrementStock(ctx context.Context, productID uuid.UUID, qty int) error {
	_, err := c.client.DecrementStock(ctx, &pb.DecrementStockRequest{
		ProductId: productID.String(),
		Quantity:  int32(qty),
	})
	return err
}

func (c *GRPCClient) InvalidateCache(ctx context.Context) error {
	_, err := c.client.InvalidateCache(ctx, &pb.InvalidateCacheRequest{})
	return err
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
```

- [ ] **Step 4: Update ecommerce main.go**

In `go/ecommerce-service/cmd/server/main.go`, replace the product repo/service usage in the order processor:

Remove:
```go
productRepo := repository.NewProductRepository(pool, pgBreaker)
productSvc := service.NewProductService(productRepo, redisClient)
processor := worker.NewOrderProcessor(orderRepo, productRepo, productSvc, kafkaPub)
```

Replace with:
```go
var prodClient *productclient.GRPCClient
if cfg.ProductGRPCAddr != "" {
	var err error
	prodClient, err = productclient.New(cfg.ProductGRPCAddr)
	if err != nil {
		log.Fatalf("product gRPC client: %v", err)
	}
	defer prodClient.Close()
	slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
}
processor := worker.NewOrderProcessor(orderRepo, prodClient, kafkaPub)
```

Also remove the product handler/routes from ecommerce-service (product routes, product handler creation, product service creation). Remove product-related imports.

- [ ] **Step 5: Update ecommerce configmap to include PRODUCT_GRPC_ADDR**

Add to `go/k8s/configmaps/ecommerce-service-config.yml`:

```yaml
  PRODUCT_GRPC_ADDR: go-product-service.go-ecommerce.svc.cluster.local:9095
```

- [ ] **Step 6: Remove product routes from ecommerce routes.go**

Remove these lines from `go/ecommerce-service/cmd/server/routes.go`:
```go
router.GET("/products", productHandler.List)
router.GET("/products/:id", productHandler.GetByID)
router.GET("/categories", productHandler.Categories)
```

And remove `productHandler *handler.ProductHandler` from `setupRouter` parameters.

- [ ] **Step 7: Run ecommerce tests to verify nothing breaks**

```bash
cd go/ecommerce-service && go mod tidy && go test ./... -v -race
```

Fix any compilation errors from removed imports.

- [ ] **Step 8: Commit**

```bash
git add go/ecommerce-service/
git commit -m "refactor(ecommerce): replace direct product repo with gRPC client to product-service"
```

---

### Task 14: Update frontend to point to product-service

**Files:**
- Modify: `frontend/src/lib/go-auth.ts`
- Modify: `frontend/src/app/go/ecommerce/page.tsx`
- Modify: `frontend/src/app/go/ecommerce/[productId]/page.tsx`

- [ ] **Step 1: Add GO_PRODUCT_URL constant**

In `frontend/src/lib/go-auth.ts`, add:

```typescript
export const GO_PRODUCT_URL =
  process.env.NEXT_PUBLIC_GO_PRODUCT_URL || "http://localhost:8095";
```

- [ ] **Step 2: Update product listing page**

In `frontend/src/app/go/ecommerce/page.tsx`, change the import and fetch URL:

Replace:
```typescript
import { GO_ECOMMERCE_URL } from "@/lib/go-auth";
```
With:
```typescript
import { GO_PRODUCT_URL } from "@/lib/go-auth";
```

Replace:
```typescript
fetch(`${GO_ECOMMERCE_URL}/products`)
```
With:
```typescript
fetch(`${GO_PRODUCT_URL}/products`)
```

- [ ] **Step 3: Update product detail page**

In `frontend/src/app/go/ecommerce/[productId]/page.tsx`, apply the same change — replace `GO_ECOMMERCE_URL` with `GO_PRODUCT_URL` for the product fetch call. Keep `GO_ECOMMERCE_URL` for any cart/order calls on the same page.

- [ ] **Step 4: Add Vercel env var (reminder)**

Before merging, add to Vercel production:
```
NEXT_PUBLIC_GO_PRODUCT_URL=https://api.kylebradshaw.dev/go-products
```

- [ ] **Step 5: Run frontend checks**

```bash
cd frontend && npx tsc --noEmit && npx next lint
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/go-auth.ts frontend/src/app/go/ecommerce/
git commit -m "feat(frontend): point product API calls to new product-service"
```

---

### Task 15: Update postgres-initdb to create productdb

**Files:**
- Determine: the postgres-initdb ConfigMap location (likely in `k8s/` or the Java namespace since postgres lives in `java-tasks`)

- [ ] **Step 1: Find and update the initdb ConfigMap**

Search for the postgres-initdb ConfigMap that creates `ecommercedb`. Add `productdb` creation alongside it:

```sql
CREATE DATABASE productdb;
```

This only runs on first boot of a fresh PVC, so existing clusters need manual database creation:

```bash
ssh debian "kubectl exec -n java-tasks deploy/postgres -- psql -U taskuser -d taskdb -c 'CREATE DATABASE productdb;'"
```

- [ ] **Step 2: Commit**

```bash
git add <path-to-initdb-configmap>
git commit -m "infra: add productdb to postgres-initdb ConfigMap"
```

---

### Task 16: Run preflight and verify

- [ ] **Step 1: Run full Go preflight**

```bash
make preflight-go
```

Expected: All lint and tests pass for all 5 Go services.

- [ ] **Step 2: Run frontend preflight**

```bash
make preflight-frontend
```

Expected: TypeScript and lint checks pass.

- [ ] **Step 3: Run security preflight**

```bash
make preflight-security
```

- [ ] **Step 4: Verify Dockerfile builds locally**

```bash
cd go && docker build -f product-service/Dockerfile -t product-service:test .
```

Expected: Image builds successfully.

- [ ] **Step 5: Verify buf generates cleanly**

```bash
cd go && buf lint && buf generate
```

Expected: No lint errors, generated code matches committed code.
