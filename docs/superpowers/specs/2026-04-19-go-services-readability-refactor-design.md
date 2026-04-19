# Go Services Readability Refactor

## Context

Kyle is applying for Go developer roles and wants the Go services in this portfolio to reflect professional-quality code. The exploration revealed a consistent pattern across all four services: bloated `cmd/server/main.go` files mixing configuration, dependency wiring, middleware, route registration, and server lifecycle. Secondary issues include duplicated business logic in internal packages. This refactor is purely structural — no behavior changes, no new features.

## Scope

All four Go services: **ai-service**, **auth-service**, **ecommerce-service**, **analytics-service**.

Two levels of changes:
1. **Main.go splits** — break god files into focused files in `cmd/server/`
2. **Major duplication fixes** — extract helpers for the worst repeated patterns in internal packages

### Out of Scope

- Test refactoring (table-driven conversions, test helpers)
- LLM client refactoring in ai-service (anthropic/openai/ollama providers)
- Agent loop refactoring in ai-service (`agent.go` Run method)
- Idempotency middleware in ecommerce-service
- Tool definition boilerplate in ai-service (catalog.go, orders.go, etc.)

---

## Changes by Service

### 1. ai-service (current main.go: 344 lines)

**Main.go split** — `cmd/server/`:

| File | Contents |
|------|----------|
| `config.go` | `Config` struct, `loadConfig()` from env vars, `newCircuitBreaker(name)` factory |
| `routes.go` | Router setup, middleware chain, route registration, `registerTools()` (shared by HTTP and MCP paths) |
| `main.go` | Entry point: load config → wire dependencies → start server (~60-80 lines) |

**Internal fixes:**
- Move inline CORS middleware from main.go to `internal/http/middleware.go`
- Consolidate duplicate tool registration (HTTP path lines 139-159 and MCP stdio path lines 285-293) into single `registerTools()` function in `routes.go`
- Extract circuit breaker factory — 4 identical `gobreaker.NewCircuitBreaker()` calls become `newCircuitBreaker(name string)`

**Files modified:** `cmd/server/main.go` (split into 3), `internal/http/middleware.go` (new or extended)

### 2. auth-service (current main.go: 201 lines)

**Main.go split** — `cmd/server/`:

| File | Contents |
|------|----------|
| `config.go` | `Config` struct, `loadConfig()` from env vars |
| `routes.go` | Router setup, middleware chain, route registration |
| `main.go` | Entry point (~50-60 lines) |

**Internal fixes:**
- **handler/auth.go:** Extract `setAuthCookies(w, accessToken, refreshToken, sameSite)` and `clearAuthCookies(w, sameSite)` helpers — eliminates 3x cookie-setting duplication across Register, Login, GoogleLogin, Logout handlers
- **service/auth.go:** Extract duplicated access/refresh JWT claim-building in `generateTokens()` into a shared claim builder

**Files modified:** `cmd/server/main.go` (split into 3), `internal/handler/auth.go`, `internal/service/auth.go`

### 3. ecommerce-service (current main.go: 267 lines)

**Main.go split** — `cmd/server/`:

| File | Contents |
|------|----------|
| `config.go` | `Config` struct, `loadConfig()` from env vars |
| `routes.go` | Router setup, middleware chain, route registration |
| `main.go` | Entry point + `rabbitPublisher` type (currently defined inline, stays in cmd/server/) (~60-80 lines) |

**Internal fixes:**
- **repository/product.go (311 lines):** Extract shared `sortConfig` builder function and `buildWhereClause()` from duplicated cursor/offset pagination implementations (~50 lines of duplication eliminated)
- **service/product.go (150 lines):** Extract generic `getFromCache[T]()` and `setInCache[T]()` helpers to replace 3x repeated cache-aside pattern (reduces file to ~80-90 lines)
- Move inline CORS middleware from main.go to `internal/middleware/` (already has middleware package)

**Files modified:** `cmd/server/main.go` (split into 3), `internal/repository/product.go`, `internal/service/product.go`

### 4. analytics-service (current main.go: 139 lines)

**Main.go split** — `cmd/server/`:

| File | Contents |
|------|----------|
| `config.go` | `Config` struct, `loadConfig()` from env vars |
| `routes.go` | Router setup, middleware chain, route registration |
| `main.go` | Entry point (~40-50 lines) |

**Internal fixes:**
- **consumer/consumer.go (169 lines):** Extract per-type message handlers (`handleOrder`, `handleCart`, `handleView`) to reduce mixed concerns
- Move inline `corsMiddleware()` (23 lines) from main.go to a middleware location
- **metrics/prometheus.go:** Wire up the unused `EventsConsumed` and `AggregationLatency` metrics into `consumer.go` (instrument Kafka event processing and aggregation timing)

**Files modified:** `cmd/server/main.go` (split into 3), `internal/consumer/consumer.go`, `internal/metrics/prometheus.go`

---

## Verification

1. `make preflight-go` — all lint checks and tests pass for all four services
2. No public API changes — all HTTP endpoints, request/response formats unchanged
3. Each new file has a clear single purpose and is under 150 lines
4. Each `main.go` is under 80 lines after the split
