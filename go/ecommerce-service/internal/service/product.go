package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/metrics"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/model"
)

// getFromCache attempts to read and unmarshal a value from Redis.
// Returns (value, true) on cache hit, (zero, false) on miss or error.
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

// setInCache marshals and stores a value in Redis with the given TTL.
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
	// Cursor values are unique per request; caching would be useless and wasteful.
	if params.Cursor != "" {
		return s.repo.List(ctx, params)
	}

	cacheKey := fmt.Sprintf("ecom:products:list:%s:%s:%d:%d", params.Category, params.Sort, params.Page, params.Limit)

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
	cacheKey := fmt.Sprintf("ecom:product:%s", id.String())

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
	cacheKey := "ecom:categories"

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
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "ecom:products:*", 100).Result()
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

	s.redis.Del(ctx, "ecom:categories")
	return nil
}
