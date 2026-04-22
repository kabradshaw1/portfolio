package main

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates the Gin engine with all middleware and route registrations.
func setupRouter(cfg Config, analyticsHandler *handler.AnalyticsHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("analytics-service"))
	router.Use(corsMiddleware(cfg.AllowedOrigins))
	router.Use(apperror.ErrorHandler())

	router.GET("/analytics/revenue", analyticsHandler.Revenue)
	router.GET("/analytics/trending", analyticsHandler.Trending)
	router.GET("/analytics/cart-abandonment", analyticsHandler.CartAbandonment)
	router.GET("/health", analyticsHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

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
			c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
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
