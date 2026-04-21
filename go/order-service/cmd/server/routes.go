package main

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates the Gin engine with all middleware and route registrations.
func setupRouter(
	cfg Config,
	orderHandler *handler.OrderHandler,
	returnHandler *handler.ReturnHandler,
	healthHandler *handler.HealthHandler,
	adminHandler *handler.AdminHandler,
	redisClient *redis.Client,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("order-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	// Public routes
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	ecomLimiter := middleware.NewRateLimiter(redisClient, "ecom:ratelimit", 60, time.Minute)

	// Authenticated routes
	auth := router.Group("/")
	auth.Use(middleware.Auth(cfg.JWTSecret))
	auth.Use(ecomLimiter.Middleware())
	{
		auth.POST("/orders", middleware.Idempotency(redisClient, true), orderHandler.Checkout)
		auth.GET("/orders", orderHandler.List)
		auth.GET("/orders/:id", orderHandler.GetByID)
		auth.POST("/orders/:id/returns", returnHandler.Initiate)
	}

	// Admin routes — no auth, protected by network boundary.
	admin := router.Group("/admin")
	{
		admin.GET("/dlq/messages", adminHandler.ListDLQ)
		admin.POST("/dlq/replay", adminHandler.ReplayDLQ)
	}

	return router
}
