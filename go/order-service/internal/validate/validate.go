package validate

import (
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

const (
	minPage      = 1
	minLimit     = 1
	maxLimit     = 100
	maxReasonLen = 500

	sortCreatedAtDesc = "created_at_desc"
	sortPriceAsc      = "price_asc"
	sortPriceDesc     = "price_desc"
	sortNameAsc       = "name_asc"
)

var validSortValues = map[string]bool{
	sortCreatedAtDesc: true,
	sortPriceAsc:      true,
	sortPriceDesc:     true,
	sortNameAsc:       true,
}

// ProductListParams validates product list query parameters.
func ProductListParams(sort string, page, limit int) []apperror.FieldError {
	var errs []apperror.FieldError

	if sort != "" && !validSortValues[sort] {
		errs = append(errs, apperror.FieldError{
			Field:   "sort",
			Message: "must be one of: created_at_desc, price_asc, price_desc, name_asc",
		})
	}

	if page < minPage {
		errs = append(errs, apperror.FieldError{Field: "page", Message: "must be at least 1"})
	}

	if limit < minLimit || limit > maxLimit {
		errs = append(errs, apperror.FieldError{Field: "limit", Message: "must be between 1 and 100"})
	}

	return errs
}

// InitiateReturn validates the initiate-return request fields.
func InitiateReturn(itemIDs []string, reason string) []apperror.FieldError {
	var errs []apperror.FieldError

	if len(itemIDs) == 0 {
		errs = append(errs, apperror.FieldError{Field: "itemIds", Message: "must not be empty"})
	}

	if len(reason) == 0 || len(reason) > maxReasonLen {
		errs = append(errs, apperror.FieldError{Field: "reason", Message: "must be between 1 and 500 characters"})
	}

	return errs
}
