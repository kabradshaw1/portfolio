package saga

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// IncompleteOrderFinder queries for orders with non-terminal saga steps.
type IncompleteOrderFinder interface {
	FindIncompleteSagas(ctx context.Context) ([]uuid.UUID, error)
}

// RecoverIncomplete resumes incomplete sagas on startup.
func RecoverIncomplete(ctx context.Context, finder IncompleteOrderFinder, orch *Orchestrator) {
	orderIDs, err := finder.FindIncompleteSagas(ctx)
	if err != nil {
		slog.Error("saga recovery: failed to find incomplete orders", "error", err)
		return
	}

	if len(orderIDs) == 0 {
		slog.Info("saga recovery: no incomplete sagas found")
		return
	}

	slog.Info("saga recovery: resuming incomplete sagas", "count", len(orderIDs))
	for _, id := range orderIDs {
		if err := orch.Advance(ctx, id); err != nil {
			slog.Error("saga recovery: failed to resume", "orderID", id, "error", err)
		}
	}
}
