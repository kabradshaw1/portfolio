package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/aggregator"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/analytics-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
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
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))

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

	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("kafka-close", 10, func(_ context.Context) error {
		return cons.Close()
	})
	sm.Register("http", 20, func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
