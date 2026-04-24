package main

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates the Gin engine with all middleware and route registrations.
func setupRouter(
	cfg Config,
	health *handler.HealthHandler,
	timeline *handler.TimelineHandler,
	summary *handler.SummaryHandler,
	stats *handler.StatsHandler,
	replay *handler.ReplayHandler,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("order-projector"))
	router.Use(corsMiddleware(cfg.AllowedOrigins))
	router.Use(apperror.ErrorHandler())

	// Infrastructure endpoints.
	router.GET("/health", health.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Projection query endpoints.
	router.GET("/orders/:id/timeline", timeline.GetTimeline)
	router.GET("/orders/:id", summary.GetOrder)
	router.GET("/orders", summary.ListOrders)
	router.GET("/stats/orders", stats.GetOrderStats)

	// Admin endpoints.
	router.POST("/admin/replay", replay.TriggerReplay)

	return router
}

// corsMiddleware returns a Gin middleware that sets CORS headers for allowed origins.
func corsMiddleware(allowedOrigins string) gin.HandlerFunc {
	originSet := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		originSet[strings.TrimSpace(o)] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if originSet[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
