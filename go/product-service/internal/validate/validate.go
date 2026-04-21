package validate

import "github.com/kabradshaw1/portfolio/go/pkg/apperror"

const (
	minPage  = 1
	minLimit = 1
	maxLimit = 100
)

var validSortValues = map[string]bool{
	"created_at_desc": true,
	"price_asc":       true,
	"price_desc":      true,
	"name_asc":        true,
}

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
