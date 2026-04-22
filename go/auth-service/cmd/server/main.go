package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	authgrpc "github.com/kabradshaw1/portfolio/go/auth-service/internal/grpc"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/middleware"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/auth-service/internal/service"
	pb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/kabradshaw1/portfolio/go/auth-service/internal/google"
)

func main() {
	cfg := loadConfig()

	ctx := context.Background()
	shutdownTracer, err := tracing.Init(ctx, "auth-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)

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

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterAuthServiceServer(grpcServer, authgrpc.NewAuthGRPCServer(cfg.JWTSecret, denylist))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("auth.v1.AuthService", healthpb.HealthCheckResponse_SERVING)

	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("gRPC listen: %v", err)
	}

	go func() {
		slog.Info("gRPC server starting", "port", cfg.GRPCPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	sm := shutdown.New(15 * time.Second)
	sm.Register("drain-http", 0, shutdown.DrainHTTP("auth-http", srv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("auth-grpc", grpcServer))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("redis", 20, func(_ context.Context) error {
		if redisClient != nil {
			return redisClient.Close()
		}
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
