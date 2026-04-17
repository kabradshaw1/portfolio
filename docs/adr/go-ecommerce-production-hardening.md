# Go Ecommerce Service — Production Hardening

- **Date:** 2026-04-17
- **Status:** Accepted

## Context

The ecommerce service already had solid architecture: clean handler/service/repo layering, circuit breakers, distributed tracing, Prometheus metrics, structured logging, and async order processing via RabbitMQ. However, several gaps would be visible in a production code review or interview walkthrough:

1. **Input validation was scattered.** Some fields had Gin `binding` tags, some had manual UUID parsing in handlers, some had ad-hoc bounds checks. The sort parameter silently fell through to a default on invalid values. There were no structured field-level error responses — just a single error string.

2. **No idempotency protection.** `POST /orders` had no way to prevent duplicate orders on network retries. Cart's UNIQUE constraint accidentally handled duplicate adds via upsert, but that wasn't intentional idempotency.

3. **Offset-only pagination.** Products used `OFFSET`-based pagination, which scans O(n) rows at depth. Orders had no pagination at all — `ListByUser` returned every order unbounded.

4. **No integration tests.** All tests were unit tests with mocked repos. Nothing verified actual SQL queries, Redis caching behavior, or RabbitMQ message flow against real infrastructure.

Separately, we evaluated splitting cart into its own service to demonstrate microservice decomposition. We decided against it: the domain boundaries were already cleanly separated behind interfaces (each domain has its own handler/service/repo/model files), and the checkout flow's synchronous dependency on cart data made the split add latency and failure modes without meaningful benefit at this scale. The stronger signal for interviews is explaining *why not* to split — premature decomposition adds operational overhead without payoff.

## Decision

Four areas of hardening, in implementation order:

### 1. Structured Input Validation

**Approach:** A dedicated `internal/validate/` package with pure functions per request type, returning `[]apperror.FieldError`. The shared `apperror` package was extended with a `Validation()` constructor that returns HTTP 422 with a `fields` array.

**Why a validate package, not Gin binding tags?** Binding tags are limited to basic type checks and can't express cross-field rules or produce structured error responses. A validate package gives field-level error messages the frontend can map to specific form fields:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "validation failed",
    "fields": [
      {"field": "quantity", "message": "must be between 1 and 99"},
      {"field": "sort", "message": "must be one of: created_at_desc, price_asc, price_desc, name_asc"}
    ]
  }
}
```

**Why 422, not 400?** 400 means "malformed request" (can't parse). 422 means "parseable but semantically invalid" — the distinction matters for API clients deciding whether to retry with different values.

**What's validated:**
- `AddToCart`: productId (required + UUID), quantity (1–99)
- `UpdateCart`: quantity (1–99)
- `ProductListParams`: sort (whitelisted values), page (≥1), limit (1–100)
- `InitiateReturn`: itemIds (non-empty), reason (1–500 chars)

Sort parameter whitelisting was the most important gap — previously, an invalid sort value silently fell through to `ORDER BY created_at DESC`. Now it returns a 422.

### 2. Cursor-Based Pagination

**Approach:** Opaque cursors encoded as base64 JSON (`{"v": "<sort_value>", "id": "<uuid>"}`), with a `internal/pagination/` package for encode/decode. Products support both cursor and offset modes for backward compatibility; orders are cursor-only (they had no pagination before).

**Why cursor over offset?** `OFFSET 10000` scans and discards 10000 rows. Cursor pagination uses an index seek: `WHERE (sort_col, id) < ($cursor_val, $cursor_id)`, which is O(1) regardless of depth. For a product catalog that could grow, this matters.

**Why keep offset as a fallback?** The frontend currently uses page numbers. Ripping that out for cursor-only would be a breaking change for no user-facing benefit. If `cursor` param is provided, cursor mode is used; if `page` is provided, offset mode runs. Both modes return `nextCursor` and `hasMore` for forward compatibility.

**Sort-aware cursors:** The cursor value changes meaning based on sort:
- `created_at_desc`: timestamp (RFC3339Nano)
- `price_asc`/`price_desc`: integer (cents)
- `name_asc`: string

DESC sorts use `<` comparison, ASC sorts use `>`. Composite indexes `(price, id)`, `(name, id)`, and `(created_at DESC, id DESC)` were added via migration 005 to support efficient seeks.

**hasMore detection:** Fetch `limit + 1` rows. If you get more than `limit`, there are more pages — trim and encode the cursor from the last returned row. This avoids a separate `COUNT(*)` query.

### 3. Idempotency Keys

**Approach:** Gin middleware (`internal/middleware/idempotency.go`) backed by Redis. Applied per-route: required on `POST /orders` (checkout), optional on `POST /cart` (add item).

**Flow:**
1. Client sends `Idempotency-Key` header (UUID)
2. Middleware checks Redis for `idempotency:{userId}:{key}`
3. Key found + done → replay cached response (status code + body)
4. Key found + processing → 409 Conflict (concurrent duplicate)
5. Key not found → `SetNX` processing marker (30s TTL), run handler, cache response (24h TTL)

**Why Redis, not Postgres?** Idempotency keys are ephemeral (24h TTL), high-read, and need fast lookups. Redis is already in the stack for caching and rate limiting. A Postgres table would work but adds write load to the primary database for transient data.

**Why fail open?** If Redis is down, the middleware passes through and lets the request proceed without idempotency. This matches the rate limiter's pattern. The alternative — rejecting all writes when Redis is unavailable — would make Redis a hard dependency for the entire write path, which is worse than the small risk of a duplicate order during a Redis outage.

**Response capture:** The middleware wraps `gin.ResponseWriter` to tee the response body into a buffer, then caches both the status code and body in Redis after the handler completes. Only 2xx responses are cached; non-2xx responses delete the processing marker so the client can retry.

**Race safety:** Uses `SetNX` (set-if-not-exists) with a 30-second TTL for the processing marker. If two requests race, the second one sees the marker and gets a 409 instead of creating a duplicate.

### 4. Integration Tests with Testcontainers

**Approach:** `testcontainers-go` spins up real Postgres, Redis, and RabbitMQ containers per test suite. Tests use `//go:build integration` tags so they don't run during normal `go test`.

**Why testcontainers, not Docker Compose?** Testcontainers manages container lifecycle within the test process — start, seed, test, teardown. No external setup scripts, no port conflicts between parallel runs, no cleanup on test failure.

**Container lifecycle:** `TestMain` starts all three containers once, runs migrations, and shares them across tests. Each test truncates tables for isolation but doesn't restart containers — startup takes ~4 seconds, truncation takes milliseconds.

**What's tested (11 tests):**
- Repository CRUD: product list/find, cart add/upsert/update/remove, order create/find/status
- Checkout E2E: cart add → checkout → order in DB → cart cleared → message published
- Cursor pagination: walk all 25 products across 5 pages, verify no duplicates and correct ordering
- Idempotency: same key returns cached response (handler called once), different keys create separate resources
- Rate limiting: 5 requests succeed, 6th returns 429 with Retry-After header

**Colima compatibility:** On macOS with Colima, testcontainers needs `DOCKER_HOST=unix://$HOME/.colima/docker.sock` and `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`. The Makefile target `preflight-go-integration` sets both.

## Consequences

**Positive:**
- Field-level validation errors give the frontend actionable information per field, not a single error string
- Cursor pagination is O(1) at any depth, with backward-compatible offset fallback
- Idempotency keys prevent duplicate orders on network retries — the most common source of duplicate writes in real ecommerce
- Integration tests catch bugs that unit tests with mocks cannot: SQL syntax errors, JOIN correctness, upsert behavior, index effectiveness
- All four features are interview-ready talking points for Go microservice roles

**Trade-offs:**
- Idempotency adds a Redis round-trip to every checkout request (~1ms). Acceptable given the protection it provides.
- Cursor pagination is harder to reason about than offset for debugging ("show me page 47" requires walking from page 1). Offset mode remains available for debugging.
- Integration tests add ~8 seconds to a full test run (container startup). They're behind a build tag so they don't slow normal development.
- The `validate` package adds a layer between request binding and handler logic. This is intentional — it centralizes validation rules and makes them independently testable.
