package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/stretchr/testify/assert"
)

func setupIdempotencyRouter(required bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.POST("/test",
		func(c *gin.Context) { c.Set("userId", uuid.New().String()); c.Next() },
		middleware.Idempotency(nil, required),
		func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{"id": "test"}) },
	)
	return r
}

// TestIdempotency_NilRedis_PassesThrough verifies that when Redis is nil the
// middleware fails open: the downstream handler executes and returns 201.
func TestIdempotency_NilRedis_PassesThrough(t *testing.T) {
	r := setupIdempotencyRouter(true)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// TestIdempotency_RequiredButMissing_Returns400 verifies that a missing header
// when required=true results in a 400 with code MISSING_IDEMPOTENCY_KEY.
func TestIdempotency_RequiredButMissing_Returns400(t *testing.T) {
	r := setupIdempotencyRouter(true)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "MISSING_IDEMPOTENCY_KEY")
}

// TestIdempotency_OptionalAndMissing_PassesThrough verifies that a missing
// header when required=false allows the request through (handler runs, 201).
func TestIdempotency_OptionalAndMissing_PassesThrough(t *testing.T) {
	r := setupIdempotencyRouter(false)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// TestIdempotency_InvalidUUID_Returns400 verifies that a non-UUID header value
// results in a 400 with code INVALID_IDEMPOTENCY_KEY.
func TestIdempotency_InvalidUUID_Returns400(t *testing.T) {
	r := setupIdempotencyRouter(true)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("Idempotency-Key", "not-a-uuid")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "INVALID_IDEMPOTENCY_KEY")
}
