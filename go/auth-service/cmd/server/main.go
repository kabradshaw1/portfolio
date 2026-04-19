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

	"github.com/kabradshaw1/portfolio/go/auth-service/internal/google"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx := context.Background()
	shutdownTracer, err := tracing.Init(ctx, "auth-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	pool := connectPostgres(ctx, cfg.DatabaseURL)
	defer pool.Close()

	redisClient := connectRedis(ctx, cfg.RedisURL)

	// Wire dependencies
	authLimiter := middleware.NewRateLimiter(redisClient, "auth:ratelimit", 10, time.Minute)
	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "auth-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	userRepo := repository.NewUserRepository(pool, pgBreaker)
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, accessTokenTTLMs, refreshTokenTTLMs)
	googleClient := google.NewClient(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleTokenURL, cfg.GoogleUserinfoURL)
	denylist := service.NewTokenDenylist(redisClient)
	accessTTL := time.Duration(accessTokenTTLMs) * time.Millisecond
	refreshTTL := time.Duration(refreshTokenTTLMs) * time.Millisecond
	cookieCfg := handler.CookieConfig{Secure: cfg.CookieSecure, Domain: cfg.CookieDomain, SameSite: cfg.CookieSameSite}
	authHandler := handler.NewAuthHandler(authSvc, googleClient, denylist, accessTTL, refreshTTL, cookieCfg)
	healthHandler := handler.NewHealthHandler(pool)

	router := setupRouter(cfg, authHandler, healthHandler, authLimiter)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

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
