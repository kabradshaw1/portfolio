package validate_test

import (
	"strings"
	"testing"

	"github.com/kabradshaw1/portfolio/go/ecommerce-service/internal/validate"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fieldNames extracts just the field names from a slice of FieldError.
func fieldNames(errs []apperror.FieldError) []string {
	names := make([]string, len(errs))
	for i, e := range errs {
		names[i] = e.Field
	}
	return names
}

// ─── ProductListParams ────────────────────────────────────────────────────────

func TestProductListParams_Valid(t *testing.T) {
	assert.Empty(t, validate.ProductListParams("created_at_desc", 1, 10))
	assert.Empty(t, validate.ProductListParams("price_asc", 5, 100))
	assert.Empty(t, validate.ProductListParams("price_desc", 1, 1))
	assert.Empty(t, validate.ProductListParams("name_asc", 1, 50))
}

func TestProductListParams_EmptySortIsValid(t *testing.T) {
	// Empty sort means "no sort specified" — not an error.
	assert.Empty(t, validate.ProductListParams("", 1, 10))
}

func TestProductListParams_InvalidSort(t *testing.T) {
	errs := validate.ProductListParams("bad_sort", 1, 10)
	require.Len(t, errs, 1)
	assert.Equal(t, "sort", errs[0].Field)
}

func TestProductListParams_PageZero(t *testing.T) {
	errs := validate.ProductListParams("", 0, 10)
	require.Len(t, errs, 1)
	assert.Equal(t, "page", errs[0].Field)
}

func TestProductListParams_PageNegative(t *testing.T) {
	errs := validate.ProductListParams("", -1, 10)
	require.Len(t, errs, 1)
	assert.Equal(t, "page", errs[0].Field)
}

func TestProductListParams_LimitZero(t *testing.T) {
	errs := validate.ProductListParams("", 1, 0)
	require.Len(t, errs, 1)
	assert.Equal(t, "limit", errs[0].Field)
}

func TestProductListParams_LimitOver100(t *testing.T) {
	errs := validate.ProductListParams("", 1, 101)
	require.Len(t, errs, 1)
	assert.Equal(t, "limit", errs[0].Field)
}

func TestProductListParams_LimitBoundaryValid(t *testing.T) {
	assert.Empty(t, validate.ProductListParams("", 1, 1))
	assert.Empty(t, validate.ProductListParams("", 1, 100))
}

func TestProductListParams_MultipleErrors(t *testing.T) {
	errs := validate.ProductListParams("invalid", 0, 0)
	require.Len(t, errs, 3)
	assert.ElementsMatch(t, []string{"sort", "page", "limit"}, fieldNames(errs))
}

// ─── InitiateReturn ───────────────────────────────────────────────────────────

func TestInitiateReturn_Valid(t *testing.T) {
	errs := validate.InitiateReturn([]string{"item-1", "item-2"}, "Product damaged on arrival")
	assert.Empty(t, errs)
}

func TestInitiateReturn_EmptyItemIDs(t *testing.T) {
	errs := validate.InitiateReturn([]string{}, "Damaged")
	require.Len(t, errs, 1)
	assert.Equal(t, "itemIds", errs[0].Field)
}

func TestInitiateReturn_NilItemIDs(t *testing.T) {
	errs := validate.InitiateReturn(nil, "Damaged")
	require.Len(t, errs, 1)
	assert.Equal(t, "itemIds", errs[0].Field)
}

func TestInitiateReturn_EmptyReason(t *testing.T) {
	errs := validate.InitiateReturn([]string{"item-1"}, "")
	require.Len(t, errs, 1)
	assert.Equal(t, "reason", errs[0].Field)
}

func TestInitiateReturn_ReasonTooLong(t *testing.T) {
	longReason := strings.Repeat("x", 501)
	errs := validate.InitiateReturn([]string{"item-1"}, longReason)
	require.Len(t, errs, 1)
	assert.Equal(t, "reason", errs[0].Field)
}

func TestInitiateReturn_ReasonExactlyMaxLen(t *testing.T) {
	maxReason := strings.Repeat("x", 500)
	assert.Empty(t, validate.InitiateReturn([]string{"item-1"}, maxReason))
}

func TestInitiateReturn_MultipleErrors(t *testing.T) {
	errs := validate.InitiateReturn(nil, "")
	require.Len(t, errs, 2)
	assert.ElementsMatch(t, []string{"itemIds", "reason"}, fieldNames(errs))
}
