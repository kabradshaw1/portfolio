package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "analytics-service", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	orders := aggregator.NewOrderAggregator()
	trending := aggregator.NewTrendingAggregator()
	carts := aggregator.NewCartAggregator()

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	cons := consumer.New(brokers, orders, trending, carts)
	go func() {
		if err := cons.Run(ctx); err != nil {
			slog.Error("kafka consumer failed", "error", err)
		}
	}()

	analyticsHandler := handler.NewAnalyticsHandler(orders, trending, carts, cons)
	router := setupRouter(cfg, analyticsHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("analytics-service starting", "port", cfg.Port, "brokers", cfg.KafkaBrokers)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down analytics-service")

	cancel() // Stop consumer

	if err := cons.Close(); err != nil {
		slog.Error("kafka consumer close error", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	slog.Info("analytics-service stopped")
}
