package guardrails

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/redis/go-redis/v9"
)

func newRedisLimiter(t *testing.T, max int, window time.Duration) (*Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return NewLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}), max, window, resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})), mr
}

func TestLimiter_AllowsUpToMax(t *testing.T) {
	l, _ := newRedisLimiter(t, 3, time.Minute)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ok, _, err := l.Allow(ctx, "ip-1")
		if err != nil || !ok {
			t.Fatalf("call %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, retry, err := l.Allow(ctx, "ip-1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Error("expected block on 4th call")
	}
	if retry <= 0 {
		t.Errorf("expected positive retryAfter, got %v", retry)
	}
}

func TestLimiter_WindowReset(t *testing.T) {
	l, mr := newRedisLimiter(t, 2, time.Second)
	ctx := context.Background()
	_, _, _ = l.Allow(ctx, "ip")
	_, _, _ = l.Allow(ctx, "ip")
	ok, _, _ := l.Allow(ctx, "ip")
	if ok {
		t.Fatal("expected block before reset")
	}
	mr.FastForward(2 * time.Second)
	ok, _, _ = l.Allow(ctx, "ip")
	if !ok {
		t.Error("expected allow after window reset")
	}
}

func TestMiddleware_NilLimiterPassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(nil))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: code=%d", i, w.Code)
		}
	}
}

func TestMiddleware_Blocks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l, _ := newRedisLimiter(t, 1, time.Minute)
	r := gin.New()
	r.Use(Middleware(l))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w1 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	r.ServeHTTP(w1, req)
	if w1.Code != http.StatusOK {
		t.Fatalf("first call: %d", w1.Code)
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.RemoteAddr = "1.2.3.4:1234"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}
