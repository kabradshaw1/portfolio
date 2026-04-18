package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
)

// ConnectivityChecker reports whether the Kafka consumer is connected.
type ConnectivityChecker interface {
	Connected() bool
}

// AnalyticsHandler serves real-time analytics endpoints.
type AnalyticsHandler struct {
	orders   *aggregator.OrderAggregator
	trending *aggregator.TrendingAggregator
	carts    *aggregator.CartAggregator
	consumer ConnectivityChecker
}

// NewAnalyticsHandler creates a handler wired to the aggregators and consumer.
func NewAnalyticsHandler(orders *aggregator.OrderAggregator, trending *aggregator.TrendingAggregator, carts *aggregator.CartAggregator, consumer ConnectivityChecker) *AnalyticsHandler {
	return &AnalyticsHandler{
		orders:   orders,
		trending: trending,
		carts:    carts,
		consumer: consumer,
	}
}

// Dashboard returns high-level aggregate metrics.
func (h *AnalyticsHandler) Dashboard(c *gin.Context) {
	orderStats := h.orders.Stats()
	cartStats := h.carts.Stats()

	stale := !h.consumer.Connected()

	c.JSON(http.StatusOK, gin.H{
		"ordersPerHour":  orderStats.OrdersPerHour,
		"revenuePerHour": orderStats.RevenuePerHour,
		"completionRate": orderStats.CompletionRate,
		"activeCarts":    cartStats.ActiveCarts,
		"stale":          stale,
	})
}

// Trending returns top trending products.
func (h *AnalyticsHandler) Trending(c *gin.Context) {
	products := h.trending.TopProducts()
	stale := !h.consumer.Connected()

	c.JSON(http.StatusOK, gin.H{
		"products": products,
		"stale":    stale,
	})
}

// Orders returns order volume and status breakdown.
func (h *AnalyticsHandler) Orders(c *gin.Context) {
	stats := h.orders.Stats()
	stale := !h.consumer.Connected()

	c.JSON(http.StatusOK, gin.H{
		"hourly":          stats.Hourly,
		"statusBreakdown": stats.StatusBreakdown,
		"stale":           stale,
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
