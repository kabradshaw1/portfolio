package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/consumer"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/replay"
	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/buildinfo"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
)

func main() {
	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdownTracer, err := tracing.Init(ctx, "order-projector", cfg.OTELEndpoint)
	if err != nil {
		log.Fatalf("tracing init: %v", err)
	}
	slog.SetDefault(slog.New(
		tracing.NewLogHandler(slog.NewJSONHandler(os.Stdout, nil)),
	))
	buildinfo.Log()

	// PostgreSQL connection pool with explicit tuning.
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to parse DATABASE_URL: %v", err)
	}
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 2
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping postgres: %v", err)
	}
	slog.Info("connected to postgres")

	// Repository with circuit breaker.
	pgBreaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name:          "projector-postgres",
		OnStateChange: resilience.ObserveStateChange,
	})
	repo := repository.New(pool, pgBreaker)

	// Kafka consumer with all three projections.
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	cons := consumer.New(brokers, repo)
	go func() {
		if err := cons.Run(ctx); err != nil {
			slog.Error("kafka consumer failed", "error", err)
		}
	}()

	// Replay coordinator.
	replayer := replay.New(repo, cons)

	// HTTP handlers.
	healthHandler := handler.NewHealthHandler(pool, cons)
	timelineHandler := handler.NewTimelineHandler(repo)
	summaryHandler := handler.NewSummaryHandler(repo)
	statsHandler := handler.NewStatsHandler(repo)
	replayHandler := handler.NewReplayHandler(replayer)

	// Router with all routes.
	router := setupRouter(cfg, healthHandler, timelineHandler, summaryHandler, statsHandler, replayHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("order-projector starting", "port", cfg.Port, "brokers", cfg.KafkaBrokers)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Graceful shutdown.
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("order-projector-http", srv))
	sm.Register("wait-kafka", 10, shutdown.WaitForInflight("kafka-consumer", cons.IsIdle, 100*time.Millisecond))
	sm.Register("kafka-close", 20, func(_ context.Context) error {
		return cons.Close()
	})
	sm.Register("postgres-close", 25, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
}
