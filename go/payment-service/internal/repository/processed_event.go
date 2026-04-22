package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

type ProcessedEventRepository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

func NewProcessedEventRepository(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *ProcessedEventRepository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &ProcessedEventRepository{pool: pool, breaker: breaker, retryCfg: cfg}
}

// TryInsert attempts to record a processed event for idempotency.
// Returns (true, nil) if the event was newly inserted, (false, nil) if it was a duplicate.
// If tx is non-nil, it uses the transaction; otherwise it wraps the call in resilience.Call with the pool.
func (r *ProcessedEventRepository) TryInsert(ctx context.Context, tx pgx.Tx, eventID, eventType string) (bool, error) {
	if tx == nil && r.pool == nil {
		return false, fmt.Errorf("database pool is nil")
	}
	const query = `INSERT INTO processed_events (stripe_event_id, event_type, processed_at)
	               VALUES ($1, $2, NOW())
	               ON CONFLICT (stripe_event_id) DO NOTHING`

	if tx != nil {
		tag, err := tx.Exec(ctx, query, eventID, eventType)
		if err != nil {
			return false, fmt.Errorf("try insert processed event (tx): %w", err)
		}
		return tag.RowsAffected() == 1, nil
	}

	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (bool, error) {
		tag, err := r.pool.Exec(ctx, query, eventID, eventType)
		if err != nil {
			return false, fmt.Errorf("try insert processed event: %w", err)
		}
		return tag.RowsAffected() == 1, nil
	})
}
