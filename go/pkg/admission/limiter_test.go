package admission

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingObserver struct {
	inFlight []int
	rejected int
	released int
	admitted int
}

func (r *recordingObserver) ObserveInFlight(n int) {
	r.inFlight = append(r.inFlight, n)
}

func (r *recordingObserver) ObserveAdmitted() {
	r.admitted++
}

func (r *recordingObserver) ObserveRejected() {
	r.rejected++
}

func (r *recordingObserver) ObserveReleased() {
	r.released++
}

func TestTryAcquireRejectsWhenFullAndReleaseRestoresCapacity(t *testing.T) {
	obs := &recordingObserver{}
	limiter, err := NewLimiter(2, WithObserver(obs))
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	first, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("first acquire rejected")
	}
	second, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("second acquire rejected")
	}
	if _, ok := limiter.TryAcquire(context.Background()); ok {
		t.Fatal("third acquire succeeded while limiter was full")
	}
	if limiter.InFlight() != 2 {
		t.Fatalf("in-flight = %d, want 2", limiter.InFlight())
	}

	first.Release()
	if limiter.InFlight() != 1 {
		t.Fatalf("in-flight after release = %d, want 1", limiter.InFlight())
	}
	third, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("acquire after release rejected")
	}

	second.Release()
	third.Release()
	if limiter.InFlight() != 0 {
		t.Fatalf("in-flight after all releases = %d, want 0", limiter.InFlight())
	}
	if obs.admitted != 3 {
		t.Fatalf("admitted observations = %d, want 3", obs.admitted)
	}
	if obs.rejected != 1 {
		t.Fatalf("rejected observations = %d, want 1", obs.rejected)
	}
	if obs.released != 3 {
		t.Fatalf("released observations = %d, want 3", obs.released)
	}
}

func TestPermitReleaseIsIdempotent(t *testing.T) {
	limiter, err := NewLimiter(1)
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}

	permit, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("acquire rejected")
	}

	permit.Release()
	permit.Release()

	if limiter.InFlight() != 0 {
		t.Fatalf("in-flight = %d, want 0", limiter.InFlight())
	}
	if _, ok := limiter.TryAcquire(context.Background()); !ok {
		t.Fatal("acquire rejected after double release")
	}
}

func TestAcquireReturnsWhenContextCancelledWithoutLeakingPermit(t *testing.T) {
	limiter, err := NewLimiter(1)
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	permit, ok := limiter.TryAcquire(context.Background())
	if !ok {
		t.Fatal("initial acquire rejected")
	}
	defer permit.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = limiter.Acquire(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Acquire err = %v, want context deadline", err)
	}
	if limiter.InFlight() != 1 {
		t.Fatalf("in-flight = %d, want 1", limiter.InFlight())
	}
}

func TestNewLimiterRejectsInvalidMax(t *testing.T) {
	for _, max := range []int{0, -1} {
		if _, err := NewLimiter(max); err == nil {
			t.Fatalf("NewLimiter(%d) succeeded, want error", max)
		}
	}
}
