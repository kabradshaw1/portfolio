//go:build integration

package integration

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/integration/testutil"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// setupIdempotencyTestRouter constructs a test router that increments counter
// on every handler invocation and returns {"call": N} in the body.
func setupIdempotencyTestRouter(t *testing.T, counter *atomic.Int32, userID string) *gin.Engine {
	t.Helper()
	infra := getInfra(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())

	r.POST("/test",
		func(c *gin.Context) { c.Set("userId", userID); c.Next() },
		middleware.Idempotency(infra.RedisClient, true),
		func(c *gin.Context) {
			n := counter.Add(1)
			c.JSON(http.StatusCreated, gin.H{"call": n})
		},
	)
	return r
}

// TestIdempotency_SameKeyReturnsCachedResponse verifies that sending the same
// Idempotency-Key twice results in the handler being called only once and both
// responses being identical.
func TestIdempotency_SameKeyReturnsCachedResponse(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()

	// Flush all idempotency keys so previous test runs don't bleed through.
	if err := infra.RedisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("FlushDB: %v", err)
	}

	userID := uuid.New().String()
	idempotencyKey := uuid.New().String()

	var counter atomic.Int32
	router := setupIdempotencyTestRouter(t, &counter, userID)

	headers := map[string]string{"Idempotency-Key": idempotencyKey}

	// First request — handler must execute.
	w1 := testutil.DoRequest(t, router, http.MethodPost, "/test", "", headers)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d (body=%s)", w1.Code, w1.Body.String())
	}

	// Second request with the same key — handler must NOT execute again.
	w2 := testutil.DoRequest(t, router, http.MethodPost, "/test", "", headers)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second request: expected 201, got %d (body=%s)", w2.Code, w2.Body.String())
	}

	// Handler should have been called exactly once.
	if calls := counter.Load(); calls != 1 {
		t.Errorf("expected handler called 1 time, got %d", calls)
	}

	// Both responses must be identical.
	if w1.Body.String() != w2.Body.String() {
		t.Errorf("responses differ:\n  first:  %s\n  second: %s", w1.Body.String(), w2.Body.String())
	}
}

// TestIdempotency_DifferentKeyCreatesSeparateResource verifies that two requests
// with different idempotency keys each trigger a handler invocation and return
// different responses.
func TestIdempotency_DifferentKeyCreatesSeparateResource(t *testing.T) {
	infra := getInfra(t)
	ctx := context.Background()

	if err := infra.RedisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("FlushDB: %v", err)
	}

	userID := uuid.New().String()
	key1 := uuid.New().String()
	key2 := uuid.New().String()

	var counter atomic.Int32
	router := setupIdempotencyTestRouter(t, &counter, userID)

	w1 := testutil.DoRequest(t, router, http.MethodPost, "/test", "", map[string]string{"Idempotency-Key": key1})
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request: expected 201, got %d (body=%s)", w1.Code, w1.Body.String())
	}

	w2 := testutil.DoRequest(t, router, http.MethodPost, "/test", "", map[string]string{"Idempotency-Key": key2})
	if w2.Code != http.StatusCreated {
		t.Fatalf("second request: expected 201, got %d (body=%s)", w2.Code, w2.Body.String())
	}

	// Handler must have been called twice.
	if calls := counter.Load(); calls != 2 {
		t.Errorf("expected handler called 2 times, got %d", calls)
	}

	// Responses must differ (different call counters).
	if w1.Body.String() == w2.Body.String() {
		t.Errorf("expected different responses for different keys, got identical: %s", w1.Body.String())
	}
}
