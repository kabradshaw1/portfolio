package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client *redis.Client
	prefix string
	max    int
	window time.Duration
}

func NewRateLimiter(client *redis.Client, prefix string, max int, window time.Duration) *RateLimiter {
	return &RateLimiter{client: client, prefix: prefix, max: max, window: window}
}

func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	if rl == nil || rl.client == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		key := rl.prefix + ":" + c.ClientIP()
		n, err := rl.client.Incr(c.Request.Context(), key).Result()
		if err != nil {
			c.Next()
			return
		}
		if n == 1 {
			rl.client.Expire(c.Request.Context(), key, rl.window)
		}
		if int(n) > rl.max {
			ttl, _ := rl.client.TTL(c.Request.Context(), key).Result()
			if ttl < 0 {
				ttl = rl.window
			}
			c.Header("Retry-After", strconv.Itoa(int(ttl.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": map[string]string{"code": "RATE_LIMITED", "message": "too many requests"},
			})
			return
		}
		c.Next()
	}
}
