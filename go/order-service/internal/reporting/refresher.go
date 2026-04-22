package reporting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var views = []string{
	"mv_daily_revenue",
	"mv_product_performance",
	"mv_customer_summary",
}

// MaterializedViews returns the list of managed materialized view names.
func MaterializedViews() []string {
	return views
}

// Refresher periodically refreshes materialized views concurrently.
type Refresher struct {
	pool     *pgxpool.Pool
	interval time.Duration
}

func NewRefresher(pool *pgxpool.Pool, interval time.Duration) *Refresher {
	return &Refresher{pool: pool, interval: interval}
}

// Run starts the refresh loop. Blocks until context is cancelled.
func (r *Refresher) Run(ctx context.Context) {
	// Initial refresh on startup
	r.refreshAll(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshAll(ctx)
		}
	}
}

func (r *Refresher) refreshAll(ctx context.Context) {
	for _, view := range views {
		start := time.Now()
		sql := fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", view)
		if _, err := r.pool.Exec(ctx, sql); err != nil {
			slog.ErrorContext(ctx, "refresh materialized view failed", "view", view, "error", err)
			continue
		}
		slog.InfoContext(ctx, "refreshed materialized view", "view", view, "duration", time.Since(start).String())
	}
}
