package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// ReplayStarter triggers a projection replay operation.
// Implemented by the replay package (created in a later task).
type ReplayStarter interface {
	Start(ctx context.Context, projection string) error
}

// ReplayHandler exposes an endpoint to trigger projection replays.
type ReplayHandler struct {
	replayer ReplayStarter
}

// NewReplayHandler creates a ReplayHandler.
func NewReplayHandler(replayer ReplayStarter) *ReplayHandler {
	return &ReplayHandler{replayer: replayer}
}

// validProjections is the set of allowed projection names for replay.
var validProjections = map[string]bool{
	"all":      true,
	"timeline": true,
	"summary":  true,
	"stats":    true,
}

// TriggerReplay starts a projection replay. Accepts an optional "projection"
// query parameter (default "all"). Returns 202 Accepted on success.
func (h *ReplayHandler) TriggerReplay(c *gin.Context) {
	projection := c.DefaultQuery("projection", "all")

	if !validProjections[projection] {
		_ = c.Error(apperror.BadRequest("INVALID_PROJECTION", "valid projections: all, timeline, summary, stats"))
		return
	}

	if err := h.replayer.Start(c.Request.Context(), projection); err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":    "replay started",
		"projection": projection,
	})
}
