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
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/kabradshaw1/portfolio/go/auth-service/internal/google"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

const (
	accessTokenTTLMs  = 900_000     // 15 minutes
	refreshTokenTTLMs = 604_800_000 // 7 days
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
	googleClientID := os.Getenv("GOOGLE_CLIENT_ID")
	if googleClientID == "" {
		log.Fatal("GOOGLE_CLIENT_ID is required")
	}
	googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if googleClientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_SECRET is required")
	}
	googleTokenURL := os.Getenv("GOOGLE_TOKEN_URL")
	if googleTokenURL == "" {
		googleTokenURL = "https://oauth2.googleapis.com/token"
	}
	googleUserinfoURL := os.Getenv("GOOGLE_USERINFO_URL")
	if googleUserinfoURL == "" {
		googleUserinfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	}
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:3000"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	// Tracing
	ctx := context.Background()
	shutdownTracer, err := tracing.Init(ctx, "auth-service", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	// Connect to Postgres
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database config: %v", err)
	}
	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")

	// Wire dependencies
	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "auth-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	userRepo := repository.NewUserRepository(pool, pgBreaker)
	authSvc := service.NewAuthService(userRepo, jwtSecret, accessTokenTTLMs, refreshTokenTTLMs)
	googleClient := google.NewClient(googleClientID, googleClientSecret, googleTokenURL, googleUserinfoURL)
	authHandler := handler.NewAuthHandler(authSvc, googleClient)
	healthHandler := handler.NewHealthHandler(pool)

	// Set up Gin
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware("auth-service"))
	router.Use(middleware.Logging())
	router.Use(apperror.ErrorHandler())
	router.Use(middleware.Metrics())
	router.Use(middleware.CORS(allowedOrigins))

	// Routes
	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/login", authHandler.Login)
	router.POST("/auth/refresh", authHandler.Refresh)
	router.POST("/auth/google", authHandler.GoogleLogin)
	router.GET("/health", healthHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Start server
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
