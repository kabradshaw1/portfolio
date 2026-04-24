package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

// TimelineEvent represents a single event in an order's timeline.
type TimelineEvent struct {
	EventID      string          `json:"eventId"`
	OrderID      string          `json:"orderId"`
	EventType    string          `json:"eventType"`
	EventVersion int             `json:"eventVersion"`
	Data         json.RawMessage `json:"data"`
	Timestamp    time.Time       `json:"timestamp"`
}

// OrderSummary is the denormalized read model for a single order.
type OrderSummary struct {
	OrderID       string          `json:"orderId"`
	UserID        string          `json:"userId"`
	Status        string          `json:"status"`
	TotalCents    int64           `json:"totalCents"`
	Currency      string          `json:"currency"`
	Items         json.RawMessage `json:"items,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	CompletedAt   *time.Time      `json:"completedAt,omitempty"`
	FailureReason *string         `json:"failureReason,omitempty"`
}

// OrderStats holds hourly aggregated order metrics.
type OrderStats struct {
	HourBucket           time.Time `json:"hourBucket"`
	OrdersCreated        int       `json:"ordersCreated"`
	OrdersCompleted      int       `json:"ordersCompleted"`
	OrdersFailed         int       `json:"ordersFailed"`
	AvgCompletionSeconds float64   `json:"avgCompletionSeconds"`
	TotalRevenueCents    int64     `json:"totalRevenueCents"`
}

// ReplayStatus tracks the state of a projection replay operation.
type ReplayStatus struct {
	IsReplaying     bool       `json:"isReplaying"`
	Projection      string     `json:"projection"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	EventsProcessed int64      `json:"eventsProcessed"`
	TotalEvents     int64      `json:"totalEvents"`
}

// Repository wraps pgxpool with circuit breaker and retry for read-model persistence.
type Repository struct {
	pool     *pgxpool.Pool
	breaker  *gobreaker.CircuitBreaker[any]
	retryCfg resilience.RetryConfig
}

// New creates a Repository with default retry config for PostgreSQL.
func New(pool *pgxpool.Pool, breaker *gobreaker.CircuitBreaker[any]) *Repository {
	cfg := resilience.DefaultRetryConfig()
	cfg.IsRetryable = resilience.IsPgRetryable
	return &Repository{pool: pool, breaker: breaker, retryCfg: cfg}
}

// ---------------------------------------------------------------------------
// Write operations
// ---------------------------------------------------------------------------

// InsertTimelineEvent inserts an event into the timeline table. Idempotent via
// ON CONFLICT (event_id) DO NOTHING so duplicate deliveries are safe.
func (r *Repository) InsertTimelineEvent(ctx context.Context, ev TimelineEvent) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_timeline (event_id, order_id, event_type, event_version, data, timestamp)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (event_id) DO NOTHING`,
			ev.EventID, ev.OrderID, ev.EventType, ev.EventVersion, ev.Data, ev.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("insert timeline event: %w", err)
		}
		return nil
	})
}

// UpsertOrderSummary creates or updates the denormalized order summary.
// COALESCE preserves existing nullable fields when the incoming value is NULL.
func (r *Repository) UpsertOrderSummary(ctx context.Context, s OrderSummary) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_summary
			   (order_id, user_id, status, total_cents, currency, items, created_at, updated_at, completed_at, failure_reason)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (order_id) DO UPDATE SET
			   user_id        = COALESCE(EXCLUDED.user_id, order_summary.user_id),
			   status         = EXCLUDED.status,
			   total_cents    = COALESCE(EXCLUDED.total_cents, order_summary.total_cents),
			   currency       = COALESCE(EXCLUDED.currency, order_summary.currency),
			   items          = COALESCE(EXCLUDED.items, order_summary.items),
			   updated_at     = EXCLUDED.updated_at,
			   completed_at   = COALESCE(EXCLUDED.completed_at, order_summary.completed_at),
			   failure_reason = COALESCE(EXCLUDED.failure_reason, order_summary.failure_reason)`,
			s.OrderID, s.UserID, s.Status, s.TotalCents, s.Currency,
			s.Items, s.CreatedAt, s.UpdatedAt, s.CompletedAt, s.FailureReason,
		)
		if err != nil {
			return fmt.Errorf("upsert order summary: %w", err)
		}
		return nil
	})
}

// UpsertOrderStats increments hourly order counters and revenue.
func (r *Repository) UpsertOrderStats(ctx context.Context, bucket time.Time, created, completed, failed int, revenueCents int64) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO order_stats (hour_bucket, orders_created, orders_completed, orders_failed, total_revenue_cents)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (hour_bucket) DO UPDATE SET
			   orders_created     = order_stats.orders_created     + EXCLUDED.orders_created,
			   orders_completed   = order_stats.orders_completed   + EXCLUDED.orders_completed,
			   orders_failed      = order_stats.orders_failed      + EXCLUDED.orders_failed,
			   total_revenue_cents = order_stats.total_revenue_cents + EXCLUDED.total_revenue_cents`,
			bucket, created, completed, failed, revenueCents,
		)
		if err != nil {
			return fmt.Errorf("upsert order stats: %w", err)
		}
		return nil
	})
}

// StartReplay inserts a new replay-started record for the given projection.
func (r *Repository) StartReplay(ctx context.Context, projection string) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`INSERT INTO replay_status (projection, is_replaying, started_at, events_processed, total_events)
			 VALUES ($1, true, NOW(), 0, 0)`,
			projection,
		)
		if err != nil {
			return fmt.Errorf("start replay: %w", err)
		}
		return nil
	})
}

// CompleteReplay marks the most recent replay record as done.
func (r *Repository) CompleteReplay(ctx context.Context) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`UPDATE replay_status
			 SET is_replaying = false, completed_at = NOW()
			 WHERE id = (SELECT id FROM replay_status ORDER BY started_at DESC LIMIT 1)`,
		)
		if err != nil {
			return fmt.Errorf("complete replay: %w", err)
		}
		return nil
	})
}

// IncrementReplayProgress adds count to events_processed on the latest replay.
func (r *Repository) IncrementReplayProgress(ctx context.Context, count int64) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx,
			`UPDATE replay_status
			 SET events_processed = events_processed + $1
			 WHERE id = (SELECT id FROM replay_status ORDER BY started_at DESC LIMIT 1)`,
			count,
		)
		if err != nil {
			return fmt.Errorf("increment replay progress: %w", err)
		}
		return nil
	})
}

// TruncateProjection truncates the target read-model tables for the given
// projection name. Valid projections: "timeline", "summary", "stats", "all".
func (r *Repository) TruncateProjection(ctx context.Context, projection string) error {
	return resilience.Do(ctx, r.breaker, r.retryCfg, func(ctx context.Context) error {
		var tables []string
		switch projection {
		case "timeline":
			tables = []string{"order_timeline"}
		case "summary":
			tables = []string{"order_summary"}
		case "stats":
			tables = []string{"order_stats"}
		case "all":
			tables = []string{"order_timeline", "order_summary", "order_stats"}
		default:
			return fmt.Errorf("unknown projection: %s", projection)
		}

		for _, t := range tables {
			// Table names are from a fixed set above, not user input.
			if _, err := r.pool.Exec(ctx, "TRUNCATE TABLE "+t); err != nil {
				return fmt.Errorf("truncate %s: %w", t, err)
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// GetTimeline returns all events for an order, ordered by timestamp ASC.
func (r *Repository) GetTimeline(ctx context.Context, orderID string) ([]TimelineEvent, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]TimelineEvent, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT event_id, order_id, event_type, event_version, data, timestamp
			 FROM order_timeline
			 WHERE order_id = $1
			 ORDER BY timestamp ASC`,
			orderID,
		)
		if err != nil {
			return nil, fmt.Errorf("get timeline: %w", err)
		}
		defer rows.Close()

		var events []TimelineEvent
		for rows.Next() {
			var ev TimelineEvent
			if err := rows.Scan(&ev.EventID, &ev.OrderID, &ev.EventType, &ev.EventVersion, &ev.Data, &ev.Timestamp); err != nil {
				return nil, fmt.Errorf("scan timeline event: %w", err)
			}
			events = append(events, ev)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("timeline rows: %w", err)
		}
		return events, nil
	})
}

// GetOrderSummary returns a single order summary or nil if not found.
func (r *Repository) GetOrderSummary(ctx context.Context, orderID string) (*OrderSummary, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*OrderSummary, error) {
		var s OrderSummary
		err := r.pool.QueryRow(ctx,
			`SELECT order_id, user_id, status, total_cents, currency, items,
			        created_at, updated_at, completed_at, failure_reason
			 FROM order_summary
			 WHERE order_id = $1`,
			orderID,
		).Scan(&s.OrderID, &s.UserID, &s.Status, &s.TotalCents, &s.Currency, &s.Items,
			&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.FailureReason)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil //nolint:nilnil // nil means not found, not an error
			}
			return nil, fmt.Errorf("get order summary: %w", err)
		}
		return &s, nil
	})
}

// ListOrderSummaries returns a paginated list ordered by updated_at DESC.
func (r *Repository) ListOrderSummaries(ctx context.Context, limit, offset int) ([]OrderSummary, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]OrderSummary, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT order_id, user_id, status, total_cents, currency, items,
			        created_at, updated_at, completed_at, failure_reason
			 FROM order_summary
			 ORDER BY updated_at DESC
			 LIMIT $1 OFFSET $2`,
			limit, offset,
		)
		if err != nil {
			return nil, fmt.Errorf("list order summaries: %w", err)
		}
		defer rows.Close()

		var summaries []OrderSummary
		for rows.Next() {
			var s OrderSummary
			if err := rows.Scan(&s.OrderID, &s.UserID, &s.Status, &s.TotalCents, &s.Currency, &s.Items,
				&s.CreatedAt, &s.UpdatedAt, &s.CompletedAt, &s.FailureReason); err != nil {
				return nil, fmt.Errorf("scan order summary: %w", err)
			}
			summaries = append(summaries, s)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("summary rows: %w", err)
		}
		return summaries, nil
	})
}

// GetOrderStats returns hourly stats since the given time, limited to limit rows.
func (r *Repository) GetOrderStats(ctx context.Context, since time.Time, limit int) ([]OrderStats, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) ([]OrderStats, error) {
		rows, err := r.pool.Query(ctx,
			`SELECT hour_bucket, orders_created, orders_completed, orders_failed,
			        avg_completion_seconds, total_revenue_cents
			 FROM order_stats
			 WHERE hour_bucket >= $1
			 ORDER BY hour_bucket ASC
			 LIMIT $2`,
			since, limit,
		)
		if err != nil {
			return nil, fmt.Errorf("get order stats: %w", err)
		}
		defer rows.Close()

		var stats []OrderStats
		for rows.Next() {
			var s OrderStats
			if err := rows.Scan(&s.HourBucket, &s.OrdersCreated, &s.OrdersCompleted, &s.OrdersFailed,
				&s.AvgCompletionSeconds, &s.TotalRevenueCents); err != nil {
				return nil, fmt.Errorf("scan order stats: %w", err)
			}
			stats = append(stats, s)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("stats rows: %w", err)
		}
		return stats, nil
	})
}

// GetReplayStatus returns the latest replay record, or nil if none exist.
func (r *Repository) GetReplayStatus(ctx context.Context) (*ReplayStatus, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*ReplayStatus, error) {
		var s ReplayStatus
		err := r.pool.QueryRow(ctx,
			`SELECT is_replaying, projection, started_at, completed_at, events_processed, total_events
			 FROM replay_status
			 ORDER BY started_at DESC
			 LIMIT 1`,
		).Scan(&s.IsReplaying, &s.Projection, &s.StartedAt, &s.CompletedAt, &s.EventsProcessed, &s.TotalEvents)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil //nolint:nilnil // nil means no replay history
			}
			return nil, fmt.Errorf("get replay status: %w", err)
		}
		return &s, nil
	})
}

// LatestEventTimestamp returns the maximum timestamp from the timeline table,
// or nil if the table is empty.
func (r *Repository) LatestEventTimestamp(ctx context.Context) (*time.Time, error) {
	return resilience.Call(ctx, r.breaker, r.retryCfg, func(ctx context.Context) (*time.Time, error) {
		var t *time.Time
		err := r.pool.QueryRow(ctx,
			`SELECT MAX(timestamp) FROM order_timeline`,
		).Scan(&t)
		if err != nil {
			return nil, fmt.Errorf("latest event timestamp: %w", err)
		}
		return t, nil
	})
}
