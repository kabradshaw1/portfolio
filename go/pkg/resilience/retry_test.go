package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_SucceedsFirstTry(t *testing.T) {
	cfg := DefaultRetryConfig()
	calls := 0
	result, err := Retry(context.Background(), cfg, func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %q", result)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetry_RetriesUpToMax(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	calls := 0
	boom := errors.New("boom")
	_, err := Retry(context.Background(), cfg, func(ctx context.Context) (int, error) {
		calls++
		return 0, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 10, BaseDelay: time.Second, MaxDelay: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	_, err := Retry(ctx, cfg, func(ctx context.Context) (int, error) {
		calls++
		return 0, errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if calls > 3 {
		t.Errorf("too many calls before cancel: %d", calls)
	}
}

func TestRetry_NonRetryableStopsEarly(t *testing.T) {
	permanent := errors.New("permanent")
	cfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   time.Millisecond,
		MaxDelay:    time.Millisecond,
		IsRetryable: func(err error) bool {
			return !errors.Is(err, permanent)
		},
	}
	calls := 0
	_, err := Retry(context.Background(), cfg, func(ctx context.Context) (int, error) {
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

func TestRetry_SucceedsOnSecondAttempt(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	calls := 0
	result, err := Retry(context.Background(), cfg, func(ctx context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("transient")
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("result = %q", result)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}
