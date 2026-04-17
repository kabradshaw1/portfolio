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

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var resp model.ProductListResponse
			if json.Unmarshal([]byte(cached), &resp) == nil {
				metrics.CacheOps.WithLabelValues("get", "hit").Inc()
				return resp.Products, resp.Total, nil
			}
		}
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
	}

	products, total, err := s.repo.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	if s.redis != nil {
		resp := model.ProductListResponse{Products: products, Total: total, Page: params.Page, Limit: params.Limit}
		if data, err := json.Marshal(resp); err == nil {
			s.redis.Set(ctx, cacheKey, data, 5*time.Minute)
			metrics.CacheOps.WithLabelValues("set", "success").Inc()
		}
	}

	return products, total, nil
}

func (s *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	cacheKey := fmt.Sprintf("ecom:product:%s", id.String())

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var p model.Product
			if json.Unmarshal([]byte(cached), &p) == nil {
				metrics.CacheOps.WithLabelValues("get", "hit").Inc()
				return &p, nil
			}
		}
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
	}

	product, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		if data, err := json.Marshal(product); err == nil {
			s.redis.Set(ctx, cacheKey, data, 5*time.Minute)
			metrics.CacheOps.WithLabelValues("set", "success").Inc()
		}
	}

	return product, nil
}

func (s *ProductService) Categories(ctx context.Context) ([]string, error) {
	cacheKey := "ecom:categories"

	if s.redis != nil {
		cached, err := s.redis.Get(ctx, cacheKey).Result()
		if err == nil {
			var cats []string
			if json.Unmarshal([]byte(cached), &cats) == nil {
				metrics.CacheOps.WithLabelValues("get", "hit").Inc()
				return cats, nil
			}
		}
		metrics.CacheOps.WithLabelValues("get", "miss").Inc()
	}

	cats, err := s.repo.Categories(ctx)
	if err != nil {
		return nil, err
	}

	if s.redis != nil {
		if data, err := json.Marshal(cats); err == nil {
			s.redis.Set(ctx, cacheKey, data, 30*time.Minute)
			metrics.CacheOps.WithLabelValues("set", "success").Inc()
		}
	}

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
