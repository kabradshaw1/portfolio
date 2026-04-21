package main

import (
	"context"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	appkafka "github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

// connectPostgres creates a tuned pgxpool connection.
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

// connectRedis optionally connects to Redis. Returns nil if URL is empty or unreachable.
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

// connectRabbitMQ connects and declares the ecommerce exchange.
func connectRabbitMQ(rabbitmqURL string) (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial(rabbitmqURL)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	if err := ch.ExchangeDeclare("ecommerce", "topic", true, false, false, false, nil); err != nil {
		log.Fatalf("failed to declare exchange: %v", err)
	}
	slog.Info("connected to RabbitMQ")
	return conn, ch
}

// connectKafka creates a Kafka producer, or a NopProducer if brokers is empty.
func connectKafka(brokers string) appkafka.Producer {
	if brokers == "" {
		return appkafka.NopProducer{}
	}
	p := appkafka.NewProducer(strings.Split(brokers, ","))
	slog.Info("kafka producer enabled", "brokers", brokers)
	return p
}
