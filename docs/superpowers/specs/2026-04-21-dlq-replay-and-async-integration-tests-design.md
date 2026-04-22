# DLQ Replay Tooling & Async Integration Tests

**Issues:** #97 (RabbitMQ DLQ replay tooling), #99 (Integration tests for RabbitMQ saga and Kafka consumer)
**Date:** 2026-04-21

## Context

The ecommerce checkout saga uses RabbitMQ for orchestration between order-service and cart-service, with a dead-letter exchange (`ecommerce.saga.dlx`) routing failed messages to `ecommerce.saga.dlq`. Currently there is no way to inspect or replay DLQ messages without manual queue manipulation via the RabbitMQ Management UI.

Separately, the async messaging flows (RabbitMQ saga and Kafka analytics consumers) have only unit tests with mocked interfaces — no integration tests that exercise real brokers. This is the most fragile part of the system and the hardest to test.

These two issues are tightly coupled: the best integration test for DLQ replay is one that exercises the full saga failure → DLQ → replay → success path.

## DLQ Replay Endpoints (Issue #97)

### Endpoints

Two REST endpoints in order-service, on an `/admin` route group. No auth — protected by network boundary (not exposed in ingress).

**`GET /admin/dlq/messages?limit=50`**
- Peeks at messages in `ecommerce.saga.dlq` using RabbitMQ `basic.get` with requeue
- Returns JSON array with: message ID (delivery tag), routing key, exchange, headers (including `x-death` metadata), timestamp, body, retry count
- Default limit 50, max 200

**`POST /admin/dlq/replay`**
- Request body: `{"index": 0}` — zero-based position in the DLQ (from the list response)
- Consumes messages from DLQ via `basic.get`, requeuing non-target messages, until reaching the target index
- Republishes target to its original exchange with original routing key (extracted from `x-death` headers)
- Increments `x-retry-count` header
- Records `saga_dlq_replayed_total` Prometheus counter
- Returns 200 with replayed message details, or 404 if index out of range

### Files

| File | Purpose |
|------|---------|
| `go/order-service/internal/saga/dlq.go` | DLQ client: `List()` and `Replay()` methods wrapping RabbitMQ channel |
| `go/order-service/internal/handler/admin.go` | Admin HTTP handlers (list, replay) |
| `go/order-service/internal/saga/metrics.go` | Add `saga_dlq_replayed_total` counter (labels: `routing_key`, `outcome`) |
| `go/order-service/cmd/server/main.go` | Register `/admin` route group |

### Message List Response Shape

```json
[
  {
    "index": 0,
    "routing_key": "saga.cart.commands",
    "exchange": "ecommerce.saga",
    "timestamp": "2026-04-21T10:30:00Z",
    "retry_count": 0,
    "headers": {
      "x-death": [...],
      "x-retry-count": 0
    },
    "body": { "order_id": "...", "command": "reserve.items", ... }
  }
]
```

### Replay Behavior

1. Iterate DLQ via `basic.get`, requeuing (nack with requeue) non-target messages
2. At the target index, ack the message (removing from DLQ)
3. Extract original exchange and routing key from `x-death[0]`
4. Republish with incremented `x-retry-count` header

## Integration Tests (Issue #99)

### RabbitMQ Saga Tests

**Location:** `go/order-service/internal/integration/saga_test.go`

Reuses existing testcontainers in `testutil/containers.go` (Postgres, Redis, RabbitMQ already configured).

**Tests:**

1. **`TestSaga_HappyPath`** — Create order → saga publishes `reserve.items` → test consumes from `saga.cart.commands` and verifies message → test publishes `items.reserved` back to `saga.order.events` → poll DB until order reaches next saga step

2. **`TestSaga_FailureToDLQ_Replay`** — Publish malformed message to `saga.cart.commands` → nack without requeue → verify message lands in `ecommerce.saga.dlq` via `GET /admin/dlq/messages` → call `POST /admin/dlq/replay` with `{"index": 0}` → verify message reappears on original queue

3. **`TestSaga_Compensation`** — Start checkout → simulate stock validation failure (mock product-service gRPC to return unavailable) → verify `release.items` command published → verify order reaches `COMPENSATION_COMPLETE`

**Setup:** Tests call `saga.DeclareTopology(ch)` on the test RabbitMQ channel to set up exchanges, queues, and DLX/DLQ bindings.

### Kafka Analytics Tests

**Location:** `go/analytics-service/internal/integration/consumer_test.go`

New testcontainers setup in `go/analytics-service/internal/integration/testutil/containers.go` — Kafka container using `testcontainers-go` Kafka module.

**Tests:**

1. **`TestConsumer_OrderEvent`** — Publish `order.created` to `ecommerce.orders` → poll until analytics aggregator records the order → verify count and data

2. **`TestConsumer_CartEvent`** — Publish `cart.item_added` to `ecommerce.cart` → verify cart aggregator updates

3. **`TestConsumer_ProductViewed`** — Publish multiple `product.viewed` events to `ecommerce.views` → verify trending aggregator counts

4. **`TestConsumer_TracePropagation`** — Publish event with injected W3C trace context → consume → verify extracted trace ID matches injected one

**Setup:** Create topics via Kafka admin client in TestMain. Use `kafka-go` writer for publishing test events.

### Build Tags and CI

- All integration tests use `//go:build integration`
- Testcontainers connect to Colima Docker socket (`DOCKER_HOST=unix://${HOME}/.colima/docker.sock`)
- Add `make integration-go` target if not already present, or extend existing integration test commands
- CI runs integration tests in the Go test job with `-tags=integration`

### Test Infrastructure Files

| File | Purpose |
|------|---------|
| `go/order-service/internal/integration/testutil/containers.go` | Existing — already has Postgres, Redis, RabbitMQ containers |
| `go/order-service/internal/integration/saga_test.go` | New — RabbitMQ saga + DLQ replay tests |
| `go/analytics-service/internal/integration/testutil/containers.go` | New — Kafka container setup |
| `go/analytics-service/internal/integration/testutil/helpers.go` | New — test helpers (publish events, poll aggregators) |
| `go/analytics-service/internal/integration/consumer_test.go` | New — Kafka consumer tests |

## Verification

1. **Unit tests:** `cd go/order-service && go test ./internal/saga/... ./internal/handler/...` — verify DLQ client and admin handler logic
2. **Integration tests (RabbitMQ):** `cd go/order-service && DOCKER_HOST=unix://${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...`
3. **Integration tests (Kafka):** `cd go/analytics-service && DOCKER_HOST=unix://${HOME}/.colima/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock go test -tags=integration -race -timeout 180s ./internal/integration/...`
4. **Manual DLQ test:** Start local stack (`docker compose up`), create an order, manually nack a message to force DLQ, `curl GET /admin/dlq/messages`, `curl POST /admin/dlq/replay/:id`
5. **Metrics:** Verify `saga_dlq_replayed_total` appears in `/metrics` output after replay
6. **Preflight:** `make preflight-go` passes (lint + unit tests)
7. **Smoke:** Existing smoke tests still pass (no changes to public-facing endpoints)
