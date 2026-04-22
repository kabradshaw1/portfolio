package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/handler"
	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

type mockReportingRepo struct {
	salesTrends []reporting.SalesTrend
	inventory   []reporting.InventoryTurnover
	customers   []reporting.CustomerSummary
	products    []reporting.ProductPerformance
	err         error
}

func (m *mockReportingRepo) SalesTrends(_ context.Context, _ int) ([]reporting.SalesTrend, error) {
	return m.salesTrends, m.err
}

func (m *mockReportingRepo) InventoryTurnover(_ context.Context, _, _ int) ([]reporting.InventoryTurnover, error) {
	return m.inventory, m.err
}

func (m *mockReportingRepo) TopCustomers(_ context.Context, _ int) ([]reporting.CustomerSummary, error) {
	return m.customers, m.err
}

func (m *mockReportingRepo) ProductPerformance(_ context.Context) ([]reporting.ProductPerformance, error) {
	return m.products, m.err
}

func setupReportingRouter(h *handler.ReportingHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(apperror.ErrorHandler())
	r.GET("/reporting/sales-trends", h.SalesTrends)
	r.GET("/reporting/inventory-turnover", h.InventoryTurnover)
	r.GET("/reporting/top-customers", h.TopCustomers)
	r.GET("/reporting/product-performance", h.ProductPerformance)
	return r
}

func TestReportingHandler_SalesTrends(t *testing.T) {
	h := handler.NewReportingHandler(&mockReportingRepo{
		salesTrends: []reporting.SalesTrend{},
	})
	router := setupReportingRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reporting/sales-trends?days=30", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReportingHandler_SalesTrends_DefaultDays(t *testing.T) {
	h := handler.NewReportingHandler(&mockReportingRepo{
		salesTrends: []reporting.SalesTrend{},
	})
	router := setupReportingRouter(h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/reporting/sales-trends", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
