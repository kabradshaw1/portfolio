# ADR: Graceful Shutdown Orchestration

## Status
Accepted

## Context
A production-readiness audit of the 6 Go microservices found that while all services handled SIGTERM/SIGINT via a shared shutdown manager, the shutdown sequences had gaps:

- **3 services missing `pool.Close()`** — product, cart, and order services registered HTTP and OTel shutdown hooks but never closed the PostgreSQL connection pool, leaking connections on pod termination.
- **No in-flight work awareness** — saga handlers (cart, order) and the Kafka consumer (analytics) could be mid-message when shutdown was triggered. Killing them mid-saga leaves orders in a stuck state (e.g., stock reserved but cart never cleared).
- **Manual shutdown hooks** — each service hand-wrote its own `grpcServer.GracefulStop()` and `srv.Shutdown(ctx)` calls, duplicating logic and using inconsistent priority ordering.
- **Unbounded queries** — `Categories()` and `FindIncompleteSagas()` had no LIMIT, risking full table scans.
- **Health check client churn** — ai-service created a new `http.Client` per health probe call instead of reusing one.

## Decision

### Shared Shutdown Helpers (`go/pkg/shutdown/drain.go`)

Three new helpers that return `func(ctx context.Context) error` closures, composable with the existing `Manager.Register()`:

- **`DrainHTTP(name, srv)`** — wraps `srv.Shutdown(ctx)` to stop accepting connections and complete in-flight requests.
- **`DrainGRPC(name, srv)`** — calls `GracefulStop()` with a context-deadline fallback to `Stop()`. Prevents hanging if a gRPC stream never completes.
- **`WaitForInflight(name, idle, pollInterval)`** — polls an `idle() bool` callback until in-flight work finishes or the context expires. Generic enough for saga handlers, Kafka consumers, or any async worker.

### Priority Ordering

All 6 services now follow the same 4-stage shutdown sequence:

| Priority | Stage | What happens |
|----------|-------|-------------|
| 0 | Stop accepting work | HTTP drain, gRPC drain, cancel context |
| 10 | Wait for in-flight work | Saga steps complete, Kafka messages finish |
| 20 | Close connections | PostgreSQL pool, Redis, RabbitMQ, Kafka reader |
| 30 | Flush telemetry | OpenTelemetry exporter |

Hooks at the same priority run concurrently. Each stage completes before the next begins. If the overall timeout (15s, matching K8s default `terminationGracePeriodSeconds`) expires, remaining hooks are cancelled.

### In-Flight Work Tracking

Saga handlers (cart-service, order-service) and the Kafka consumer (analytics-service) gained an `IsIdle() bool` method backed by `sync/atomic.Bool`. The flag is set to true before message handling and cleared after. The shutdown manager polls this via `WaitForInflight` at priority 10 — after accepting is stopped (priority 0) but before connections close (priority 20).

This ensures that if the order-service is mid-saga (e.g., stock reserved, waiting for cart-cleared event), the current step completes before RabbitMQ connections close. On the next pod startup, `RecoverIncomplete()` picks up any sagas that didn't finish.

### Quick Polish

- **Defensive LIMIT 100** on `Categories()` and `FindIncompleteSagas()` — prevents unbounded result sets even though current data volumes are small.
- **Shared health check client** in ai-service — single `http.Client` reused across probes instead of allocating per call.

## Verification

- **Integration test** (`go/pkg/shutdown/shutdown_test.go`): starts an HTTP server with a 500ms handler, triggers shutdown mid-request, and verifies:
  1. The in-flight request completes (HTTP 200, not dropped)
  2. Hooks execute in priority order (drain-http before close-pool before flush-otel)
  3. New requests after shutdown are rejected
- **WaitForInflight test**: verifies polling waits ~200ms for a simulated worker to finish, then returns.
- `make preflight-go` passes across all 6 services.

## Consequences

- **All services now depend on `go/pkg/shutdown/`** for drain helpers. This was already true for the Manager — the helpers just extend it.
- **gRPC dependency in `go/pkg/`** — `drain.go` imports `google.golang.org/grpc` for the `DrainGRPC` helper. This was already an indirect dependency via OTel; now it's direct.
- **`atomic.Bool` overhead** — negligible. One `Store(true)` and `Store(false)` per message. No contention since only one goroutine writes and the shutdown manager reads.
- **15s shutdown timeout** is hardcoded, matching the K8s default `terminationGracePeriodSeconds`. If a saga step takes longer than 15s, it will be interrupted and recovered on startup via `RecoverIncomplete()`.
