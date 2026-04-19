package main

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates the Gin engine with all middleware and route registrations.
func setupRouter(
	cfg Config,
	productHandler *handler.ProductHandler,
	cartHandler *handler.CartHandler,
	orderHandler *handler.OrderHandler,
	returnHandler *handler.ReturnHandler,
	healthHandler *handler.HealthHandler,
	redisClient *redis.Client,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("ecommerce-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	// Public routes
	router.GET("/products", productHandler.List)
	router.GET("/products/:id", productHandler.GetByID)
	router.GET("/categories", productHandler.Categories)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	ecomLimiter := middleware.NewRateLimiter(redisClient, "ecom:ratelimit", 60, time.Minute)

	// Authenticated routes
	auth := router.Group("/")
	auth.Use(middleware.Auth(cfg.JWTSecret))
	auth.Use(ecomLimiter.Middleware())
	{
		auth.GET("/cart", cartHandler.GetCart)
		auth.POST("/cart", middleware.Idempotency(redisClient, false), cartHandler.AddItem)
		auth.PUT("/cart/:itemId", cartHandler.UpdateQuantity)
		auth.DELETE("/cart/:itemId", cartHandler.RemoveItem)

		auth.POST("/orders", middleware.Idempotency(redisClient, true), orderHandler.Checkout)
		auth.GET("/orders", orderHandler.List)
		auth.GET("/orders/:id", orderHandler.GetByID)
		auth.POST("/orders/:id/returns", returnHandler.Initiate)
	}

	return router
}
