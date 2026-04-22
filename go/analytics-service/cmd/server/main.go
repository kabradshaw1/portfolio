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
	"github.com/redis/go-redis/v9"
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

	redisClient := connectRedis(ctx, cfg.RedisURL)

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
	sm.Register("drain-http", 0, shutdown.DrainHTTP("analytics-http", srv))
	sm.Register("wait-kafka", 10, shutdown.WaitForInflight("kafka-consumer", cons.IsIdle, 100*time.Millisecond))
	sm.Register("kafka-close", 20, func(_ context.Context) error {
		return cons.Close()
	})
	if redisClient != nil {
		sm.Register("redis-close", 25, func(_ context.Context) error {
			return redisClient.Close()
		})
	}
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}

func connectRedis(ctx context.Context, redisURL string) *redis.Client {
	if redisURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("failed to parse REDIS_URL: %v", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("redis not available, continuing without cache", "error", err)
		return nil
	}
	slog.Info("connected to redis")
	return client
}
