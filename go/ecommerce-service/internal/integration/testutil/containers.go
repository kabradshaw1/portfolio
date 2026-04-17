//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	tcontainers "github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Infra holds live connections to all three backing services spun up by
// testcontainers for a single test run.
type Infra struct {
	Pool        *pgxpool.Pool
	RedisClient *redis.Client
	RabbitConn  *amqp091.Connection
	RabbitCh    *amqp091.Channel

	pgContainer     *tcpostgres.PostgresContainer
	redisContainer  tcontainers.Container
	rabbitContainer *tcrabbit.RabbitMQContainer
}

// SetupInfra starts Postgres 16-alpine, Redis 7-alpine, and RabbitMQ 3-alpine
// containers, establishes connections to all three, and registers t.Cleanup to
// tear everything down at the end of the test.
func SetupInfra(ctx context.Context, t *testing.T) *Infra {
	t.Helper()

	// ── Postgres ──────────────────────────────────────────────────────────────
	pgC, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("ecommercedb_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	pgDSN, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		t.Fatalf("create pgxpool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	// ── Redis ─────────────────────────────────────────────────────────────────
	rdC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("start redis container: %v", err)
	}

	rdEndpoint, err := rdC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("redis endpoint: %v", err)
	}

	rdClient := redis.NewClient(&redis.Options{Addr: rdEndpoint})
	if err := rdClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}

	// ── RabbitMQ ──────────────────────────────────────────────────────────────
	rmqC, err := tcrabbit.Run(ctx, "rabbitmq:3-alpine")
	if err != nil {
		t.Fatalf("start rabbitmq container: %v", err)
	}

	rmqURL, err := rmqC.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("rabbitmq amqp url: %v", err)
	}

	rmqConn, err := amqp091.Dial(rmqURL)
	if err != nil {
		t.Fatalf("dial rabbitmq: %v", err)
	}

	rmqCh, err := rmqConn.Channel()
	if err != nil {
		t.Fatalf("open rabbitmq channel: %v", err)
	}

	if err := rmqCh.ExchangeDeclare(
		"ecommerce", // name
		"topic",     // kind
		true,        // durable
		false,       // auto-delete
		false,       // internal
		false,       // no-wait
		nil,         // args
	); err != nil {
		t.Fatalf("declare ecommerce exchange: %v", err)
	}

	infra := &Infra{
		Pool:            pool,
		RedisClient:     rdClient,
		RabbitConn:      rmqConn,
		RabbitCh:        rmqCh,
		pgContainer:     pgC,
		redisContainer:  rdC,
		rabbitContainer: rmqC,
	}

	t.Cleanup(func() {
		if err := rmqCh.Close(); err != nil {
			t.Logf("close rabbitmq channel: %v", err)
		}
		if err := rmqConn.Close(); err != nil {
			t.Logf("close rabbitmq connection: %v", err)
		}
		if err := rdClient.Close(); err != nil {
			t.Logf("close redis client: %v", err)
		}
		pool.Close()

		cleanCtx := context.Background()
		if err := pgC.Terminate(cleanCtx); err != nil {
			t.Logf("terminate postgres container: %v", err)
		}
		if err := rdC.Terminate(cleanCtx); err != nil {
			t.Logf("terminate redis container: %v", err)
		}
		if err := rmqC.Terminate(cleanCtx); err != nil {
			t.Logf("terminate rabbitmq container: %v", err)
		}
	})

	return infra
}
