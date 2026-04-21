package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	amqp "github.com/rabbitmq/amqp091-go"

	grpcsrv "github.com/kabradshaw1/portfolio/go/cart-service/internal/grpc"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/productclient"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/cart-service/internal/worker"
	pb "github.com/kabradshaw1/portfolio/go/cart-service/pb/cart/v1"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "cart-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)
	defer pool.Close()

	redisClient := connectRedis(ctx, cfg.RedisURL)
	kafkaPub := connectKafka(cfg.KafkaBrokers)
	defer kafkaPub.Close()

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "cart-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})

	var prodClient *productclient.GRPCClient
	if cfg.ProductGRPCAddr != "" {
		prodClient, err = productclient.New(cfg.ProductGRPCAddr)
		if err != nil {
			log.Fatalf("product gRPC client: %v", err)
		}
		defer prodClient.Close()
		slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
	}

	cartRepo := repository.NewCartRepository(pool, pgBreaker)
	cartSvc := service.NewCartService(cartRepo, kafkaPub, prodClient)

	// RabbitMQ saga handler (optional)
	if cfg.RabbitmqURL != "" {
		conn, err := amqp.Dial(cfg.RabbitmqURL)
		if err != nil {
			log.Fatalf("rabbitmq connect: %v", err)
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			log.Fatalf("rabbitmq channel: %v", err)
		}
		defer ch.Close()

		sagaHandler := worker.NewSagaHandler(cartSvc, ch)
		go func() {
			if err := sagaHandler.Start(ctx); err != nil {
				slog.Error("saga handler failed", "error", err)
			}
		}()
		slog.Info("saga command handler enabled", "url", cfg.RabbitmqURL)
	}

	// REST server
	router := setupRouter(cfg,
		handler.NewCartHandler(cartSvc),
		handler.NewHealthHandler(pool, redisClient),
		redisClient,
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
	pb.RegisterCartServiceServer(grpcServer, grpcsrv.NewCartGRPCServer(cartSvc))

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("cart.v1.CartService", healthpb.HealthCheckResponse_SERVING)

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
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down servers")

	cancel()
	grpcServer.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("REST server forced to shutdown: %v", err)
	}
	slog.Info("servers stopped")
}
