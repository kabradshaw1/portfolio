package main

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/cart-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

func setupRouter(
	cfg Config,
	cartHandler *handler.CartHandler,
	healthHandler *handler.HealthHandler,
	redisClient *redis.Client,
	authMw gin.HandlerFunc,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("cart-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	cartLimiter := middleware.NewRateLimiter(redisClient, "cart:ratelimit", 60, time.Minute)

	auth := router.Group("/")
	auth.Use(authMw)
	auth.Use(cartLimiter.Middleware())
	{
		auth.GET("/cart", cartHandler.GetCart)
		auth.POST("/cart", middleware.Idempotency(redisClient, false), cartHandler.AddItem)
		auth.PUT("/cart/:itemId", cartHandler.UpdateQuantity)
		auth.DELETE("/cart/:itemId", cartHandler.RemoveItem)
	}

	return router
}
