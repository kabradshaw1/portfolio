package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// ReportingRepo abstracts reporting queries.
type ReportingRepo interface {
	SalesTrends(ctx context.Context, days int) ([]reporting.SalesTrend, error)
	InventoryTurnover(ctx context.Context, days, limit int) ([]reporting.InventoryTurnover, error)
	TopCustomers(ctx context.Context, limit int) ([]reporting.CustomerSummary, error)
	ProductPerformance(ctx context.Context) ([]reporting.ProductPerformance, error)
}

type ReportingHandler struct {
	repo ReportingRepo
}

func NewReportingHandler(repo ReportingRepo) *ReportingHandler {
	return &ReportingHandler{repo: repo}
}

func (h *ReportingHandler) SalesTrends(c *gin.Context) {
	days := intQueryParam(c, "days", 30)

	trends, err := h.repo.SalesTrends(c.Request.Context(), days)
	if err != nil {
		_ = c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch sales trends"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"trends": trends})
}

func (h *ReportingHandler) InventoryTurnover(c *gin.Context) {
	days := intQueryParam(c, "days", 30)
	limit := intQueryParam(c, "limit", 20)

	items, err := h.repo.InventoryTurnover(c.Request.Context(), days, limit)
	if err != nil {
		_ = c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch inventory turnover"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"inventory": items})
}

func (h *ReportingHandler) TopCustomers(c *gin.Context) {
	limit := intQueryParam(c, "limit", 10)

	customers, err := h.repo.TopCustomers(c.Request.Context(), limit)
	if err != nil {
		_ = c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch top customers"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"customers": customers})
}

func (h *ReportingHandler) ProductPerformance(c *gin.Context) {
	products, err := h.repo.ProductPerformance(c.Request.Context())
	if err != nil {
		_ = c.Error(apperror.Internal("REPORTING_ERROR", "failed to fetch product performance"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"products": products})
}

func intQueryParam(c *gin.Context, key string, defaultVal int) int {
	if v := c.Query(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}
