//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
)

// TestRateLimiter_BlocksAfterThreshold creates a rate limiter with max=5 over
// a 1-minute window, sends 5 requests (all must be 200), and verifies the 6th
// is rejected with 429 and a Retry-After header.
func TestRateLimiter_BlocksAfterThreshold(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()

	// Use a test-scoped prefix so keys don't bleed across parallel test runs.
	prefix := "test:ratelimit:" + t.Name()

	// Clean up any leftover keys from a previous run.
	keys, err := infra.RedisClient.Keys(ctx, prefix+":*").Result()
	if err == nil && len(keys) > 0 {
		infra.RedisClient.Del(ctx, keys...)
	}

	const max = 5

	rl := middleware.NewRateLimiter(infra.RedisClient, prefix, max, time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", rl.Middleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// First `max` requests must all succeed.
	for i := 1; i <= max; i++ {
		w := testutil.DoRequest(t, r, http.MethodGet, "/test", "", nil)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d (body=%s)", i, w.Code, w.Body.String())
		}
	}

	// The (max+1)th request must be rejected.
	w := testutil.DoRequest(t, r, http.MethodGet, "/test", "", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("6th request: expected 429, got %d (body=%s)", w.Code, w.Body.String())
	}

	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("6th request: expected Retry-After header, got none")
	}
}
