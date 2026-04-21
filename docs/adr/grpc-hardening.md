# gRPC Hardening: Auth gRPC, Graceful Shutdown, Proto Governance

- **Date:** 2026-04-21
- **Status:** Accepted

## Context

The Go microservices had three infrastructure gaps that would surface in a production code review or interview walkthrough:

1. **Token revocation was unenforceable.** Services validated JWTs independently — checking signature and expiry — but had no way to check whether a token had been revoked. The auth-service maintained a Redis denylist (`auth:denied:<sha256>`) for its logout endpoint, but other services never consulted it. A revoked token worked until natural expiry (15 minutes for access tokens). This is the standard critique of stateless JWT auth: you trade session lookup for inability to revoke.

2. **Shutdown was ad-hoc and inconsistent.** Every service had its own signal handling block: create a `quit` channel, call `signal.Notify`, block on `<-quit`, then manually sequence server shutdowns. The ordering varied — some services cancelled their root context before draining gRPC, some didn't cancel it at all (ai-service), some closed Kafka readers without committing offsets first. None had a configurable timeout aligned with Kubernetes' `terminationGracePeriodSeconds`. During rolling updates, in-flight requests could be dropped and saga steps left incomplete.

3. **No proto contract governance.** Two services (product, cart) had proto definitions managed by `buf`, but nothing prevented a developer from making a wire-incompatible change (renaming a field, changing a type, removing an RPC) that would break consumers at runtime. Proto changes were only caught when a consumer failed to deserialize — in production.

These three problems share a theme: the services worked correctly in isolation but lacked the coordination mechanisms that production microservices need.

## Decision

### 1. Auth-Service gRPC with CheckToken

**The approach:** Add a gRPC server to auth-service (port 9091 alongside REST on 8091) with a single `CheckToken` RPC. The handler reuses the existing JWT parsing logic and Redis denylist, returning a structured response with `valid`, `user_id`, and `reason` (expired, revoked, malformed).

**Why gRPC, not REST?** The denylist check is an internal service-to-service call — no browser will ever call it directly. gRPC gives binary serialization (lower latency than JSON), generated client/server stubs (no hand-written HTTP clients), health checking and reflection for free, and automatic OTel instrumentation via `grpc.StatsHandler(otelgrpc.NewServerHandler())`. It also gives auth-service a gRPC server, which was a prerequisite for the mTLS work in issue #101.

**Why not a sidecar or service mesh?** Istio and Linkerd solve this problem at the infrastructure layer — every pod gets a proxy that handles auth, mTLS, retries. But they add significant operational complexity (custom resource definitions, control plane management, resource overhead) that isn't justified for a portfolio project with 6 services. A purpose-built gRPC call is simpler to understand, debug, and explain in an interview.

**The proto definition** (`go/proto/auth/v1/auth.proto`):

```protobuf
service AuthService {
  rpc CheckToken(CheckTokenRequest) returns (CheckTokenResponse);
}

message CheckTokenRequest {
  string token = 1;
}

message CheckTokenResponse {
  bool valid = 1;
  string user_id = 2;
  string reason = 3;  // populated on rejection: "expired", "revoked", "malformed"
}
```

The response always returns without error — validation failures are conveyed via `valid=false` with a `reason`. This is intentional: the caller needs to distinguish "this token is bad" from "auth-service is unreachable." A gRPC error means the network failed; `valid=false` means the token is bad. The middleware uses this distinction to fail open on network errors (local JWT validation already passed) while blocking revoked tokens.

### 2. Shared Auth Middleware with Denylist Cache

**The approach:** A `authmiddleware` package at `go/auth-service/authmiddleware/` (public, not `internal/`) that other services import. The middleware:

1. Extracts the Bearer token from the `Authorization` header or `access_token` cookie
2. Validates JWT signature + expiry locally (fast path — no network call)
3. Calls auth-service `CheckToken` via gRPC to check the denylist
4. Caches the `CheckToken` result in a local `sync.RWMutex`-guarded map with a configurable TTL (default 30 seconds)
5. On gRPC error, fails open — local validation already passed

**Why cache?** Without caching, every authenticated request adds a gRPC round-trip to auth-service. At 30s TTL, a revoked token can still work for up to 30 seconds — but auth-service load stays constant regardless of request volume. The TTL is a constructor option (`WithCacheTTL(d)`) so services can tune the tradeoff.

**Why fail open on gRPC errors?** The alternative is fail-closed: if auth-service is unreachable, reject all requests. This creates a hard dependency — auth-service downtime takes down every service. Since the middleware already validated the JWT locally (signature, expiry, claims), the only thing lost on gRPC failure is the denylist check. The risk window is bounded: a revoked token works until its natural 15-minute expiry. For a portfolio project this is the right tradeoff; in a high-security environment you'd fail closed and accept the availability risk.

**Why in auth-service, not go/pkg?** The middleware imports `go/auth-service/pb/auth/v1` for the generated proto types. Putting it in `go/pkg` would make `go/pkg` depend on `go/auth-service`, creating a circular dependency through all services. Placing it in the auth-service module (but outside `internal/`) keeps the dependency tree clean: services that need denylist checking import `go/auth-service/authmiddleware` and `go/auth-service/pb/auth/v1` directly.

**Service adoption:** Order-service and cart-service now use the shared middleware. Their local `internal/middleware/auth.go` files are no longer imported by routes but remain in the codebase (cleanup for later). Each service creates a gRPC client connection to auth-service at startup and passes it to the middleware constructor. The `AUTH_GRPC_URL` env var configures the connection target.

### 3. Graceful Shutdown Manager

**The approach:** A `shutdown` package at `go/pkg/shutdown/` with a `Manager` that accepts named hooks at numeric priorities, blocks on SIGINT/SIGTERM, then runs hooks in ascending priority order with a configurable timeout.

```go
sm := shutdown.New(15 * time.Second)
sm.Register("cancel-ctx", 0, func(_ context.Context) error { cancel(); return nil })
sm.Register("grpc-drain", 10, func(ctx context.Context) error { grpcServer.GracefulStop(); return nil })
sm.Register("http", 20, func(ctx context.Context) error { return srv.Shutdown(ctx) })
sm.Register("otel", 30, func(ctx context.Context) error { return shutdownTracer(ctx) })
sm.Wait()
```

**Priority convention:**
| Priority | Action | Examples |
|----------|--------|----------|
| 0 | Stop accepting new work | Cancel root context, stop RabbitMQ consumer |
| 10 | Drain in-flight work | gRPC `GracefulStop()`, finish saga step, commit Kafka offsets |
| 20 | Close connections | HTTP server shutdown, database pools, Redis |
| 30 | Flush telemetry | OTel TracerProvider shutdown |

**Why priorities, not an ordered list?** Priorities let hooks at the same level run concurrently (`sync.WaitGroup` per group). HTTP and database shutdown are independent at priority 20 — running them concurrently saves time. An ordered list would force sequential execution.

**Why not a lifecycle framework?** Libraries like `fx` (Uber) or `run.Group` (oklog) manage the full application lifecycle — startup, readiness, shutdown. They're powerful but opinionated: you register components, not functions, and the framework controls startup order. The shutdown manager is intentionally minimal: it only handles the shutdown path, and each service's `main.go` explicitly registers what needs to happen. Six services with different components (some have gRPC, some have Kafka, some have RabbitMQ) benefit from explicit registration over framework magic.

**Kubernetes alignment:** All deployments now set `terminationGracePeriodSeconds: 20`, giving 5 seconds of buffer over the 15-second code timeout. When Kubernetes sends SIGTERM, the pod has 20 seconds before a SIGKILL. The shutdown manager runs its hooks within 15 seconds. If a hook hangs, the context deadline fires at 15 seconds and the manager logs a warning and moves on. The 5-second buffer covers the time between SIGTERM delivery and the manager starting its sequence.

### 4. Proto Contract Governance

**The approach:** A `buf-breaking` CI job that runs on pull requests when `go/proto/` files change. It compares the PR's proto definitions against the base branch using `buf breaking` with the `FILE` category, catching:

- Removing or renaming an RPC, message, or field
- Changing field numbers or types
- Changing service or package names

**No escape hatch.** Breaking changes require versioning the proto package (`auth/v1` -> `auth/v2`). There's no CI bypass label. The reasoning: bypass labels invite shortcuts ("I'll fix the consumers later"), and with only three proto packages (product, cart, auth) there's no legitimate reason to break wire compatibility without a version bump.

**Why `buf breaking` over manual review?** A human reviewer can catch `"you renamed this field"` but will miss `"you changed field 3 from int32 to int64"` — a wire-compatible Go change that silently corrupts data for any consumer still on the old proto. `buf breaking` checks the wire format, not the source code.

## Consequences

**Positive:**
- Revoked tokens are now enforced within the cache TTL window (30s default) across all authenticated services
- Shutdown is consistent, ordered, and timeout-bounded — no more dropped requests during rolling updates
- Proto breaking changes are caught before merge, not in production
- Auth-service now speaks gRPC, unblocking mTLS work (#101)
- The shutdown manager is reusable for any future Go service
- Interview talking points: zero-trust auth patterns, latency-vs-consistency tradeoffs (cache TTL), graceful shutdown ordering, API contract governance

**Trade-offs:**
- Order-service and cart-service now depend on auth-service at startup (gRPC client connection). If auth-service is down, the middleware fails open — requests proceed with local-only JWT validation but no denylist checking
- The in-memory denylist cache is per-pod, not shared. Two pods can have different cache states for the same token during the TTL window. For this portfolio's single-replica deployments this is irrelevant; at scale you'd use a shared cache (Redis) or shorter TTL
- Cross-module `replace` directives in go.mod add friction when adding services. Each service that imports the auth proto or middleware needs `replace github.com/kabradshaw1/portfolio/go/auth-service => ../auth-service` and a corresponding `COPY auth-service/` in its Dockerfile. This is the standard Go multi-module monorepo tradeoff — it works cleanly but requires the checklist in CLAUDE.md
- `buf breaking` adds ~10 seconds to PR CI runs that touch proto files. Negligible compared to Go lint and test time
