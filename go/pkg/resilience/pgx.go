package resilience

import (
	"strings"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// IsPgRetryable returns true for connection/network errors, false for
// constraint violations, no-rows, and domain errors that shouldn't be retried.
func IsPgRetryable(err error) bool {
	if err == nil {
		return false
	}
	// AppError types are domain errors — never retry.
	if _, ok := apperror.Is(err); ok {
		return false
	}
	msg := err.Error()
	for _, s := range []string{"no rows", "duplicate key", "violates"} {
		if strings.Contains(msg, s) {
			return false
		}
	}
	return true
}
