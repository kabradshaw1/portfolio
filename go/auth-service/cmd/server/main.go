package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	// Connect to Postgres
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")

	// Wire dependencies
	userRepo := repository.NewUserRepository(pool)
	authSvc := service.NewAuthService(userRepo, jwtSecret, 900000, 604800000)
	authHandler := handler.NewAuthHandler(authSvc, nil) // TODO(task8): wire google client
	healthHandler := handler.NewHealthHandler(pool)

	// Set up Gin
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.Logging())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(allowedOrigins))

	// Routes
	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Start server
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("server stopped")
}
