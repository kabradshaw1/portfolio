package reporting_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/reporting"
)

func TestNewRefresher(t *testing.T) {
	r := reporting.NewRefresher(nil, 15*time.Minute)
	assert.NotNil(t, r)
}

func TestMaterializedViewNames(t *testing.T) {
	views := reporting.MaterializedViews()
	assert.Contains(t, views, "mv_daily_revenue")
	assert.Contains(t, views, "mv_product_performance")
	assert.Contains(t, views, "mv_customer_summary")
	assert.Len(t, views, 3)
}
