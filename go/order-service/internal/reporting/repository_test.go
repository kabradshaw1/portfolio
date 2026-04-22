package reporting_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
	"github.com/kabradshaw1/portfolio/go/pkg/resilience"
)

func TestReportingRepository_SalesTrends_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.SalesTrends(context.Background(), 30)
	assert.Error(t, err)
}

func TestReportingRepository_InventoryTurnover_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.InventoryTurnover(context.Background(), 30, 20)
	assert.Error(t, err)
}

func TestReportingRepository_TopCustomers_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.TopCustomers(context.Background(), 10)
	assert.Error(t, err)
}

func TestReportingRepository_ProductPerformance_NilPool(t *testing.T) {
	breaker := resilience.NewBreaker(resilience.BreakerConfig{Name: "test"})
	repo := reporting.NewRepository(nil, breaker)
	_, err := repo.ProductPerformance(context.Background())
	assert.Error(t, err)
}
