package resilience

import (
	"context"
	"errors"

	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

// Do is like Call but for functions that don't return a value.
func Do(ctx context.Context, cb *gobreaker.CircuitBreaker[any], cfg RetryConfig, fn func(ctx context.Context) error) error {
	_, err := Call(ctx, cb, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// Call wraps fn with retry and circuit breaker. Each retry attempt goes
// through the breaker. If the breaker is open, the error is returned
// immediately without retrying.
func Call[T any](ctx context.Context, cb *gobreaker.CircuitBreaker[any], cfg RetryConfig, fn func(ctx context.Context) (T, error)) (T, error) {
	// Wrap the IsRetryable to also skip retries on breaker-open errors.
	origRetryable := cfg.IsRetryable
	cfg.IsRetryable = func(err error) bool {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return false
		}
		if origRetryable != nil {
			return origRetryable(err)
		}
		return true
	}

	return Retry(ctx, cfg, func(ctx context.Context) (T, error) {
		result, err := cb.Execute(func() (any, error) {
			return fn(ctx)
		})
		if err != nil {
			if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
				var zero T
				return zero, apperror.Wrap(err, "CIRCUIT_OPEN", "dependency temporarily unavailable: "+cb.Name(), 503)
			}
			var zero T
			return zero, err
		}
		return result.(T), nil
	})
}
