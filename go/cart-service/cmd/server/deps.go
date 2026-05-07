package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	appkafka "github.com/kabradshaw1/portfolio/go/cart-service/internal/kafka"
)

const (
	rabbitInitialReconnectBackoff = 1 * time.Second
	rabbitMaxReconnectBackoff     = 30 * time.Second
)

func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database URL: %v", err)
	}
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	slog.Info("connected to database")
	return pool
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

func connectKafka(brokers string) appkafka.Producer {
	if brokers == "" {
		return appkafka.NopProducer{}
	}
	p := appkafka.NewProducer(strings.Split(brokers, ","))
	slog.Info("kafka producer enabled", "brokers", brokers)
	return p
}

func connectRabbitMQ(rabbitmqURL string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	slog.Info("connected to RabbitMQ")
	return conn, ch, nil
}

func waitForRabbitMQReconnect(ctx context.Context, attempt int) bool {
	backoff := rabbitReconnectBackoff(attempt)
	select {
	case <-ctx.Done():
		return false
	case <-time.After(backoff):
		return true
	}
}

func rabbitReconnectBackoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	multiplier := math.Pow(2, float64(attempt))
	backoff := time.Duration(float64(rabbitInitialReconnectBackoff) * multiplier)
	if backoff > rabbitMaxReconnectBackoff {
		return rabbitMaxReconnectBackoff
	}
	return backoff
}
