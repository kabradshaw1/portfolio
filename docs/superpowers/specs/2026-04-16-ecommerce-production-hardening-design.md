# Ecommerce Service Production Hardening

## Context

The Go ecommerce service already has solid architecture: clean domain separation (Product/Cart/Order/Return), circuit breakers, distributed tracing, Prometheus metrics, structured logging, and async order processing via RabbitMQ. However, several production patterns are missing that would strengthen the service for real-world use and demonstrate production readiness for Go microservice roles. This spec covers four areas: input validation, idempotency, cursor-based pagination, and integration tests.

This is Phase 1 of a two-phase effort. A future Phase 2 may address cart service extraction if warranted.

## 1. Input Validation Layer

### Current State

Validation is scattered: Gin `binding` tags on some request structs, manual UUID parsing in handlers, ad-hoc bounds checks. The sort parameter in product listing silently falls through to a default on invalid values.

### Design

**New package:** `internal/validate/`

**Validator functions** — one per request type:
- `ValidateAddToCart(req) → []FieldError` — productId required + UUID, quantity 1–99
- `ValidateUpdateCart(req) → []FieldError` — quantity 1–99
- `ValidateProductListParams(params) → []FieldError` — sort must be one of `created_at_desc`, `price_asc`, `price_desc`, `name_asc`; limit 1–100; page ≥ 1
- `ValidateInitiateReturn(req) → []FieldError` — itemIds non-empty, reason 1–500 chars

**FieldError struct** (defined in `go/pkg/apperror/` to avoid circular imports between validate and apperror):
```go
type FieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}
```

**Error response format** (extends existing apperror pattern):
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "validation failed",
    "request_id": "...",
    "fields": [
      {"field": "quantity", "message": "must be between 1 and 99"},
      {"field": "sort", "message": "must be one of: created_at_desc, price_asc, price_desc, name_asc"}
    ]
  }
}
```

**Integration with apperror:** Add a `Validation(fields []FieldError)` constructor to `apperror` that returns a 422 status with the `fields` array. The existing `ErrorHandler` middleware serializes it — no new middleware needed.

**Handler changes:** Each handler calls the relevant validator before proceeding. If validation fails, `c.Error(apperror.Validation(fields))` and return.

### Files to Modify
- `go/pkg/apperror/apperror.go` — add `Validation()` constructor and `Fields` to `AppError`
- `go/ecommerce-service/internal/validate/validate.go` — new file, all validators
- `go/ecommerce-service/internal/validate/validate_test.go` — new file, unit tests
- `go/ecommerce-service/internal/handler/cart.go` — call validators
- `go/ecommerce-service/internal/handler/product.go` — call validators, remove inline bounds
- `go/ecommerce-service/internal/handler/order.go` — no changes (checkout has no request body)
- `go/ecommerce-service/internal/handler/return.go` — call validators

## 2. Idempotency Keys

### Current State

No duplicate protection on mutating endpoints. Network retries could create duplicate orders. Cart's UNIQUE constraint handles duplicate adds accidentally via upsert, but this isn't intentional idempotency.

### Design

**New middleware:** `internal/middleware/idempotency.go`

**Flow:**
1. Client sends `Idempotency-Key` header (UUID format)
2. Middleware checks Redis for `idempotency:{user_id}:{key}`
3. **Key found + response cached** → return cached response immediately (replay)
4. **Key found + in-progress marker** → return `409 Conflict` (concurrent duplicate)
5. **Key not found** → set in-progress marker (short TTL ~30s), proceed to handler, cache response on completion, set 24h TTL

**Redis key structure:**
- Key: `idempotency:{user_id}:{idempotency_key}`
- Value: JSON `{"status": "processing"}` or `{"status": "done", "status_code": 201, "body": "..."}`
- TTL: 30s for processing, 24h for completed

**Route application:**
- Required on `POST /orders` (checkout) — return 400 if missing
- Optional on `POST /cart` — if provided, enforces idempotency; if missing, normal flow
- Not applied to `PUT`, `DELETE`, or `GET` routes

**Circuit breaker:** Wrap Redis calls with the existing resilience package. If Redis is down, fail open (skip idempotency check, log warning) — same pattern as rate limiting.

**Response writer interception:** The middleware wraps `gin.ResponseWriter` to capture the response body and status code before they're flushed, then caches them in Redis.

### Files to Modify
- `go/ecommerce-service/internal/middleware/idempotency.go` — new file
- `go/ecommerce-service/internal/middleware/idempotency_test.go` — new file, unit tests
- `go/ecommerce-service/cmd/server/main.go` — apply middleware to checkout and cart-add routes

## 3. Cursor-Based Pagination

### Current State

Products use offset pagination (`page` + `limit`). Orders return all results unbounded. Neither supports cursors.

### Design

**New package:** `internal/pagination/`

**Cursor encoding:**
- Cursor = base64-encoded JSON `{"v": "<sort_value>", "id": "<uuid>"}`
- `EncodeCursor(sortValue string, id uuid.UUID) string`
- `DecodeCursor(cursor string) (sortValue string, id uuid.UUID, err error)`

**Product listing (`GET /products`):**
- New query params: `cursor` (opaque string), `limit` (1–100, default 20)
- If `cursor` is provided → cursor mode: `WHERE (sort_col, id) > ($val, $id) ORDER BY sort_col, id LIMIT $limit+1`
- If `page` is provided (no cursor) → offset mode (existing behavior, unchanged)
- Response adds `nextCursor` and `hasMore` fields:
  ```json
  {
    "products": [...],
    "total": 120,
    "page": 1,
    "limit": 20,
    "nextCursor": "eyJ2Ijoi...",
    "hasMore": true
  }
  ```
- `total` is only computed in offset mode (count queries are expensive with cursors)

**Order listing (`GET /orders`):**
- New query params: `cursor`, `limit` (1–50, default 20)
- Cursor keyed on `(created_at, id)` since orders are always sorted by creation date DESC
- Response:
  ```json
  {
    "orders": [...],
    "nextCursor": "eyJ2Ijoi...",
    "hasMore": true
  }
  ```

**SQL pattern (products, cursor mode):**
```sql
-- For created_at_desc (default sort):
SELECT ... FROM products
WHERE (created_at, id) < ($cursor_created_at, $cursor_id)
ORDER BY created_at DESC, id DESC
LIMIT $limit + 1
```

Note: for DESC ordering, the comparison operator is `<`, not `>`.

**Index requirements:** The existing indexes on `created_at` and `price` support this. Composite index `(created_at, id)` is already implicitly available since `id` is the primary key. For sorted queries (`price_asc`), add a composite index `(price, id)` for efficient cursor seeks.

### Files to Modify
- `go/ecommerce-service/internal/pagination/cursor.go` — new file, encode/decode
- `go/ecommerce-service/internal/pagination/cursor_test.go` — new file
- `go/ecommerce-service/internal/model/product.go` — add `Cursor`, `HasMore` to `ProductListParams` and response
- `go/ecommerce-service/internal/model/order.go` — add pagination params and response fields
- `go/ecommerce-service/internal/repository/product.go` — add cursor-mode query branch
- `go/ecommerce-service/internal/repository/order.go` — add `ListByUser` with cursor + limit
- `go/ecommerce-service/internal/handler/product.go` — parse cursor param, pass to service
- `go/ecommerce-service/internal/handler/order.go` — parse cursor/limit params
- `go/ecommerce-service/internal/service/order.go` — pass pagination params through
- `go/ecommerce-service/migrations/005_add_pagination_indexes.up.sql` — composite index on `(price, id)`
- `go/ecommerce-service/migrations/005_add_pagination_indexes.down.sql` — drop index

## 4. Integration Tests

### Current State

All tests are unit tests with mocked dependencies. No tests verify actual database queries, Redis caching, or RabbitMQ message flow.

### Design

**Framework:** `testcontainers-go` for Postgres, Redis, and RabbitMQ containers.

**Build tag:** `//go:build integration` — keeps integration tests separate from unit tests. Run with `go test -tags=integration -race ./...`.

**Test structure:**
```
go/ecommerce-service/
├── internal/
│   └── integration/              # new directory
│       ├── testutil/
│       │   ├── containers.go     # Postgres, Redis, RabbitMQ container setup
│       │   ├── db.go             # migration runner, test data seeder
│       │   └── helpers.go        # HTTP test client, assertion helpers
│       ├── repository_test.go    # Repository CRUD against real Postgres
│       ├── checkout_test.go      # Full checkout flow (cart → order → RabbitMQ)
│       ├── idempotency_test.go   # Idempotency key behavior against real Redis
│       ├── pagination_test.go    # Cursor pagination with real data
│       └── ratelimit_test.go     # Rate limiter against real Redis
```

**Container lifecycle:**
- `TestMain` starts containers once per test suite (shared across tests in the package)
- Each test gets a clean database (truncate tables between tests, not recreate containers)
- Containers are cleaned up via `t.Cleanup`

**Test coverage targets:**
- Repository: product CRUD, cart add/update/remove/clear, order create with items, return create
- Checkout flow: add items → checkout → verify order in DB → verify cart cleared → verify RabbitMQ message consumed
- Idempotency: same key returns cached 201, different key creates new order, expired key allows new request
- Pagination: 50 seeded products → cursor through pages → verify ordering + no duplicates + hasMore flag
- Rate limiting: burst 60 requests → 61st returns 429

**CI integration:** Add to `Makefile` as `preflight-go-integration` target. Add to CI pipeline as an optional job that runs when Go files change.

### Files to Create
- `go/ecommerce-service/internal/integration/testutil/containers.go`
- `go/ecommerce-service/internal/integration/testutil/db.go`
- `go/ecommerce-service/internal/integration/testutil/helpers.go`
- `go/ecommerce-service/internal/integration/repository_test.go`
- `go/ecommerce-service/internal/integration/checkout_test.go`
- `go/ecommerce-service/internal/integration/idempotency_test.go`
- `go/ecommerce-service/internal/integration/pagination_test.go`
- `go/ecommerce-service/internal/integration/ratelimit_test.go`

## Implementation Order

1. **Validation** — foundational, other features depend on clean error reporting
2. **Pagination** — changes repository interfaces, better to do before integration tests lock them in
3. **Idempotency** — middleware addition, independent of pagination
4. **Integration tests** — written last so they cover all new features

## Verification

- `make preflight-go` — existing lint + unit tests pass
- `go test -tags=integration -race ./internal/integration/...` — all integration tests pass
- Manual smoke test: `POST /orders` with same `Idempotency-Key` twice → second call returns cached response
- Manual smoke test: `GET /products?cursor=...&limit=5` → paginate through all products
- Manual smoke test: `POST /cart` with quantity 0 → `422` with field-level error
- CI pipeline green on feature branch
