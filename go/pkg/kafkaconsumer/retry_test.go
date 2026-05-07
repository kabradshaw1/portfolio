package kafkaconsumer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryEventuallySucceeds(t *testing.T) {
	t.Parallel()

	attempts := 0
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond}
	err := Retry(context.Background(), cfg, func(context.Context) error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestRetryStopsOnNonRetryableError(t *testing.T) {
	t.Parallel()

	attempts := 0
	errBadRecord := errors.New("bad record")
	cfg := RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   time.Nanosecond,
		MaxDelay:    time.Nanosecond,
		IsRetryable: func(err error) bool { return !errors.Is(err, errBadRecord) },
	}
	err := Retry(context.Background(), cfg, func(context.Context) error {
		attempts++
		return errBadRecord
	})
	if !errors.Is(err, errBadRecord) {
		t.Fatalf("error = %v, want bad record", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
