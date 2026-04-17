package apperror

import (
	"errors"
	"net/http"
)

// AppError is a structured application error with an HTTP status code and
// machine-readable error code. It satisfies the error interface.
type AppError struct {
	Code       string       `json:"code"`
	Message    string       `json:"message"`
	HTTPStatus int          `json:"-"`
	Err        error        `json:"-"`
	Fields     []FieldError `json:"-"`
}

func (e *AppError) Error() string { return e.Message }

func (e *AppError) Unwrap() error { return e.Err }

// ErrorBody is the nested JSON shape inside the error response.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorResponse is the top-level JSON envelope returned to clients.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ValidationErrorBody extends ErrorBody with field-level validation details.
type ValidationErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields"`
}

// ValidationErrorResponse is the top-level JSON envelope for 422 responses.
type ValidationErrorResponse struct {
	Error ValidationErrorBody `json:"error"`
}

// --- Constructors ---

func NotFound(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusNotFound}
}

func BadRequest(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusBadRequest}
}

func Unauthorized(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusUnauthorized}
}

func Forbidden(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusForbidden}
}

func Conflict(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusConflict}
}

func Internal(code, message string) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: http.StatusInternalServerError}
}

// Wrap creates an AppError with an underlying cause.
func Wrap(err error, code, message string, status int) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: status, Err: err}
}

// Is extracts an *AppError from err using errors.As.
func Is(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
