package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	grpcsrv "github.com/kabradshaw1/portfolio/go/product-service/internal/grpc"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/handler"
	pb "github.com/kabradshaw1/portfolio/go/product-service/pb/product/v1"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/product-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "product-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)

	redisClient := connectRedis(ctx, cfg.RedisURL)

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "product-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	productRepo := repository.NewProductRepository(pool, pgBreaker)
	productSvc := service.NewProductService(productRepo, redisClient)

	// REST server
	router := setupRouter(cfg,
		handler.NewProductHandler(productSvc),
		handler.NewHealthHandler(pool, redisClient),
	)

	httpSrv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("REST server starting", "port", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("REST server failed: %v", err)
		}
	}()

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	pb.RegisterProductServiceServer(grpcServer, grpcsrv.NewProductGRPCServer(
		productSvc,
		grpcsrv.WithStockDecrementer(productRepo),
	))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("product.v1.ProductService", healthpb.HealthCheckResponse_SERVING)

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

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("product-http", httpSrv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("product-grpc", grpcServer))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
