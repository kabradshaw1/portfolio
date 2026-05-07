package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	kafka "github.com/segmentio/kafka-go"

	rabbitmq "github.com/kabradshaw1/portfolio/go/pkg/rabbitmq"
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

type rabbitMQConnection interface {
	Channel() (rabbitmq.ConfirmChannel, error)
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
	Close() error
}

type amqpConnection struct {
	*amqp.Connection
}

func (c amqpConnection) Channel() (rabbitmq.ConfirmChannel, error) {
	return c.Connection.Channel()
}

type rabbitMQDialer func(url string) (rabbitMQConnection, error)

// reconnectingRabbitMQPublisher owns the RabbitMQ connection used by the
// payment outbox. It never retries a publish after an ambiguous channel error;
// it reconnects on the next publish attempt so the poller leaves that outbox row
// unpublished until a later confirmed publish succeeds.
type reconnectingRabbitMQPublisher struct {
	url  string
	dial rabbitMQDialer
	mu   sync.Mutex
	conn rabbitMQConnection
	pub  interface {
		Publish(ctx context.Context, exchange, routingKey string, msg amqp.Publishing) error
	}
	closed <-chan *amqp.Error
}

func connectRabbitMQPublisher(url string) *reconnectingRabbitMQPublisher {
	return newReconnectingRabbitMQPublisher(url, func(url string) (rabbitMQConnection, error) {
		conn, err := amqp.Dial(url)
		if err != nil {
			return nil, err
		}
		return amqpConnection{Connection: conn}, nil
	})
}

func newReconnectingRabbitMQPublisher(url string, dial rabbitMQDialer) *reconnectingRabbitMQPublisher {
	return &reconnectingRabbitMQPublisher{
		url:  url,
		dial: dial,
	}
}

func (p *reconnectingRabbitMQPublisher) Publish(
	ctx context.Context,
	exchange, routingKey string,
	msg amqp.Publishing,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensurePublisherLocked(ctx); err != nil {
		return err
	}

	if err := p.pub.Publish(ctx, exchange, routingKey, msg); err != nil {
		if closeErr := p.closeLocked(); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close RabbitMQ after publish failure: %w", closeErr))
		}
		return err
	}
	return nil
}

func (p *reconnectingRabbitMQPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeLocked()
}

func (p *reconnectingRabbitMQPublisher) ensurePublisherLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.pub != nil && !p.connectionClosedLocked() {
		return nil
	}

	if closeErr := p.closeLocked(); closeErr != nil {
		slog.WarnContext(ctx, "failed to close stale RabbitMQ connection", "error", closeErr)
	}

	conn, err := p.dial(p.url)
	if err != nil {
		return fmt.Errorf("connect RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open RabbitMQ channel: %w", err)
	}

	pub, err := rabbitmq.NewPublisher(ch)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("create RabbitMQ publisher: %w", err)
	}

	p.conn = conn
	p.pub = pub
	p.closed = conn.NotifyClose(make(chan *amqp.Error, 1))
	slog.InfoContext(ctx, "connected to RabbitMQ")
	return nil
}

func (p *reconnectingRabbitMQPublisher) connectionClosedLocked() bool {
	if p.closed == nil {
		return false
	}
	select {
	case err, ok := <-p.closed:
		if ok && err != nil {
			slog.Warn("RabbitMQ connection closed", "error", err)
		}
		return true
	default:
		return false
	}
}

func (p *reconnectingRabbitMQPublisher) closeLocked() error {
	conn := p.conn
	p.conn = nil
	p.pub = nil
	p.closed = nil
	if conn == nil {
		return nil
	}
	return conn.Close()
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
