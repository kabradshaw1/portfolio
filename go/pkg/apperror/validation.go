package apperror

import "net/http"

// FieldError describes a single field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validation returns a 422 AppError with field-level detail.
func Validation(fields []FieldError) *AppError {
	return &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    "validation failed",
		HTTPStatus: http.StatusUnprocessableEntity,
		Fields:     fields,
	}
}
