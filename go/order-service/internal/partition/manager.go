package partition

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const monthsAhead = 3 // Create partitions 3 months in advance

// Name returns the partition table name for a given date.
func Name(t time.Time) string {
	return fmt.Sprintf("orders_%04d_%02d", t.Year(), t.Month())
}

// CreateSQL returns the DDL to create a monthly partition for the given date.
func CreateSQL(t time.Time) string {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	name := Name(start)

	return fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF orders FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
}

// EnsureFuturePartitions creates partitions for the next N months from now.
func EnsureFuturePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	now := time.Now().UTC()
	for i := 0; i <= monthsAhead; i++ {
		target := now.AddDate(0, i, 0)
		sql := CreateSQL(target)
		if _, err := pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("create partition %s: %w", Name(target), err)
		}
		slog.InfoContext(ctx, "ensured partition exists", "partition", Name(target))
	}
	return nil
}

// RunMaintenance starts a background goroutine that creates future partitions daily.
func RunMaintenance(ctx context.Context, pool *pgxpool.Pool) {
	// Create partitions on startup
	if err := EnsureFuturePartitions(ctx, pool); err != nil {
		slog.ErrorContext(ctx, "initial partition maintenance failed", "error", err)
	}

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := EnsureFuturePartitions(ctx, pool); err != nil {
					slog.ErrorContext(ctx, "partition maintenance failed", "error", err)
				}
			}
		}
	}()
}
