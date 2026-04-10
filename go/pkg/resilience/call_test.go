package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)

func TestCall_SuccessPassesThrough(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test-call"})
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	result, err := Call(context.Background(), cb, cfg, func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q", result)
	}
}

func TestCall_RetriesTransientErrors(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test-retry", MaxFailures: 10})
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	calls := 0
	result, err := Call(context.Background(), cb, cfg, func(ctx context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q", result)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestCall_BreakerOpenNotRetried(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test-open", MaxFailures: 2, HalfOpenWindow: time.Second})
	cfg := RetryConfig{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	// Trip the breaker.
	for range 2 {
		_, _ = Call(context.Background(), cb, cfg, func(ctx context.Context) (int, error) {
			return 0, errors.New("fail")
		})
	}

	// Now the breaker is open — Call should return immediately, not retry.
	calls := 0
	_, err := Call(context.Background(), cb, cfg, func(ctx context.Context) (int, error) {
		calls++
		return 42, nil
	})
	if err == nil {
		t.Fatal("expected error from open breaker")
	}

	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperror.AppError, got %T: %v", err, err)
	}
	if ae.Code != "CIRCUIT_OPEN" {
		t.Errorf("code = %q", ae.Code)
	}
	if calls != 0 {
		t.Errorf("fn was called %d times, want 0 (breaker should block)", calls)
	}
}

func TestCall_RespectsIsRetryable(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test-noretry", MaxFailures: 10})
	permanent := errors.New("permanent")
	cfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		MaxDelay:    time.Millisecond,
		IsRetryable: func(err error) bool { return !errors.Is(err, permanent) },
	}

	calls := 0
	_, err := Call(context.Background(), cb, cfg, func(ctx context.Context) (int, error) {
		calls++
		return 0, permanent
	})
	if !errors.Is(err, permanent) {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// Verify breaker-open error is detected as non-retryable even when wrapped.
func TestCall_BreakerOpenErrorWrappedInAppError(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test-wrap", MaxFailures: 1, HalfOpenWindow: time.Second})

	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

	// Trip breaker.
	_, _ = Call(context.Background(), cb, cfg, func(ctx context.Context) (int, error) {
		return 0, errors.New("fail")
	})

	// Should get CIRCUIT_OPEN AppError.
	_, err := Call(context.Background(), cb, cfg, func(ctx context.Context) (int, error) {
		return 0, nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		// The error should unwrap to gobreaker.ErrOpenState through AppError.Unwrap().
		var ae *apperror.AppError
		if !errors.As(err, &ae) {
			t.Fatalf("expected AppError, got %T", err)
		}
		if ae.Code != "CIRCUIT_OPEN" {
			t.Errorf("code = %q", ae.Code)
		}
	}
}
