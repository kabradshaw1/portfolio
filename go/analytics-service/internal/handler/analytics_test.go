package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
)

type stubConsumer struct {
	connected bool
}

func (s *stubConsumer) Connected() bool { return s.connected }

func setupRouter(h *AnalyticsHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/analytics/dashboard", h.Dashboard)
	r.GET("/analytics/trending", h.Trending)
	r.GET("/analytics/orders", h.Orders)
	r.GET("/health", h.Health)
	return r
}

func TestDashboard_Empty(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	// nil consumer — will report stale
	h := &AnalyticsHandler{orders: orders, trending: trending, carts: carts, consumer: &stubConsumer{}}
	r := setupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/dashboard", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := body["ordersPerHour"]; !ok {
		t.Error("expected ordersPerHour in response")
	}
	if _, ok := body["stale"]; !ok {
		t.Error("expected stale flag in response")
	}
}

func TestTrending_WithData(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	trending.RecordView("p1", "Widget")
	trending.RecordView("p1", "Widget")

	h := &AnalyticsHandler{orders: orders, trending: trending, carts: carts, consumer: &stubConsumer{connected: true}}
	r := setupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/analytics/trending", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	products := body["products"].([]any)
	if len(products) != 1 {
		t.Errorf("expected 1 product, got %d", len(products))
	}
}

func TestHealth(t *testing.T) {
	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	h := &AnalyticsHandler{orders: orders, trending: trending, carts: carts, consumer: &stubConsumer{connected: true}}
	r := setupRouter(h)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["kafka"] != "connected" {
		t.Errorf("expected kafka connected, got %v", body["kafka"])
	}
}
