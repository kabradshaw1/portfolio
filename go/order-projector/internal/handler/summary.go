package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// SummaryHandler serves the denormalized order summary read model.
type SummaryHandler struct {
	repo *repository.Repository
}

// NewSummaryHandler creates a SummaryHandler.
func NewSummaryHandler(repo *repository.Repository) *SummaryHandler {
	return &SummaryHandler{repo: repo}
}

// GetOrder returns a single order summary by ID.
func (h *SummaryHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("id")
	if orderID == "" {
		_ = c.Error(apperror.BadRequest("MISSING_ORDER_ID", "order ID is required"))
		return
	}

	summary, err := h.repo.GetOrderSummary(c.Request.Context(), orderID)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if summary == nil {
		_ = c.Error(apperror.NotFound("ORDER_NOT_FOUND", "order not found"))
		return
	}

	c.JSON(http.StatusOK, summary)
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

// ListOrders returns a paginated list of order summaries.
func (h *SummaryHandler) ListOrders(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	summaries, err := h.repo.ListOrderSummaries(c.Request.Context(), limit, offset)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if summaries == nil {
		summaries = []repository.OrderSummary{}
	}

	c.JSON(http.StatusOK, gin.H{
		"orders": summaries,
		"limit":  limit,
		"offset": offset,
	})
}
