package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type stubConsumer struct {
	connected bool
}

func (s *stubConsumer) Connected() bool { return s.connected }

// stubStore wraps MockStore with pre-seeded data for handler tests.
type stubStore struct {
	*store.MockStore
}

func newSeededStore() *stubStore {
	s := store.NewMockStore()
	ctx := context.Background()

	// Seed revenue data.
	now := time.Now().UTC().Truncate(time.Hour)
	key := now.Format("2006-01-02T15")
	_ = s.FlushRevenue(ctx, key, 10000, 5)

	// Seed trending data.
	_ = s.FlushTrending(ctx, key, map[string]float64{
		"prod-1": 10.0,
		"prod-2": 5.0,
	})

	// Seed abandonment data.
	_ = s.FlushAbandonment(ctx, key, 20, 15)

	return &stubStore{MockStore: s}
}

func setupTestRouter(h *AnalyticsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.GET("/analytics/revenue", h.Revenue)
	r.GET("/analytics/trending", h.Trending)
	r.GET("/analytics/cart-abandonment", h.CartAbandonment)
	r.GET("/health", h.Health)
	return r
}

func TestRevenue_DefaultParams(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/revenue", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	windows, ok := body["windows"].([]any)
	require.True(t, ok, "windows should be an array")
	assert.NotEmpty(t, windows)
	assert.Equal(t, false, body["stale"])
}

func TestRevenue_CustomHours(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/revenue?hours=1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotNil(t, body["windows"])
}

func TestRevenue_MaxClamping(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/revenue?hours=100", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRevenue_EmptyStore(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: false})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/revenue", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	windows, ok := body["windows"].([]any)
	require.True(t, ok, "windows should be an array, not null")
	assert.Empty(t, windows)
	assert.Equal(t, true, body["stale"])
}

func TestTrending_DefaultLimit(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/trending", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	products, ok := body["products"].([]any)
	require.True(t, ok, "products should be an array")
	assert.Len(t, products, 2)
	assert.NotEmpty(t, body["window_end"])
	assert.Equal(t, false, body["stale"])
}

func TestTrending_WithLimit(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/trending?limit=1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	products, ok := body["products"].([]any)
	require.True(t, ok)
	assert.Len(t, products, 1)
}

func TestTrending_MaxClamping(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/trending?limit=100", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTrending_EmptyStore(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/trending", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	products, ok := body["products"].([]any)
	require.True(t, ok, "products should be an array, not null")
	assert.Empty(t, products)
	assert.Equal(t, "", body["window_end"])
}

func TestCartAbandonment_DefaultParams(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/cart-abandonment", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	windows, ok := body["windows"].([]any)
	require.True(t, ok, "windows should be an array")
	assert.NotEmpty(t, windows)
	assert.Equal(t, false, body["stale"])
}

func TestCartAbandonment_CustomHours(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/cart-abandonment?hours=6", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCartAbandonment_MaxClamping(t *testing.T) {
	s := newSeededStore()
	h := NewAnalyticsHandler(s.MockStore, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/cart-abandonment?hours=100", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCartAbandonment_EmptyStore(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: false})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/cart-abandonment", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	windows, ok := body["windows"].([]any)
	require.True(t, ok, "windows should be an array, not null")
	assert.Empty(t, windows)
}

func TestHealth_Connected(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "connected", body["kafka"])
	assert.Equal(t, "ok", body["status"])
}

func TestHealth_Disconnected(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: false})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "disconnected", body["kafka"])
}

func TestParseIntParam_InvalidValue(t *testing.T) {
	s := store.NewMockStore()
	h := NewAnalyticsHandler(s, &stubConsumer{connected: true})
	r := setupTestRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/revenue?hours=abc", nil)
	r.ServeHTTP(w, req)

	// Should use default, not error.
	assert.Equal(t, http.StatusOK, w.Code)
}
