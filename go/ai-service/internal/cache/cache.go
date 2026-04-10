package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	gobreaker "github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

// Cache is a tiny key/value interface. Bytes in, bytes out.
// Callers handle their own serialization so the cache stays transport-agnostic.
type Cache interface {
	Get(ctx context.Context, key string) (value []byte, ok bool, err error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// NopCache is a zero-cost no-op. Used when Redis is unavailable or disabled.
type NopCache struct{}

func (NopCache) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (NopCache) Set(context.Context, string, []byte, time.Duration) error { return nil }

// RedisCache is a Redis-backed Cache. All keys are automatically prefixed.
// If a circuit breaker is provided, Redis failures fail open (cache miss / skip write).
type RedisCache struct {
	client  *redis.Client
	prefix  string
	breaker *gobreaker.CircuitBreaker[any]
}

func NewRedisCache(client *redis.Client, prefix string, breaker *gobreaker.CircuitBreaker[any]) *RedisCache {
	return &RedisCache{client: client, prefix: prefix, breaker: breaker}
}

func (c *RedisCache) key(k string) string { return c.prefix + ":" + k }

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	ctx, span := tracing.RedisSpan(ctx, "GET", c.key(key))
	defer span.End()
	result, err := c.breaker.Execute(func() (any, error) {
		val, err := c.client.Get(ctx, c.key(key)).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil, nil // cache miss, not an error
		}
		if err != nil {
			return nil, err
		}
		return val, nil
	})
	if err != nil {
		// Fail open: breaker open or Redis error → treat as cache miss.
		return nil, false, nil
	}
	if result == nil {
		return nil, false, nil
	}
	return result.([]byte), true, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	ctx, span := tracing.RedisSpan(ctx, "SET", c.key(key))
	defer span.End()
	_, err := c.breaker.Execute(func() (any, error) {
		return nil, c.client.Set(ctx, c.key(key), value, ttl).Err()
	})
	if err != nil {
		// Fail open: skip write silently.
		return nil
	}
	return nil
}
