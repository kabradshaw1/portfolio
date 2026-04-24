package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-projector/internal/repository"
)

const (
	defaultHours = 24
	maxStatsRows = 168 // 7 days of hourly buckets
)

// StatsHandler serves the hourly order statistics read model.
type StatsHandler struct {
	repo *repository.Repository
}

// NewStatsHandler creates a StatsHandler.
func NewStatsHandler(repo *repository.Repository) *StatsHandler {
	return &StatsHandler{repo: repo}
}

// GetOrderStats returns hourly order stats for the given time window.
func (h *StatsHandler) GetOrderStats(c *gin.Context) {
	hours, err := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if err != nil || hours < 1 {
		hours = defaultHours
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	stats, err := h.repo.GetOrderStats(c.Request.Context(), since, maxStatsRows)
	if err != nil {
		_ = c.Error(err)
		return
	}

	if stats == nil {
		stats = []repository.OrderStats{}
	}

	c.JSON(http.StatusOK, gin.H{
		"stats": stats,
		"hours": hours,
	})
}
