package shutdown

import (
	"context"
	"net"
	"net/http"
	"sync"
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

func TestDrainHTTPCompletesInflightRequests(t *testing.T) {
	// Handler that takes 500ms to respond — simulates in-flight work.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Handler: handler}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()

	addr := "http://" + ln.Addr().String()

	// Start an in-flight request.
	var resp *http.Response
	var reqErr error
	reqDone := make(chan struct{})
	go func() {
		resp, reqErr = http.Get(addr)
		close(reqDone)
	}()

	// Give the request time to reach the handler.
	time.Sleep(50 * time.Millisecond)

	// Track shutdown hook execution order.
	var order []string
	var mu sync.Mutex
	record := func(name string) {
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}

	m := New(5 * time.Second)
	m.Register("drain-http", 0, func(ctx context.Context) error {
		err := DrainHTTP("test-http", srv)(ctx)
		record("drain-http")
		return err
	})
	m.Register("close-pool", 20, func(ctx context.Context) error {
		record("close-pool")
		return nil
	})
	m.Register("flush-otel", 30, func(ctx context.Context) error {
		record("flush-otel")
		return nil
	})

	// Trigger shutdown (bypass signal, call runAll directly).
	m.runAll()

	// Wait for in-flight request to complete.
	<-reqDone

	if reqErr != nil {
		t.Fatalf("in-flight request failed: %v", reqErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify hook execution order.
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Fatalf("expected 3 hooks, got %d: %v", len(order), order)
	}
	if order[0] != "drain-http" || order[1] != "close-pool" || order[2] != "flush-otel" {
		t.Fatalf("expected [drain-http close-pool flush-otel], got %v", order)
	}

	// Verify new requests are rejected after shutdown.
	_, err = http.Get(addr)
	if err == nil {
		t.Fatal("expected error for request after shutdown")
	}
}

func TestWaitForInflight(t *testing.T) {
	var processing atomic.Bool
	processing.Store(true)

	// Simulate work completing after 200ms.
	go func() {
		time.Sleep(200 * time.Millisecond)
		processing.Store(false)
	}()

	m := New(5 * time.Second)
	m.Register("wait-inflight", 10, WaitForInflight("test-worker", func() bool {
		return !processing.Load()
	}, 50*time.Millisecond))

	start := time.Now()
	m.runAll()
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond || elapsed > 1*time.Second {
		t.Fatalf("expected ~200ms wait, got %v", elapsed)
	}
}
