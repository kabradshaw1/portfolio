package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/store"
)

const (
	defaultRevenueHours     = 24
	maxRevenueHours         = 48
	defaultTrendingLimit    = 10
	maxTrendingLimit        = 50
	defaultAbandonmentHours = 12
	maxAbandonmentHours     = 24
)

// ConnectivityChecker reports whether the Kafka consumer is connected.
type ConnectivityChecker interface {
	Connected() bool
}

// AnalyticsHandler serves real-time analytics endpoints.
type AnalyticsHandler struct {
	store    store.Store
	consumer ConnectivityChecker
}

// NewAnalyticsHandler creates a handler wired to the store and consumer.
func NewAnalyticsHandler(s store.Store, consumer ConnectivityChecker) *AnalyticsHandler {
	return &AnalyticsHandler{
		store:    s,
		consumer: consumer,
	}
}

// Revenue returns windowed revenue data for the last N hours.
func (h *AnalyticsHandler) Revenue(c *gin.Context) {
	hours := parseIntParam(c, "hours", defaultRevenueHours, maxRevenueHours)

	windows, err := h.store.GetRevenue(c.Request.Context(), hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch revenue data"})
		return
	}
	if windows == nil {
		windows = []store.RevenueWindow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"windows": windows,
		"stale":   !h.consumer.Connected(),
	})
}

// Trending returns top trending products.
func (h *AnalyticsHandler) Trending(c *gin.Context) {
	limit := parseIntParam(c, "limit", defaultTrendingLimit, maxTrendingLimit)

	result, err := h.store.GetTrending(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trending data"})
		return
	}

	products := []store.TrendingProduct{}
	windowEnd := ""
	if result != nil && result.Products != nil {
		products = result.Products
		windowEnd = result.WindowEnd.UTC().Format("2006-01-02T15:04:05Z")
	}

	c.JSON(http.StatusOK, gin.H{
		"window_end": windowEnd,
		"products":   products,
		"stale":      !h.consumer.Connected(),
	})
}

// CartAbandonment returns cart abandonment metrics for the last N hours.
func (h *AnalyticsHandler) CartAbandonment(c *gin.Context) {
	hours := parseIntParam(c, "hours", defaultAbandonmentHours, maxAbandonmentHours)

	windows, err := h.store.GetAbandonment(c.Request.Context(), hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch abandonment data"})
		return
	}
	if windows == nil {
		windows = []store.AbandonmentWindow{}
	}

	c.JSON(http.StatusOK, gin.H{
		"windows": windows,
		"stale":   !h.consumer.Connected(),
	})
}

// Health returns service health including Kafka connectivity.
func (h *AnalyticsHandler) Health(c *gin.Context) {
	status := "ok"
	kafkaStatus := "connected"
	if !h.consumer.Connected() {
		kafkaStatus = "disconnected"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": status,
		"kafka":  kafkaStatus,
	})
}

// parseIntParam reads an integer query parameter, returning def if missing
// and clamping to max if the value exceeds it.
func parseIntParam(c *gin.Context, name string, def, max int) int {
	raw := c.Query(name)
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 1 {
		return def
	}
	if val > max {
		return max
	}
	return val
}
