package main

import (
	"context"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	kafka "github.com/segmentio/kafka-go"
)

// connectPostgres creates a tuned pgxpool connection.
func connectPostgres(ctx context.Context, databaseURL string) *pgxpool.Pool {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("failed to parse database URL: %v", err)
	}

	poolConfig.MaxConns = 15
	poolConfig.MinConns = 3
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

// connectRabbitMQ dials RabbitMQ and opens an AMQP channel.
func connectRabbitMQ(url string) (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial(url)
	if err != nil {
		log.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	slog.Info("connected to RabbitMQ")
	return conn, ch
}

// connectKafka creates a Kafka writer. Returns nil if brokers is empty.
func connectKafka(brokers string) *kafka.Writer {
	if brokers == "" {
		return nil
	}
	addrs := strings.Split(brokers, ",")
	w := &kafka.Writer{
		Addr:         kafka.TCP(addrs...),
		Balancer:     &kafka.LeastBytes{},
		Async:        true,
		BatchSize:    100,
		WriteTimeout: 1 * time.Second,
		RequiredAcks: kafka.RequireOne,
	}
	slog.Info("kafka writer enabled", "brokers", brokers)
	return w
}
