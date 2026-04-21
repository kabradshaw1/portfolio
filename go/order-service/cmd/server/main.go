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

	"github.com/kabradshaw1/portfolio/go/order-service/internal/cartclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/productclient"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/repository"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/service"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
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
	defer func() { _ = shutdownTracer(ctx) }()

	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

	pool := connectPostgres(ctx, cfg.DatabaseURL)
	defer pool.Close()

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
		prodClient, err = productclient.New(cfg.ProductGRPCAddr)
		if err != nil {
			log.Fatalf("product gRPC client: %v", err)
		}
		defer prodClient.Close()
		slog.Info("connected to product-service gRPC", "addr", cfg.ProductGRPCAddr)
	}

	var cartClient *cartclient.GRPCClient
	if cfg.CartGRPCAddr != "" {
		var err error
		cartClient, err = cartclient.New(cfg.CartGRPCAddr, cfg.ProductGRPCAddr)
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

	router := setupRouter(cfg,
		handler.NewOrderHandler(orderSvc),
		handler.NewReturnHandler(returnSvc),
		handler.NewHealthHandler(pool, redisClient),
		handler.NewAdminHandler(dlqClient),
		redisClient,
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("server stopped")
}
