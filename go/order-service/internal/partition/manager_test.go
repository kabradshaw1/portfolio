package partition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kabradshaw1/portfolio/go/order-service/internal/partition"
)

func TestPartitionName(t *testing.T) {
	tests := []struct {
		date     time.Time
		expected string
	}{
		{time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), "orders_2026_04"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "orders_2027_01"},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "orders_2026_12"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			name := partition.Name(tt.date)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestCreatePartitionSQL(t *testing.T) {
	sql := partition.CreateSQL(time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC))
	assert.Contains(t, sql, "orders_2027_07")
	assert.Contains(t, sql, "2027-07-01")
	assert.Contains(t, sql, "2027-08-01")
}
