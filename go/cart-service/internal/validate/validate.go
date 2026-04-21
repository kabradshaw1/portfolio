package validate

import (
	"github.com/google/uuid"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

const (
	minQuantity = 1
	maxQuantity = 99
)

func AddToCart(productID string, quantity int) []apperror.FieldError {
	var errs []apperror.FieldError

	if productID == "" {
		errs = append(errs, apperror.FieldError{Field: "productId", Message: "required"})
	} else if _, err := uuid.Parse(productID); err != nil {
		errs = append(errs, apperror.FieldError{Field: "productId", Message: "must be a valid UUID"})
	}

	if quantity < minQuantity || quantity > maxQuantity {
		errs = append(errs, apperror.FieldError{Field: "quantity", Message: "must be between 1 and 99"})
	}

	return errs
}

func UpdateCart(quantity int) []apperror.FieldError {
	var errs []apperror.FieldError

	if quantity < minQuantity || quantity > maxQuantity {
		errs = append(errs, apperror.FieldError{Field: "quantity", Message: "must be between 1 and 99"})
	}

	return errs
}
