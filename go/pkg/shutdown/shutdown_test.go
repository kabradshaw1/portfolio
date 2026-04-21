package shutdown

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunsInPriorityOrder(t *testing.T) {
	m := New(5 * time.Second)

	var order []int
	m.Register("third", 20, func(ctx context.Context) error {
		order = append(order, 20)
		return nil
	})
	m.Register("first", 0, func(ctx context.Context) error {
		order = append(order, 0)
		return nil
	})
	m.Register("second", 10, func(ctx context.Context) error {
		order = append(order, 10)
		return nil
	})

	m.runAll()

	if len(order) != 3 || order[0] != 0 || order[1] != 10 || order[2] != 20 {
		t.Fatalf("expected [0 10 20], got %v", order)
	}
}

func TestSamePriorityRunsConcurrently(t *testing.T) {
	m := New(5 * time.Second)

	var running atomic.Int32
	var maxConcurrent atomic.Int32

	for i := 0; i < 3; i++ {
		m.Register("concurrent", 10, func(ctx context.Context) error {
			n := running.Add(1)
			for {
				old := maxConcurrent.Load()
				if n <= old || maxConcurrent.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(50 * time.Millisecond)
			running.Add(-1)
			return nil
		})
	}

	m.runAll()

	if maxConcurrent.Load() < 2 {
		t.Fatalf("expected concurrent execution, max was %d", maxConcurrent.Load())
	}
}

func TestTimeoutCancelsContext(t *testing.T) {
	m := New(100 * time.Millisecond)

	var cancelled bool
	m.Register("slow", 0, func(ctx context.Context) error {
		<-ctx.Done()
		cancelled = true
		return nil
	})

	m.runAll()

	if !cancelled {
		t.Fatal("expected context to be cancelled after timeout")
	}
}

func TestErrorsAreLoggedNotFatal(t *testing.T) {
	m := New(5 * time.Second)

	m.Register("failing", 0, func(ctx context.Context) error {
		return context.DeadlineExceeded
	})
	m.Register("after-fail", 10, func(ctx context.Context) error {
		return nil
	})

	// Should not panic
	m.runAll()
}
