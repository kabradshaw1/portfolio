package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/kabradshaw1/portfolio/go/auth-service/authmiddleware"
	authpb "github.com/kabradshaw1/portfolio/go/auth-service/pb/auth/v1"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/cartclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/productclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "order-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)

	redisClient := connectRedis(ctx, cfg.RedisURL)

	conn, ch := connectRabbitMQ(cfg.RabbitmqURL)
	defer conn.Close()
	defer ch.Close()

	kafkaPub := connectKafka(cfg.KafkaBrokers)
	defer kafkaPub.Close()

	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "order-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	orderRepo := repository.NewOrderRepository(pool, pgBreaker)
	returnRepo := repository.NewReturnRepository(pool, pgBreaker)

	var prodClient *productclient.GRPCClient
	if cfg.ProductGRPCAddr != "" {
		var err error
		prodClient, err = productclient.New(cfg.ProductGRPCAddr, insecure.NewCredentials())
		if err != nil {
			log.Fatalf("product gRPC client: %v", err)
		}
		defer prodClient.Close()
		slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
	}

	var cartClient *cartclient.GRPCClient
	if cfg.CartGRPCAddr != "" {
		var err error
		cartClient, err = cartclient.New(cfg.CartGRPCAddr, cfg.ProductGRPCAddr, insecure.NewCredentials())
		if err != nil {
			log.Fatalf("cart gRPC client: %v", err)
		}
		defer cartClient.Close()
		slog.Info("connected to cart-service gRPC", "addr", cfg.CartGRPCAddr)
	}

	// Declare saga RabbitMQ topology.
	if err := saga.DeclareTopology(ch); err != nil {
		log.Fatalf("saga topology: %v", err)
	}

	// Create DLQ client for admin endpoints.
	dlqClient := saga.NewDLQClient(ch)

	// Create saga orchestrator with stock checker adapter.
	sagaPub := saga.NewPublisher(ch)
	orch := saga.NewOrchestrator(orderRepo, sagaPub, prodClient, kafkaPub)

	// Start saga event consumer.
	consumer := saga.NewConsumer(orch)
	go func() {
		if err := consumer.Start(ctx, ch); err != nil {
			slog.Error("saga consumer failed", "error", err)
		}
	}()

	// Recover incomplete sagas from previous crashes.
	saga.RecoverIncomplete(ctx, orderRepo, orch)

	orderSvc := service.NewOrderService(orderRepo, cartClient, orch)
	returnSvc := service.NewReturnService(returnRepo, orderSvc)

	// Auth-service gRPC connection for denylist checks.
	authConn, err := grpc.NewClient(cfg.AuthGRPCURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("auth gRPC dial: %v", err)
	}
	defer authConn.Close()
	authClient := authpb.NewAuthServiceClient(authConn)
	authMw := authmiddleware.New(cfg.JWTSecret, authClient)

	router := setupRouter(cfg,
		handler.NewOrderHandler(orderSvc),
		handler.NewReturnHandler(returnSvc),
		handler.NewHealthHandler(pool, redisClient),
		handler.NewAdminHandler(dlqClient),
		redisClient,
		authMw,
	)

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

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("http", 20, func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
