package resilience

import (
	"context"
	"math"
	"math/rand/v2"
	"time"
)

// RetryConfig controls retry behavior.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	IsRetryable func(error) bool // optional — if nil, all errors are retried
}

// DefaultRetryConfig returns sensible defaults: 3 attempts, 100ms base, 2s max.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    2 * time.Second,
	}
}

// Retry executes fn up to cfg.MaxAttempts times with exponential backoff and jitter.
// It respects context cancellation between attempts.
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := range cfg.MaxAttempts {
		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if cfg.IsRetryable != nil && !cfg.IsRetryable(err) {
			return zero, err
		}

		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := backoff(cfg.BaseDelay, cfg.MaxDelay, attempt)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	return zero, lastErr
}

func backoff(base, max time.Duration, attempt int) time.Duration {
	exp := time.Duration(float64(base) * math.Pow(2, float64(attempt)))
	jitter := time.Duration(rand.Int64N(int64(base)))
	d := exp + jitter
	if d > max {
		d = max
	}
	return d
}
