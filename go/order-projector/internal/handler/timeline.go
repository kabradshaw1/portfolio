package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// TimelineHandler serves the order event timeline read model.
type TimelineHandler struct {
	repo *repository.Repository
}

// NewTimelineHandler creates a TimelineHandler.
func NewTimelineHandler(repo *repository.Repository) *TimelineHandler {
	return &TimelineHandler{repo: repo}
}

// GetTimeline returns all events for a given order, ordered by timestamp ASC.
func (h *TimelineHandler) GetTimeline(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		_ = c.Error(apperror.BadRequest("MISSING_ORDER_ID", "order ID is required"))
		return
	}

	events, err := h.repo.GetTimeline(c.Request.Context(), orderID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if events == nil {
		events = []repository.TimelineEvent{}
	}

	c.JSON(http.StatusOK, gin.H{"events": events})
}
