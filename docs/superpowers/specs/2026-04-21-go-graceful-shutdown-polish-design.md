# Go Services Polish: Graceful Shutdown & Production Hardening

**Date:** 2026-04-21
**Status:** Draft
**Services:** All 6 Go services

## Context

After the DB optimization work (PR #111), a comprehensive audit of all 6 Go services found zero critical issues across 13,000+ lines. The remaining polish items focus on graceful shutdown orchestration and defensive query patterns — the kind of production-readiness details that separate senior engineers in interviews.

## Goal

Enhance the shared `go/pkg/shutdown/` package with HTTP drain, gRPC drain, and in-flight work awareness. Update all 6 services to use consistent shutdown orchestration. Fix the 3 services missing `pool.Close()`. Add defensive limits on unbounded queries. Prove it works with an integration test.

## Motivation

Interviewers ask "what happens when your pod gets killed mid-request?" This work gives a concrete answer: in-flight HTTP requests complete, active saga steps reach safe checkpoints, gRPC streams drain, connections close in dependency order, and telemetry flushes last. Verified by a test.

## Enhanced `go/pkg/shutdown/` Package

Three new helpers added to the existing shutdown manager:

### `DrainHTTP(srv *http.Server, timeout time.Duration) Hook`

Calls `srv.Shutdown(ctx)` which stops accepting new connections and waits for in-flight requests to complete up to the timeout. Wraps the stdlib behavior as a registerable shutdown hook.

### `DrainGRPC(srv *grpc.Server) Hook`

Calls `srv.GracefulStop()` which stops accepting new RPCs and waits for active RPCs to finish. Falls back to `srv.Stop()` if the context deadline expires.

### `WaitForInflight(name string, check func() bool, pollInterval time.Duration) Hook`

Generic "wait until idle" hook. Polls `check()` at `pollInterval` until it returns true (no in-flight work) or context expires. Used by saga consumers and Kafka consumers to wait for active message processing to complete before closing connections.

The `IsProcessing() bool` method on saga handlers and the Kafka consumer uses `atomic.Bool` to track whether a message is currently being handled — set to true on message receive, false on handler completion. Thread-safe with zero contention.

### Priority Ordering (consistent across all services)

| Priority | Stage | What happens |
|----------|-------|-------------|
| 0 | Stop accepting work | HTTP drain, gRPC drain |
| 10 | Wait for in-flight work | Saga steps complete, Kafka messages finish |
| 20 | Close connections | PostgreSQL pool, Redis, RabbitMQ, Kafka |
| 30 | Flush telemetry | OpenTelemetry exporter |

## Service-Specific Changes

### product-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- Register `DrainGRPC` at priority 0
- Register `pool.Close()` at priority 20 **(currently missing)**
- OTel at priority 30

### cart-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- Register `DrainGRPC` at priority 0
- Register `WaitForInflight` at priority 10 for saga handler — add `IsProcessing() bool` method (atomic.Bool) on the saga handler that returns true when actively handling a RabbitMQ message
- Register `pool.Close()` at priority 20 **(currently missing)**
- Close RabbitMQ at priority 20
- OTel at priority 30

### order-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- Register `WaitForInflight` at priority 10 for saga orchestrator — add `IsProcessing() bool` method (atomic.Bool). Critical: if mid-saga (stock reserved but cart not cleared), must let current step complete
- Register `pool.Close()` at priority 20 **(currently missing)**
- Close RabbitMQ at priority 20
- OTel at priority 30

### auth-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- Standardize on shared `DrainGRPC` at priority 0 (already does GracefulStop, use the shared hook for consistency)
- Rest already correct (pool.Close and Redis already registered)

### ai-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- OTel at priority 30

### analytics-service (`cmd/server/main.go`)
- Register `DrainHTTP` at priority 0
- Register `WaitForInflight` at priority 10 for Kafka consumer — add `IsProcessing() bool` (atomic.Bool) on the consumer
- Close Kafka reader at priority 20
- OTel at priority 30

## Quick Polish (rides along in same PR)

### Defensive LIMIT on unbounded queries
- `go/product-service/internal/repository/product.go` — `Categories()`: add `LIMIT 100` to `SELECT DISTINCT category FROM products ORDER BY category`
- `go/order-service/internal/repository/order.go` — `FindIncompleteSagas()`: add `LIMIT 100` to `SELECT id FROM orders WHERE saga_step NOT IN (...)`

### Health check HTTP client reuse
- `go/ai-service/cmd/server/routes.go` — Extract per-call `&http.Client{Timeout: 2s}` into shared `healthClient` initialized once in route setup

## Shutdown Integration Test

Test in `go/pkg/shutdown/` proving orchestration works:

1. Start HTTP server with 500ms handler (simulates in-flight request)
2. Send request to server
3. Trigger shutdown while request is in-flight
4. Assert: in-flight request completes successfully (not dropped)
5. Assert: hooks run in priority order (drain-http before close-pool before flush-otel)
6. Assert: new requests after shutdown initiated get rejected

Uses `httptest.NewServer` — no Docker, runs in normal `go test`, ~1s execution.

Does NOT test gRPC drain or saga waiting directly (those are covered by the `WaitForInflight` polling logic and the priority ordering guarantee tested here).

## Deliverables

| Artifact | Path |
|----------|------|
| Enhanced shutdown package | `go/pkg/shutdown/` (drain helpers + WaitForInflight) |
| Shutdown integration test | `go/pkg/shutdown/shutdown_test.go` |
| Product service main.go | Updated shutdown hooks + pool.Close() |
| Cart service main.go | Updated shutdown hooks + pool.Close() + IsProcessing |
| Order service main.go | Updated shutdown hooks + pool.Close() + IsProcessing |
| Auth service main.go | Standardized on shared DrainHTTP + DrainGRPC |
| AI service main.go + routes.go | DrainHTTP + shared health client |
| Analytics service main.go | DrainHTTP + WaitForInflight for Kafka |
| Product repo | Categories() LIMIT 100 |
| Order repo | FindIncompleteSagas() LIMIT 100 |

## Success Criteria

- `make preflight-go` passes (lint + all tests)
- Shutdown test passes: in-flight request completes, hooks run in order, new requests rejected
- Structured log output during shutdown shows each hook name and priority
- Zero regressions in existing tests

## Out of Scope

- Kubernetes preStop hooks / terminationGracePeriodSeconds YAML changes (infrastructure, not Go code)
- Load testing (separate future spec)
- Configurable shutdown timeout (hardcode 30s — matches K8s default terminationGracePeriodSeconds)
