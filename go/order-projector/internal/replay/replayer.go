package replay

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

// OffsetResetter can reset a Kafka consumer's offset to earliest.
type OffsetResetter interface {
	ResetOffset() error
}

type Replayer struct {
	repo     *repository.Repository
	consumer OffsetResetter
}

func New(repo *repository.Repository, consumer OffsetResetter) *Replayer {
	return &Replayer{repo: repo, consumer: consumer}
}

func (r *Replayer) Start(ctx context.Context, projection string) error {
	// 1. Validate projection: "timeline", "summary", "stats", "all" — else error
	validProjections := map[string]bool{
		"timeline": true,
		"summary":  true,
		"stats":    true,
		"all":      true,
	}
	if !validProjections[projection] {
		return fmt.Errorf("unknown projection: %s", projection)
	}

	// 2. Log "starting replay"
	slog.Info("starting replay", "projection", projection)

	// 3. Call r.repo.StartReplay(ctx, projection) — record in DB
	if err := r.repo.StartReplay(ctx, projection); err != nil {
		return fmt.Errorf("failed to start replay: %w", err)
	}

	// 4. Call r.repo.TruncateProjection(ctx, projection) — clear target tables
	if err := r.repo.TruncateProjection(ctx, projection); err != nil {
		return fmt.Errorf("failed to truncate projection: %w", err)
	}

	// 5. Call r.consumer.ResetOffset() — reset to earliest (log error but don't fail)
	if err := r.consumer.ResetOffset(); err != nil {
		slog.Error("failed to reset offset", "error", err)
	}

	// 6. Return nil — consumer's Run loop will rebuild from earliest
	return nil
}
