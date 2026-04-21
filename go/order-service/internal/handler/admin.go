package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/saga"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// DLQLister abstracts the DLQ read operations for testability.
type DLQLister interface {
	List(limit int) ([]saga.DLQMessage, error)
	Replay(index int) (*saga.DLQMessage, error)
}

// AdminHandler exposes DLQ inspection and replay endpoints.
type AdminHandler struct {
	dlq DLQLister
}

// NewAdminHandler creates an admin handler.
func NewAdminHandler(dlq DLQLister) *AdminHandler {
	return &AdminHandler{dlq: dlq}
}

// ListDLQ returns messages currently in the dead-letter queue.
func (h *AdminHandler) ListDLQ(c *gin.Context) {
	limit := 50
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && l > 0 {
		limit = l
	}

	messages, err := h.dlq.List(limit)
	if err != nil {
		_ = c.Error(apperror.Internal("DLQ_LIST_FAILED", err.Error()))
		return
	}

	if messages == nil {
		messages = []saga.DLQMessage{}
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages, "count": len(messages)})
}

// ReplayDLQ replays a single message from the dead-letter queue.
func (h *AdminHandler) ReplayDLQ(c *gin.Context) {
	var req struct {
		Index int `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.BadRequest("INVALID_BODY", "request body must contain index"))
		return
	}

	msg, err := h.dlq.Replay(req.Index)
	if err != nil {
		_ = c.Error(apperror.NotFound("DLQ_MESSAGE_NOT_FOUND", err.Error()))
		return
	}

	c.JSON(http.StatusOK, gin.H{"replayed": msg})
}
