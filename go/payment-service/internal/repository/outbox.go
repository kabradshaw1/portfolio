package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/payment-service/internal/model"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

type OutboxRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewOutboxRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *OutboxRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &OutboxRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

// Insert adds a message to the outbox. If tx is non-nil, it uses the transaction;
// otherwise it wraps the call in resilience.Call with the pool.
func (r *OutboxRepository) Insert(ctx context.Context, tx pgx.Tx, exchange, routingKey string, payload []byte) error {
	if tx == nil && r.pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	if tx != nil {
		_, err := tx.Exec(ctx,
			`INSERT INTO outbox (id, exchange, routing_key, payload, published, created_at)
			 VALUES ($1, $2, $3, $4, false, NOW())`,
			uuid.New(), exchange, routingKey, payload,
		)
		if err != nil {
			return fmt.Errorf("insert outbox message (tx): %w", err)
		}
		return nil
	}
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO outbox (id, exchange, routing_key, payload, published, created_at)
			 VALUES ($1, $2, $3, $4, false, NOW())`,
			uuid.New(), exchange, routingKey, payload,
		)
		if err != nil {
			return fmt.Errorf("insert outbox message: %w", err)
		}
		return nil
	})
}

// FetchUnpublished returns up to limit unpublished outbox messages ordered by created_at.
func (r *OutboxRepository) FetchUnpublished(ctx context.Context, limit int) ([]model.OutboxMessage, error) {
	if r.pool == nil {
		return nil, fmt.Errorf("database pool is nil")
	}
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]model.OutboxMessage, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT id, exchange, routing_key, payload, published, created_at
			 FROM outbox WHERE published = false ORDER BY created_at LIMIT $1`,
			limit,
		)
		if err != nil {
			return nil, fmt.Errorf("fetch unpublished outbox: %w", err)
		}
		defer rows.Close()

		var messages []model.OutboxMessage
		for rows.Next() {
			var m model.OutboxMessage
			if err := rows.Scan(&m.ID, &m.Exchange, &m.RoutingKey, &m.Payload, &m.Published, &m.CreatedAt); err != nil {
				return nil, fmt.Errorf("scan outbox message: %w", err)
			}
			messages = append(messages, m)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows error: %w", err)
		}
		return messages, nil
	})
}

// MarkPublished sets the published flag to true for the given outbox message ID.
func (r *OutboxRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	if r.pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			"UPDATE outbox SET published = true WHERE id = $1",
			id,
		)
		if err != nil {
			return fmt.Errorf("mark outbox published: %w", err)
		}
		return nil
	})
}
