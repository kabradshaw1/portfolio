# gRPC Hardening: Auth gRPC + Proto Governance + Graceful Shutdown

**Issues:** #96, #98, #100
**Date:** 2026-04-21

## Overview

Consolidates three related improvements into a single deliverable: adding a gRPC server to auth-service with token revocation checking, enforcing proto contract governance in CI, and standardizing graceful shutdown across all Go services.

## 1. Auth-service gRPC + CheckToken

### Proto

New file `go/proto/auth/v1/auth.proto`:

```protobuf
syntax = "proto3";
package auth.v1;
option go_package = "gen_ai_engineer/go/auth-service/pb/auth/v1;authv1";

service AuthService {
  rpc CheckToken(CheckTokenRequest) returns (CheckTokenResponse);
}

message CheckTokenRequest {
  string token = 1;
}

message CheckTokenResponse {
  bool valid = 1;
  string user_id = 2;
  string reason = 3; // populated on rejection: "expired", "revoked", "malformed"
}
```

Generated code output: `go/auth-service/pb/auth/v1/`.

### gRPC Server

auth-service gets a dual server: REST on :8091 (existing), gRPC on :9091 (new). Registers:
- `AuthService` implementation
- gRPC health check (`grpc_health_v1`)
- gRPC reflection
- OTel interceptor (`otelgrpc.NewServerHandler()`)

Follows the same pattern as product-service's dual server.

### CheckToken Implementation

1. Parse the JWT (reuse existing `internal/auth/` token validation logic)
2. Validate signature and expiry
3. Look up the token's `jti` in the Redis denylist (existing `auth:revoked:<jti>` keys)
4. Return `valid=false` with `reason` if any step fails

No new Redis data structures needed — the denylist already exists from the logout flow.

### Shared Auth Middleware

New package: `go/pkg/authmiddleware/`

A Gin middleware that:
1. Extracts Bearer token from the `Authorization` header
2. Validates JWT signature + expiry locally (fast path, no network call)
3. Calls auth-service `CheckToken` via gRPC to check the denylist
4. Caches the `CheckToken` result in a local TTL map (default 30s, configurable via `WithCacheTTL()`)
5. On success: sets `user_id` in the Gin context
6. On failure: returns 401 with the rejection reason

Constructor: `New(authConn *grpc.ClientConn, opts ...Option) gin.HandlerFunc`

Options:
- `WithCacheTTL(d time.Duration)` — denylist check cache duration (default 30s)
- `WithSkipPaths(paths ...string)` — paths that bypass auth (e.g., health checks)

Services opt in by adding the middleware to their Gin router group. The gRPC client connection to auth-service is created at service startup.

### Services to Update

- **product-service**: Replace current JWT-only middleware with shared authmiddleware
- **cart-service**: Replace current JWT-only middleware with shared authmiddleware
- **order-service**: Replace current JWT-only middleware with shared authmiddleware
- **ai-service**: Replace current JWT-only middleware with shared authmiddleware
- **analytics-service**: No auth (internal metrics only), skip

Each service's go.mod needs no new `replace` directive since `go/pkg/` is already referenced.

### Kubernetes

- auth-service deployment: add gRPC port (9091) to container ports and service
- auth-service service: add gRPC port mapping
- Other services: add `AUTH_GRPC_URL` env var pointing to `auth-service:9091`

## 2. Graceful Shutdown

### Shared Utility

New package: `go/pkg/shutdown/`

```go
type Manager struct { ... }

func New(timeout time.Duration) *Manager
func (m *Manager) Register(name string, priority int, fn func(ctx context.Context) error)
func (m *Manager) Wait()
```

`Wait()` blocks on SIGINT/SIGTERM, then runs all registered functions in priority order (ascending). Functions at the same priority run concurrently. The timeout context is passed to each function. Errors are logged but don't stop the sequence.

### Priority Convention

| Priority | Action | Examples |
|----------|--------|----------|
| 0 | Stop accepting new work | Stop RabbitMQ consumer, stop Kafka reader |
| 10 | Drain in-flight work | gRPC `GracefulStop()`, finish current saga step, commit Kafka offsets |
| 20 | Close connections | HTTP server `Shutdown()`, database pool close, Redis close, RabbitMQ connection close |
| 30 | Flush telemetry | OTel TracerProvider and MeterProvider `Shutdown()` |

### Per-service Changes

Each service's `main.go` replaces its current ad-hoc signal handling with the shutdown manager. All services use a 15-second shutdown timeout.

**auth-service:**
- Priority 10: gRPC `GracefulStop()` (new)
- Priority 20: HTTP server shutdown, Redis close, DB pool close
- Priority 30: OTel shutdown

**product-service:**
- Priority 10: gRPC `GracefulStop()` (currently present but ad-hoc)
- Priority 20: HTTP server shutdown, DB pool close
- Priority 30: OTel shutdown

**cart-service:**
- Priority 0: Stop RabbitMQ saga consumer (if running)
- Priority 10: gRPC `GracefulStop()`, finish current saga handler
- Priority 20: HTTP server shutdown, Redis close, DB pool close, RabbitMQ connection close
- Priority 30: OTel shutdown

**order-service:**
- Priority 0: Stop RabbitMQ saga consumer, stop Kafka producer flush
- Priority 10: Finish current saga step
- Priority 20: HTTP server shutdown, Redis close, DB pool close, RabbitMQ connection close
- Priority 30: OTel shutdown

**ai-service:**
- Priority 0: Stop Kafka producer flush (if enabled)
- Priority 20: HTTP server shutdown, Redis close
- Priority 30: OTel shutdown

**analytics-service:**
- Priority 0: Stop Kafka reader (`reader.Close()`)
- Priority 10: Commit final offsets
- Priority 20: HTTP server shutdown
- Priority 30: OTel shutdown

### Kubernetes Alignment

All Go service deployments: `terminationGracePeriodSeconds: 20` (5s buffer over the 15s code timeout). Update any deployments that currently use the default 30s.

## 3. Proto Contract Testing

### CI Step

Add a `buf-breaking` job to `.github/workflows/ci.yml`:

```yaml
buf-breaking:
  name: Proto breaking change check
  runs-on: ubuntu-latest
  if: needs.changes.outputs.go == 'true'
  steps:
    - uses: actions/checkout@v4
      with:
        fetch-depth: 0  # need full history for branch comparison
    - uses: bufbuild/buf-setup-action@v1
    - run: buf breaking go/proto --against '.git#branch=origin/${{ github.base_ref }}'
      working-directory: .
```

Runs on PRs that touch Go files (using the existing `changes` job output). Compares proto files against the PR's base branch.

### What It Catches

Uses the `FILE` category (buf default):
- Removing or renaming an RPC, message, or field
- Changing field numbers or types
- Changing service or package names
- Any wire-incompatible change

### No Escape Hatch

Breaking changes require versioning the proto package (e.g., `auth/v1` → `auth/v2`). No CI bypass labels.

## Testing Strategy

### Unit Tests
- `go/pkg/shutdown/` — test priority ordering, timeout behavior, concurrent same-priority execution
- `go/pkg/authmiddleware/` — test token extraction, cache behavior, gRPC call mocking, skip paths
- auth-service `CheckToken` handler — test valid/expired/revoked/malformed tokens

### Integration Tests
- auth-service: start gRPC server, call `CheckToken` with valid and revoked tokens, verify Redis denylist lookup
- authmiddleware: spin up auth-service + a test service, verify end-to-end token validation with denylist

### Existing Test Updates
- Services that currently test their own JWT middleware need tests updated to use the shared authmiddleware
- Existing product-service gRPC tests should still pass (shutdown changes are additive)

## Files Changed (Summary)

**New files:**
- `go/proto/auth/v1/auth.proto`
- `go/auth-service/pb/auth/v1/*.pb.go` (generated)
- `go/auth-service/internal/grpc/` (gRPC server + CheckToken handler)
- `go/pkg/shutdown/shutdown.go` + `shutdown_test.go`
- `go/pkg/authmiddleware/middleware.go` + `middleware_test.go`

**Modified files:**
- `go/auth-service/cmd/server/main.go` — add gRPC server, shutdown manager
- `go/product-service/cmd/server/main.go` — shutdown manager, shared authmiddleware
- `go/cart-service/cmd/server/main.go` — shutdown manager, shared authmiddleware
- `go/order-service/cmd/server/main.go` — shutdown manager, shared authmiddleware
- `go/ai-service/cmd/server/main.go` — shutdown manager, shared authmiddleware
- `go/analytics-service/cmd/server/main.go` — shutdown manager
- `go/auth-service/k8s/` (or `go/k8s/`) — deployment + service for gRPC port
- All Go service k8s deployments — `terminationGracePeriodSeconds: 20`
- `.github/workflows/ci.yml` — add `buf-breaking` job
- `go/buf.yaml` — ensure auth proto path is included
