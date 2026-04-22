# Go Graceful Shutdown & Production Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enhance the shared shutdown package with drain helpers, update all 6 Go services with consistent shutdown orchestration, add in-flight work awareness to saga/Kafka consumers, and polish unbounded queries and health checks.

**Architecture:** Three new helpers in `go/pkg/shutdown/` (DrainHTTP, DrainGRPC, WaitForInflight) compose into each service's shutdown sequence via the existing priority-based Manager. Saga handlers and the Kafka consumer get an `atomic.Bool` processing flag so the shutdown manager can wait for in-flight work to finish before closing connections.

**Tech Stack:** Go 1.26, net/http, google.golang.org/grpc, sync/atomic, go/pkg/shutdown

---

### Task 1: Add Drain Helpers to `go/pkg/shutdown/`

**Files:**
- Create: `go/pkg/shutdown/drain.go`

Add three helper functions that return closures matching the `func(ctx context.Context) error` signature expected by `Manager.Register()`.

- [ ] **Step 1: Create `go/pkg/shutdown/drain.go`**

```go
package shutdown

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
)

// DrainHTTP returns a shutdown hook that gracefully drains an HTTP server.
// It stops accepting new connections and waits for in-flight requests to
// complete, up to the context deadline.
func DrainHTTP(name string, srv interface{ Shutdown(ctx context.Context) error }) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("draining HTTP server", "name", name)
		return srv.Shutdown(ctx)
	}
}

// DrainGRPC returns a shutdown hook that gracefully drains a gRPC server.
// It stops accepting new RPCs and waits for active RPCs to finish. Falls
// back to hard Stop() if the context deadline expires.
func DrainGRPC(name string, srv *grpc.Server) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("draining gRPC server", "name", name)
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			slog.Warn("gRPC drain timeout, forcing stop", "name", name)
			srv.Stop()
			return ctx.Err()
		}
	}
}

// WaitForInflight returns a shutdown hook that polls check() until it
// returns true (no in-flight work) or the context expires. Use this to
// let saga handlers and Kafka consumers finish processing their current
// message before closing connections.
func WaitForInflight(name string, idle func() bool, pollInterval time.Duration) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		slog.Info("waiting for in-flight work to complete", "name", name)
		for {
			if idle() {
				slog.Info("in-flight work complete", "name", name)
				return nil
			}
			select {
			case <-ctx.Done():
				slog.Warn("in-flight wait timeout", "name", name)
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}
}
```

Note: `DrainHTTP` accepts an interface instead of `*http.Server` to make it testable without importing `net/http` into the shutdown package. The `Shutdown(ctx) error` method is the only thing we need.

- [ ] **Step 2: Run `go mod tidy` in `go/pkg/` to pick up grpc dependency**

```bash
cd go/pkg && go get google.golang.org/grpc@latest && go mod tidy
```

- [ ] **Step 3: Verify the package compiles**

```bash
cd go/pkg && go build ./shutdown/
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add go/pkg/shutdown/drain.go go/pkg/go.mod go/pkg/go.sum
git commit -m "feat(pkg/shutdown): add DrainHTTP, DrainGRPC, and WaitForInflight helpers"
```

---

### Task 2: Shutdown Integration Test

**Files:**
- Modify: `go/pkg/shutdown/shutdown_test.go`

Add a test that proves in-flight HTTP requests complete during shutdown, hooks run in priority order, and new requests are rejected after shutdown begins.

- [ ] **Step 1: Add the integration test to `go/pkg/shutdown/shutdown_test.go`**

Append these tests at the end of the file:

```go
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
```

Add these imports to the import block at the top of the test file:

```go
"net"
"net/http"
"sync"
```

- [ ] **Step 2: Run the tests**

```bash
cd go/pkg && go test -v -race ./shutdown/
```

Expected: all tests pass including new ones.

- [ ] **Step 3: Commit**

```bash
git add go/pkg/shutdown/shutdown_test.go
git commit -m "test(pkg/shutdown): add integration tests for HTTP drain and in-flight wait"
```

---

### Task 3: Add IsProcessing to Cart Saga Handler

**Files:**
- Modify: `go/cart-service/internal/worker/saga_handler.go`

Add an `atomic.Bool` processing flag and an `IsIdle() bool` method.

- [ ] **Step 1: Add the processing field and IsIdle method**

In `go/cart-service/internal/worker/saga_handler.go`, add `"sync/atomic"` to the imports, then modify the `SagaHandler` struct and add the `IsIdle` method. Also wrap the message handling in `Start()` with processing flag.

Change the struct (around line 47-49) from:

```go
type SagaHandler struct {
	svc CartServiceForSaga
	ch  *amqp.Channel
}
```

to:

```go
type SagaHandler struct {
	svc        CartServiceForSaga
	ch         *amqp.Channel
	processing atomic.Bool
}
```

Add the `IsIdle` method after the constructor:

```go
// IsIdle returns true when no message is actively being processed.
// Used by the shutdown manager to wait for in-flight saga commands.
func (h *SagaHandler) IsIdle() bool {
	return !h.processing.Load()
}
```

In the `Start()` method, wrap the message handling (the `case msg` branch, around lines 74-79) to set/clear the flag:

```go
			case msg, ok := <-msgs:
				if !ok {
					return nil
				}
				h.processing.Store(true)
				if err := h.handleMessage(ctx, msg); err != nil {
					slog.Error("saga command handling failed", "error", err)
					_ = msg.Nack(false, true)
				} else {
					_ = msg.Ack(false)
				}
				h.processing.Store(false)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/cart-service && go vet ./internal/worker/
```

- [ ] **Step 3: Commit**

```bash
git add go/cart-service/internal/worker/saga_handler.go
git commit -m "feat(cart-service): add IsIdle() to saga handler for graceful shutdown"
```

---

### Task 4: Add IsProcessing to Order Saga Consumer

**Files:**
- Modify: `go/order-service/internal/saga/consumer.go`

Same pattern as Task 3 — add `atomic.Bool` and `IsIdle()`.

- [ ] **Step 1: Add the processing field and IsIdle method**

In `go/order-service/internal/saga/consumer.go`, add `"sync/atomic"` to imports, then modify the struct (lines 14-16) from:

```go
type Consumer struct {
	orch *Orchestrator
}
```

to:

```go
type Consumer struct {
	orch       *Orchestrator
	processing atomic.Bool
}
```

Add after the constructor:

```go
// IsIdle returns true when no saga event is actively being processed.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}
```

In `Start()`, wrap the message handling (the `case msg` branch, around lines 40-45):

```go
			case msg, ok := <-msgs:
				if !ok {
					return nil
				}
				c.processing.Store(true)
				if err := c.handleMessage(ctx, msg); err != nil {
					slog.Error("saga event handling failed", "error", err)
					_ = msg.Nack(false, true)
				} else {
					_ = msg.Ack(false)
				}
				c.processing.Store(false)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/order-service && go vet ./internal/saga/
```

- [ ] **Step 3: Commit**

```bash
git add go/order-service/internal/saga/consumer.go
git commit -m "feat(order-service): add IsIdle() to saga consumer for graceful shutdown"
```

---

### Task 5: Add IsProcessing to Analytics Kafka Consumer

**Files:**
- Modify: `go/analytics-service/internal/consumer/consumer.go`

The consumer already has `connected atomic.Bool`. Add a `processing atomic.Bool` and `IsIdle()`.

- [ ] **Step 1: Add the processing field and IsIdle method**

The struct (around lines 25-31) already has `connected atomic.Bool`. Add `processing`:

```go
type Consumer struct {
	reader     *kafka.Reader
	orders     *OrderAggregator
	trending   *TrendingAggregator
	carts      *CartAggregator
	connected  atomic.Bool
	processing atomic.Bool
}
```

Add after the `Connected()` method:

```go
// IsIdle returns true when no Kafka message is actively being processed.
func (c *Consumer) IsIdle() bool {
	return !c.processing.Load()
}
```

In `Run()`, wrap the message processing (around lines 72-86) with the flag:

```go
		c.connected.Store(true)

		// Record consumer lag from reader stats.
		stats := c.reader.Stats()
		metrics.ConsumerLag.Set(float64(stats.Lag))

		// Extract trace context from Kafka headers.
		msgCtx := tracing.ExtractKafka(ctx, msg.Headers)
		_ = msgCtx // available for span creation if tracing is enabled

		c.processing.Store(true)
		c.route(msg)
		c.processing.Store(false)

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			slog.Error("kafka commit error", "error", err)
		}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/analytics-service && go vet ./internal/consumer/
```

- [ ] **Step 3: Commit**

```bash
git add go/analytics-service/internal/consumer/consumer.go
git commit -m "feat(analytics-service): add IsIdle() to Kafka consumer for graceful shutdown"
```

---

### Task 6: Update product-service Shutdown Hooks

**Files:**
- Modify: `go/product-service/cmd/server/main.go`

Replace the manual shutdown hooks with shared helpers. Add missing `pool.Close()`.

- [ ] **Step 1: Update shutdown registration**

In `go/product-service/cmd/server/main.go`, add `"github.com/kabradshaw1/portfolio/go/pkg/shutdown"` is already imported. Replace lines 103-119 (the shutdown block) with:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("product-http", httpSrv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("product-grpc", grpcServer))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/product-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/product-service/cmd/server/main.go
git commit -m "feat(product-service): use shared shutdown helpers, add pool.Close()"
```

---

### Task 7: Update cart-service Shutdown Hooks

**Files:**
- Modify: `go/cart-service/cmd/server/main.go`

Replace manual hooks, add `pool.Close()`, add `WaitForInflight` for saga handler.

- [ ] **Step 1: Update shutdown registration**

Replace lines 155-171 (the shutdown block) with:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("cart-http", httpSrv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("cart-grpc", grpcServer))
	sm.Register("wait-saga", 10, shutdown.WaitForInflight("cart-saga", sagaHandler.IsIdle, 100*time.Millisecond))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("rabbitmq", 20, func(_ context.Context) error {
		_ = rmqCh.Close()
		return rmqConn.Close()
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

NOTE: The variable names for RabbitMQ connection/channel and sagaHandler may differ from the above. Check the actual variable names in main.go and adjust. The saga handler is created around line 87-88. The RabbitMQ connection and channel are typically `rmqConn` and `rmqCh` or similar — check lines 74-80.

- [ ] **Step 2: Verify it compiles**

```bash
cd go/cart-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/cart-service/cmd/server/main.go
git commit -m "feat(cart-service): use shared shutdown helpers, add pool.Close() and saga drain"
```

---

### Task 8: Update order-service Shutdown Hooks

**Files:**
- Modify: `go/order-service/cmd/server/main.go`

Replace manual hooks, add `pool.Close()`, add `WaitForInflight` for saga consumer.

- [ ] **Step 1: Update shutdown registration**

Replace lines 142-154 (the shutdown block) with:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("order-http", srv))
	sm.Register("wait-saga", 10, shutdown.WaitForInflight("order-saga", sagaConsumer.IsIdle, 100*time.Millisecond))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("rabbitmq", 20, func(_ context.Context) error {
		_ = ch.Close()
		return conn.Close()
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

NOTE: Check actual variable names for the saga consumer (likely `sagaConsumer` or the value returned from `saga.NewConsumer()`), RabbitMQ connection/channel (likely `conn` and `ch`), and pool. Adjust accordingly.

- [ ] **Step 2: Verify it compiles**

```bash
cd go/order-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/order-service/cmd/server/main.go
git commit -m "feat(order-service): use shared shutdown helpers, add pool.Close() and saga drain"
```

---

### Task 9: Update auth-service Shutdown Hooks

**Files:**
- Modify: `go/auth-service/cmd/server/main.go`

Standardize on shared `DrainHTTP` and `DrainGRPC` helpers. Auth-service already has `pool.Close()` and `redis.Close()`.

- [ ] **Step 1: Update shutdown registration**

Replace lines 103-124 (the shutdown block) with:

```go
	sm := shutdown.New(15 * time.Second)
	sm.Register("drain-http", 0, shutdown.DrainHTTP("auth-http", srv))
	sm.Register("drain-grpc", 0, shutdown.DrainGRPC("auth-grpc", grpcServer))
	sm.Register("postgres", 20, func(_ context.Context) error {
		pool.Close()
		return nil
	})
	sm.Register("redis", 20, func(_ context.Context) error {
		if redisClient != nil {
			return redisClient.Close()
		}
		return nil
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/auth-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/auth-service/cmd/server/main.go
git commit -m "refactor(auth-service): standardize on shared shutdown helpers"
```

---

### Task 10: Update ai-service Shutdown Hooks

**Files:**
- Modify: `go/ai-service/cmd/server/main.go`

Add `DrainHTTP`.

- [ ] **Step 1: Update shutdown registration**

Replace lines 162-170 (the shutdown block) with:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("drain-http", 0, shutdown.DrainHTTP("ai-http", srv))
	sm.Register("otel", 30, func(sctx context.Context) error {
		return shutdownTracer(sctx)
	})
	sm.Wait()
```

Add import for the shutdown package if not already imported:
```go
"github.com/kabradshaw1/portfolio/go/pkg/shutdown"
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/ai-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/ai-service/cmd/server/main.go
git commit -m "feat(ai-service): use shared DrainHTTP shutdown helper"
```

---

### Task 11: Update analytics-service Shutdown Hooks

**Files:**
- Modify: `go/analytics-service/cmd/server/main.go`

Add `DrainHTTP` and `WaitForInflight` for Kafka consumer.

- [ ] **Step 1: Update shutdown registration**

Replace lines 63-78 (the shutdown block) with:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("analytics-http", srv))
	sm.Register("wait-kafka", 10, shutdown.WaitForInflight("kafka-consumer", cons.IsIdle, 100*time.Millisecond))
	sm.Register("kafka-close", 20, func(_ context.Context) error {
		return cons.Close()
	})
	sm.Register("http", 20, func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

Wait — the `DrainHTTP` helper already calls `srv.Shutdown(ctx)`, so we don't need the separate "http" hook at priority 20. Let me fix that:

```go
	// Graceful shutdown
	sm := shutdown.New(15 * time.Second)
	sm.Register("cancel-ctx", 0, func(_ context.Context) error {
		cancel()
		return nil
	})
	sm.Register("drain-http", 0, shutdown.DrainHTTP("analytics-http", srv))
	sm.Register("wait-kafka", 10, shutdown.WaitForInflight("kafka-consumer", cons.IsIdle, 100*time.Millisecond))
	sm.Register("kafka-close", 20, func(_ context.Context) error {
		return cons.Close()
	})
	sm.Register("otel", 30, func(ctx context.Context) error {
		return shutdownTracer(ctx)
	})
	sm.Wait()
```

Add import for shutdown package if not already present.

- [ ] **Step 2: Verify it compiles**

```bash
cd go/analytics-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/analytics-service/cmd/server/main.go
git commit -m "feat(analytics-service): use shared shutdown helpers with Kafka in-flight wait"
```

---

### Task 12: Quick Polish — Defensive LIMIT on Unbounded Queries

**Files:**
- Modify: `go/product-service/internal/repository/product.go`
- Modify: `go/order-service/internal/repository/order.go`

- [ ] **Step 1: Add LIMIT to Categories query**

In `go/product-service/internal/repository/product.go`, find the Categories method (around line 257). Change:

```go
rows, err := r.pool.Query(ctx, "SELECT DISTINCT category FROM products ORDER BY category")
```

to:

```go
rows, err := r.pool.Query(ctx, "SELECT DISTINCT category FROM products ORDER BY category LIMIT 100")
```

- [ ] **Step 2: Add LIMIT to FindIncompleteSagas query**

In `go/order-service/internal/repository/order.go`, find the FindIncompleteSagas method (around line 189). Change:

```go
rows, err := r.pool.Query(ctx,
    `SELECT id FROM orders WHERE saga_step NOT IN ($1, $2, $3)`,
    "COMPLETED", "COMPENSATION_COMPLETE", "FAILED",
)
```

to:

```go
rows, err := r.pool.Query(ctx,
    `SELECT id FROM orders WHERE saga_step NOT IN ($1, $2, $3) LIMIT 100`,
    "COMPLETED", "COMPENSATION_COMPLETE", "FAILED",
)
```

- [ ] **Step 3: Verify both compile**

```bash
cd go/product-service && go vet ./internal/repository/
cd go/order-service && go vet ./internal/repository/
```

- [ ] **Step 4: Commit**

```bash
git add go/product-service/internal/repository/product.go go/order-service/internal/repository/order.go
git commit -m "fix: add defensive LIMIT to unbounded categories and saga recovery queries"
```

---

### Task 13: Quick Polish — Health Check HTTP Client Reuse (ai-service)

**Files:**
- Modify: `go/ai-service/cmd/server/routes.go`

- [ ] **Step 1: Extract shared health check client**

In `go/ai-service/cmd/server/routes.go`, add a package-level variable near the top (after the imports, before `setupRouter`):

```go
// healthClient is reused across health check probes to avoid creating
// a new transport per call.
var healthClient = &http.Client{Timeout: 2 * time.Second}
```

Then replace both instances of `client := &http.Client{Timeout: 2 * time.Second}` (lines 46 and 62) with `healthClient`. Specifically:

Line 46, change:
```go
				client := &http.Client{Timeout: 2 * time.Second}
				resp, err := client.Do(req)
```
to:
```go
				resp, err := healthClient.Do(req)
```

Line 62, change:
```go
				client := &http.Client{Timeout: 2 * time.Second}
				resp, err := client.Do(req)
```
to:
```go
				resp, err := healthClient.Do(req)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd go/ai-service && go vet ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add go/ai-service/cmd/server/routes.go
git commit -m "refactor(ai-service): reuse shared HTTP client for health checks"
```

---

### Task 14: Run `go mod tidy` Across All Services

**Files:** Various go.mod/go.sum

Since `go/pkg/` added grpc as a dependency, tidy all modules.

- [ ] **Step 1: Tidy all Go modules**

```bash
cd go/pkg && go mod tidy
cd go/product-service && go mod tidy
cd go/order-service && go mod tidy
cd go/cart-service && go mod tidy
cd go/auth-service && go mod tidy
cd go/ai-service && go mod tidy
cd go/analytics-service && go mod tidy
```

- [ ] **Step 2: Commit if changed**

```bash
git add go/*/go.mod go/*/go.sum go/pkg/go.mod go/pkg/go.sum
git diff --cached --quiet || git commit -m "chore: go mod tidy across all services"
```

---

### Task 15: Preflight Checks

**Files:** None (validation only)

- [ ] **Step 1: Run Go preflight**

```bash
make preflight-go
```

Expected: lint + all tests pass.

- [ ] **Step 2: Fix any lint issues**

If golangci-lint reports issues, fix and commit.

- [ ] **Step 3: Commit fixes if needed**

```bash
git add -u && git commit -m "fix: address lint issues from shutdown polish work"
```
