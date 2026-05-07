package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	appkafka "github.com/kabradshaw1/portfolio/go/order-service/internal/kafka"
)

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

const (
	rabbitInitialReconnectBackoff = 1 * time.Second
	rabbitMaxReconnectBackoff     = 30 * time.Second
)

// connectRabbitMQ connects and declares the ecommerce exchange.
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
	if err := ch.ExchangeDeclare("ecommerce", "topic", true, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("declare ecommerce exchange: %w", err)
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

// connectKafka creates a Kafka producer, or a NopProducer if brokers is empty.
func connectKafka(brokers string) appkafka.Producer {
	if brokers == "" {
		return appkafka.NopProducer{}
	}
	p := appkafka.NewBestEffortProducer(strings.Split(brokers, ","))
	slog.Info("kafka producer enabled", "brokers", brokers)
	return p
}
