package resilience

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestNewBreaker_Defaults(t *testing.T) {
	cb := NewBreaker(BreakerConfig{Name: "test"})
	if cb.Name() != "test" {
		t.Errorf("name = %q", cb.Name())
	}
	// Should allow calls when closed.
	_, err := cb.Execute(func() (any, error) { return "ok", nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewBreaker(BreakerConfig{
		Name:           "test-open",
		MaxFailures:    3,
		HalfOpenWindow: time.Second,
	})

	boom := errors.New("boom")
	for range 3 {
		_, _ = cb.Execute(func() (any, error) { return nil, boom })
	}

	// Breaker should now be open.
	_, err := cb.Execute(func() (any, error) { return "ok", nil })
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected ErrOpenState, got %v", err)
	}
}

func TestNewBreaker_StateChangeCallback(t *testing.T) {
	var changes []gobreaker.State
	cb := NewBreaker(BreakerConfig{
		Name:        "test-callback",
		MaxFailures: 2,
		OnStateChange: func(name string, from, to gobreaker.State) {
			changes = append(changes, to)
		},
	})

	boom := errors.New("boom")
	for range 2 {
		_, _ = cb.Execute(func() (any, error) { return nil, boom })
	}

	if len(changes) == 0 {
		t.Fatal("expected state change callback")
	}
	if changes[0] != gobreaker.StateOpen {
		t.Errorf("expected transition to Open, got %v", changes[0])
	}
}

func TestObserveStateChange(t *testing.T) {
	// Just verify it doesn't panic.
	ObserveStateChange("test", gobreaker.StateClosed, gobreaker.StateOpen)
	ObserveStateChange("test", gobreaker.StateOpen, gobreaker.StateHalfOpen)
	ObserveStateChange("test", gobreaker.StateHalfOpen, gobreaker.StateClosed)
}
