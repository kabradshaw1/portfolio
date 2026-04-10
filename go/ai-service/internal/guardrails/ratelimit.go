package guardrails

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	gobreaker "github.com/sony/gobreaker/v2"
)

// Limiter is a Redis fixed-window limiter keyed by IP.
// Fixed window (INCR + EXPIRE) keeps the logic trivial and gives a clear
// "N per window" rule. Not strictly as smooth as a token bucket, but good enough.
type Limiter struct {
	client  *redis.Client
	breaker *gobreaker.CircuitBreaker[any]
	prefix  string
	max     int
	window  time.Duration
}

func NewLimiter(client *redis.Client, max int, window time.Duration, breaker *gobreaker.CircuitBreaker[any]) *Limiter {
	return &Limiter{client: client, prefix: "ai:ratelimit", max: max, window: window, breaker: breaker}
}

// Allow returns (allowed, retryAfter, error). retryAfter is 0 on allow.
func (l *Limiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	type result struct {
		ok    bool
		retry time.Duration
	}
	res, err := l.breaker.Execute(func() (any, error) {
		k := l.prefix + ":" + key
		n, err := l.client.Incr(ctx, k).Result()
		if err != nil {
			return nil, err
		}
		if n == 1 {
			if err := l.client.Expire(ctx, k, l.window).Err(); err != nil {
				return nil, err
			}
		}
		if int(n) > l.max {
			ttl, _ := l.client.TTL(ctx, k).Result()
			if ttl < 0 {
				ttl = l.window
			}
			return result{ok: false, retry: ttl}, nil
		}
		return result{ok: true}, nil
	})
	if err != nil {
		// Fail open on breaker-open or Redis errors.
		return true, 0, nil
	}
	r := res.(result)
	return r.ok, r.retry, nil
}

// Middleware returns Gin middleware that applies the limiter. If l is nil,
// it's a no-op — callers wire it conditionally based on Redis availability.
func Middleware(l *Limiter) gin.HandlerFunc {
	if l == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		ok, retry, err := l.Allow(c.Request.Context(), c.ClientIP())
		if err != nil {
			// Fail open on Redis errors — outages must not disable the service.
			c.Next()
			return
		}
		if !ok {
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
