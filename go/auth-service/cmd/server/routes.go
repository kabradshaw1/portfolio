package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	pkgmw "github.com/kabradshaw1/portfolio/go/pkg/middleware"
)

// setupRouter creates a Gin engine with all middleware and routes registered.
func setupRouter(cfg Config, authHandler *handler.AuthHandler, healthHandler *handler.HealthHandler, authLimiter *middleware.RateLimiter) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(pkgmw.SecurityHeaders())
	router.Use(otelgin.Middleware("auth-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(cfg.AllowedOrigins))

	router.POST("/auth/register", authLimiter.Middleware(), authHandler.Register)
	router.POST("/auth/login", authLimiter.Middleware(), authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/auth/google", authLimiter.Middleware(), authHandler.GoogleLogin)
	router.POST("/auth/logout", authHandler.Logout)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return router
}
