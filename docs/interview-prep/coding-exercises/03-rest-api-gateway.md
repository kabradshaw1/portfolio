# REST API And Gateway Coding Exercises

### 1. Idempotent Create Endpoint

Prompt:

> Idempotent Create Endpoint


Time target: 40 minutes.

Build:

- `POST /orders`
- Requires `Idempotency-Key`.
- First request stores `processing`, then stores completed JSON response.
- Duplicate completed request returns the original response.
- Duplicate in-flight request returns `409 Conflict`.

Fast design:

- Redis versus database storage.
- TTL policy.
- Same key with different body.
- What happens if the service crashes after the side effect but before storing
  the completed response?

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 2. Middleware Chain

Prompt:

> Middleware Chain


Time target: 35 minutes.

Build middleware for:

- request ID
- logging
- panic recovery
- auth stub
- rate limit stub
- JSON error envelope

Fast design:

- Middleware order.
- `http.Handler` wrapping.
- When to stop the chain.
- Unit tests with `httptest`.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,

### 3. Cursor-Paginated List

Prompt:

> Cursor-Paginated List


Time target: 35 minutes.

Build:

- `GET /items?limit=20&cursor=...`
- Stable sort by `created_at` and `id`.
- Return `items` and `nextCursor`.
- Validate max limit.

Fast design:

- Why offset pagination breaks at scale.
- Cursor encoding.
- Stable ordering.
- Backward compatibility of cursor shape.

Repo anchors:
- `go/cart-service/cmd/server/routes.go` - route composition with recovery,
