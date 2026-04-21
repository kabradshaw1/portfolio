# Phase 3: Order Service + Saga Orchestrator

## Context

Final phase of the ecommerce decomposition roadmap (see `2026-04-20-ecommerce-decomposition-grpc-design.md`). Renames the now-order-only `ecommerce-service` to `order-service`, adds a RabbitMQ-based saga orchestrator for the checkout flow, and implements reservation semantics in cart-service. Completes the decomposition — after this phase, the original monolithic ecommerce-service is fully retired.

Phase 1 (product-service) and Phase 2 (cart-service) are deployed and running. The ecommerce-service currently contains only order/return logic with gRPC clients to cart-service and product-service.

## Design Decisions

- **No payment step:** The saga is reserve → stock check → confirm → clear cart. No simulated payment gateway — the portfolio value is in the saga orchestration pattern itself.
- **Rename, don't recreate:** Ecommerce-service is renamed to order-service rather than scaffolding a new service. Preserves git history, less error-prone.
- **Database polling for saga recovery:** On startup, query orders with incomplete `saga_step` and resume from last known state. Simple, reliable, no additional infrastructure.
- **Hybrid RabbitMQ + gRPC:** Saga commands/events via RabbitMQ (reserve, release, clear) demonstrate async orchestration. Stock check via gRPC (sync query, not a state change). Shows understanding of when each pattern is appropriate.
- **DLQ infrastructure only, no replay tooling:** Dead letter exchange, queue, and retry policy are set up. Admin replay endpoint is a future enhancement (issue #97).
- **Two sub-phases:** Sub-phase A renames ecommerce→order (mechanical, no logic changes). Sub-phase B adds saga orchestrator (logic changes). De-risks by catching rename issues before adding saga complexity.

## Sub-phase A: Rename Ecommerce-Service to Order-Service

### Scope

All mechanical renames — no logic changes. Everything works identically after this step.

- `go/ecommerce-service/` → `go/order-service/`
- Module path: `github.com/kabradshaw1/portfolio/go/order-service`
- Docker image: `go-order-service`
- K8s manifests: deployment `go-order-service`, service, configmap `order-service-config`, migration job `go-order-migrate`, HPA `go-order-hpa`, PDB `go-order-pdb`
- Ingress: `/go-api` → `/go-orders` path rewrite to order-service:8092
- QA overlay: all ecommerce-service patches renamed to order-service
- CI matrices: replace `ecommerce-service` with `order-service` in lint, test, build, hadolint, deploy
- `deploy.sh`: update `kubectl wait` deployment names
- Makefile: update `preflight-go` target
- Frontend: `GO_ECOMMERCE_URL` → `GO_ORDER_URL`, `NEXT_PUBLIC_GO_ECOMMERCE_URL` → `NEXT_PUBLIC_GO_ORDER_URL`
- `go-api.ts` renamed to `go-order-api.ts`, all order API calls use `/go-orders` path
- Smoke tests: update order endpoint paths from `/go-api/orders` to `/go-orders/orders`
- Other services with replace directives (cart-service, product-service go.mod, Dockerfiles): update `ecommerce-service` → `order-service`
- AI-service tools: update any ecommerce-service URLs to order-service

### Verification

All existing smoke tests pass with renamed paths. No behavior changes.

## Sub-phase B: Saga Orchestrator

### Saga Flow — Happy Path

1. Client `POST /orders/checkout`
2. Order-service gets cart items via gRPC (`cart-service.GetCart`) + enriches with product prices via gRPC (`product-service.GetProduct`)
3. Creates order in DB (status: `PENDING`, saga_step: `CREATED`)
4. Returns order to client immediately with status `PENDING`
5. Publishes `reserve.items` command to RabbitMQ → `saga.cart.commands` queue
6. Cart-service consumes, reserves items (sets `reserved = true`), replies `items.reserved` → `saga.order.events` queue
7. Order-service consumes reply, updates saga_step: `ITEMS_RESERVED`
8. Calls `product-service.CheckAvailability` via gRPC for each item (sync stock validation)
9. Updates order status: `CONFIRMED`, saga_step: `STOCK_VALIDATED`
10. Publishes `clear.cart` command → `saga.cart.commands`
11. Cart-service clears cart, replies `cart.cleared`
12. Order finalizes: status `COMPLETED`, saga_step: `COMPLETED`
13. Publishes Kafka event `order.created` for analytics (existing pattern)

### Saga Flow — Compensation (stock insufficient at step 8)

1. Order marked `FAILED`, saga_step: `COMPENSATING`
2. Publishes `release.items` command → `saga.cart.commands`
3. Cart-service releases reserved items (sets `reserved = false`), replies `items.released`
4. Order saga_step: `COMPENSATION_COMPLETE`

### Saga State Machine

Column added to orders table: `saga_step TEXT NOT NULL DEFAULT 'CREATED'`

States: `CREATED` → `ITEMS_RESERVED` → `STOCK_VALIDATED` → `COMPLETED`
Compensation: `COMPENSATING` → `COMPENSATION_COMPLETE`
Terminal: `FAILED`

On startup, order-service queries `SELECT * FROM orders WHERE saga_step NOT IN ('COMPLETED', 'COMPENSATION_COMPLETE', 'FAILED')` and resumes each from its last known step.

### RabbitMQ Topology

- **Exchange:** `ecommerce.saga` (topic, durable)
- **Command queue:** `saga.cart.commands` — cart-service consumes
- **Event queue:** `saga.order.events` — order-service consumes
- **Dead letter exchange:** `ecommerce.saga.dlx` (fanout, durable)
- **Dead letter queue:** `ecommerce.saga.dlq` — failed messages after retry exhaustion
- **Retry:** 3 attempts, exponential backoff (1s, 5s, 25s) via message headers (`x-retry-count`) and retry exchange with per-message TTL

### Message Format

Commands:
```json
{
  "command": "reserve.items",
  "order_id": "uuid",
  "user_id": "uuid",
  "items": [{"product_id": "uuid", "quantity": 2}],
  "trace_id": "w3c-trace-id",
  "timestamp": "2026-04-21T..."
}
```

Replies use `"event"` instead of `"command"` (e.g., `"event": "items.reserved"`).

### Cart-Service Modifications

**Database migration** `002_add_reserved_column.up.sql`:
```sql
ALTER TABLE cart_items ADD COLUMN reserved BOOLEAN NOT NULL DEFAULT false;
```

**New dependencies:** RabbitMQ connection (`RABBITMQ_URL` config).

**New code:**
- `internal/worker/saga_handler.go` — RabbitMQ consumer on `saga.cart.commands`, dispatches to service methods, publishes replies to `saga.order.events`
- `internal/service/cart.go` — add `ReserveItems(ctx, userID, orderID)`, `ReleaseItems(ctx, userID, orderID)` methods
- `internal/repository/cart.go` — add `Reserve(ctx, userID)`, `Release(ctx, userID)` queries toggling the `reserved` column
- gRPC stubs (`ReserveItems`, `ReleaseItems`) wired to real service methods

**REST handler guard:** UpdateQuantity and RemoveItem reject changes to reserved items (409 Conflict):
```sql
UPDATE cart_items SET quantity = $1 WHERE id = $2 AND user_id = $3 AND reserved = false
```

### Order-Service New Code

**Database migration:** adds `saga_step` column:
```sql
ALTER TABLE orders ADD COLUMN saga_step TEXT NOT NULL DEFAULT 'CREATED';
```

**Saga orchestrator:**
- `internal/saga/orchestrator.go` — core state machine. Reads current `saga_step`, executes next action, updates step. Methods: `handleCreated()`, `handleItemsReserved()`, `handleStockValidated()`, etc.
- `internal/saga/consumer.go` — RabbitMQ consumer on `saga.order.events`. Parses reply events, advances saga.
- `internal/saga/recovery.go` — startup recovery. Queries incomplete orders, feeds each to orchestrator.
- `internal/saga/publisher.go` — RabbitMQ publishing with trace context, retry headers, structured logging.

**Existing code changes:**
- `internal/service/order.go` `Checkout()` — simplified. Creates order with `saga_step: CREATED`, kicks off saga (async), returns order immediately as `PENDING`.
- Remove `internal/worker/` — replaced by saga orchestrator.

### Proto Definition

`go/proto/order/v1/order.proto` with `GetOrder` and `ListOrders` RPCs. Minimal — completes the gRPC pattern across all three decomposed services.

### Order-Service Dual Server

After sub-phase B, order-service runs:
- **REST** on `:8092` — frontend checkout, order listing
- **gRPC** on `:9092` — inter-service `GetOrder`, `ListOrders`
- **RabbitMQ consumer** — saga event replies
- **RabbitMQ publisher** — saga commands

### Observability

- **Prometheus metrics:** `saga_steps_total{step,outcome}` (counter), `saga_duration_seconds` (histogram), `saga_dlq_messages_total` (counter)
- **Structured logs:** at each saga transition with `traceID`, `orderID`, `sagaStep`, `nextStep`
- **Jaeger waterfall:** REST → order creation → RabbitMQ reserve → cart reservation → gRPC stock check → RabbitMQ clear → completion. Each saga step gets a child span.

### K8s Changes

- Order-service deployment gains gRPC port 9092
- Order-service service gets second port (grpc: 9092)
- Cart-service configmap gains `RABBITMQ_URL`
- Cart-service deployment needs RabbitMQ connectivity
- QA overlay patches for new env vars

### Frontend Changes

No frontend logic changes needed for the saga — checkout still calls `POST /orders/checkout` and gets back an order. The saga runs server-side. The only frontend change is the URL rename from sub-phase A.

### Verification

1. **Full checkout:** POST /orders/checkout → Jaeger waterfall shows all saga steps
2. **Compensation:** Intentionally fail stock check → verify items released, order marked FAILED
3. **Recovery:** Kill order-service mid-saga → restart → saga resumes from last step
4. **DLQ:** Poison a saga message → verify it lands in `ecommerce.saga.dlq`
5. **Cart guard:** Try to update a reserved cart item → 409 Conflict
6. **Metrics:** `/metrics` shows saga counters/histograms
7. **Kafka:** analytics-service still receives `order.created` events
8. **CI:** all matrix jobs pass with renamed service
9. **Smoke tests:** full checkout lifecycle passes with `/go-orders` path

## 15-Step Checklist (per CLAUDE.md)

Applied to order-service (sub-phase A rename + sub-phase B additions):

1. Service code (`go/order-service/` — renamed from ecommerce-service)
2. Proto (`go/proto/order/v1/order.proto`), `buf generate`
3. Cross-module imports (cart-service and product-service go.mod update `ecommerce-service` → `order-service` replace directives, Dockerfiles update COPY)
4. Seed data — existing `seed.sql` carries over from ecommerce-service
5. K8s manifests (all renamed, gRPC port added)
6. QA database — `ecommercedb_qa` stays (just rename the service, not the database)
7. QA Kustomize overlay (all ecommerce patches → order-service patches, add RABBITMQ_URL to cart-service)
8. CI matrices (replace ecommerce-service with order-service everywhere)
9. CI deploy steps (migration job names updated)
10. `deploy.sh` (`kubectl wait` names updated)
11. Ingress (`/go-orders` replaces `/go-api`)
12. Frontend (`NEXT_PUBLIC_GO_ORDER_URL` in Vercel before merge)
13. Smoke tests (update all order endpoint paths)
14. Makefile (`preflight-go` target updated)
15. Migration state — existing `ecommerce_schema_migrations` table continues working (just the service name changes, not the database or migration table)
