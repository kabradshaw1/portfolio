# DLQ Replay Tooling & Async Integration Tests

- **Date:** 2026-04-21
- **Status:** Accepted

## Context

The ecommerce checkout saga uses RabbitMQ for orchestration between order-service and cart-service, with a dead-letter exchange (`ecommerce.saga.dlx`) routing failed messages to `ecommerce.saga.dlq`. When a saga message is nacked without requeue — due to malformed payloads, transient downstream failures, or bugs — it lands in the DLQ with no way to inspect or replay it without manual queue manipulation via the RabbitMQ Management UI.

Separately, the async messaging flows (RabbitMQ saga and Kafka analytics consumers) only had unit tests with mocked interfaces. The saga orchestrator, publisher, and consumer were tested in isolation but never exercised through a live broker. This is the most fragile part of the system — serialization mismatches, routing key typos, and DLX misconfiguration are invisible to unit tests.

Two constraints shaped the design:

1. **Portfolio project** — the solution needs to demonstrate operational maturity (not just "I have a DLQ") and be explainable in an interview setting.
2. **No new services** — adding a standalone admin CLI or separate DLQ service would increase deployment complexity without proportional benefit.

## Decision

### DLQ Replay: REST endpoints in order-service

We added two admin REST endpoints to order-service rather than a standalone CLI tool or a separate admin service:

- `GET /admin/dlq/messages?limit=50` — peeks at DLQ contents via `basic.get` with immediate nack+requeue, so listing doesn't consume messages.
- `POST /admin/dlq/replay` with `{"index": 0}` — walks the DLQ to the target position, acks the target (removing it), and republishes to the original exchange/routing key extracted from RabbitMQ's `x-death` header. Increments an `x-retry-count` header on each replay.

**Why REST over CLI:** Endpoints are visible in the portfolio, testable via integration tests, and could be surfaced in a frontend admin panel later. A CLI would require a separate binary and deployment path.

**Why no auth:** The `/admin` route group is not exposed through the Kubernetes ingress. It's only reachable from within the cluster, which is how most teams handle internal admin tooling. Adding JWT auth or role-based access would require auth-service changes (role claims) for no security benefit in this topology.

**Why index-based replay (not message ID):** RabbitMQ delivery tags are channel-scoped and ephemeral — they change on each `basic.get` call. The alternative would be matching on a custom header or body field, but that couples the replay mechanism to message content. Index-based addressing is simple and sufficient for a DLQ that typically has few messages.

**Trade-off: sequential scan for replay.** Replaying message at index N requires consuming and requeuing messages 0 through N-1. This is O(N) and temporarily reorders the DLQ. Acceptable because DLQs are small (if they're large, you have bigger problems), and the alternative — RabbitMQ's management HTTP API with `ackmode=ack_requeue_true` — has its own complexity and isn't available from the AMQP protocol.

### Integration tests: testcontainers, not docker-compose

We used `testcontainers-go` to spin up real RabbitMQ and Kafka brokers in integration tests, gated behind `//go:build integration`:

**RabbitMQ saga tests (order-service):**
- Happy path: checkout → reserve.items command → items.reserved reply → clear.cart → cart.cleared → COMPLETED
- DLQ replay: publish → nack → verify in DLQ → replay → verify on original queue
- Compensation: stock failure → release.items → COMPENSATION_COMPLETE

**Kafka consumer tests (analytics-service):**
- Order, cart, and product view event consumption with aggregator verification
- Trace propagation (W3C traceparent header round-trips through Kafka)

**Why testcontainers over docker-compose:** Testcontainers are self-contained — each test run creates and destroys its own infrastructure. No port conflicts, no stale state, no "did you remember to `docker compose up` first?" Docker-compose is better for local development; testcontainers are better for CI.

**Why not mock the brokers:** The whole point is to catch the bugs that mocks hide — serialization mismatches, routing key typos, DLX binding errors, consumer group coordination issues. The existing unit tests already cover the business logic with mocks. Integration tests exist to verify the plumbing.

**Trade-off: slow tests.** Kafka containers take 15-30s to start. RabbitMQ is faster (~5s). Both are gated behind the `integration` build tag so they don't slow down `go test ./...` or CI's unit test jobs. The `preflight-go-integration` Makefile target runs them explicitly.

### Observability

Added `saga_dlq_replayed_total` Prometheus counter (labels: `routing_key`, `outcome`) to track replay operations. This complements the existing `saga_dlq_messages_total` counter (messages entering the DLQ) and provides the full lifecycle: messages in → messages replayed.

## Consequences

**Positive:**
- Operators can inspect and replay failed saga messages without RabbitMQ Management UI access or manual `rabbitmqctl` commands.
- Integration tests catch a class of bugs that unit tests with mocked interfaces cannot — the DLQ replay test itself proved the full nack → DLX → DLQ → replay → republish flow works end-to-end.
- The `x-retry-count` header provides visibility into how many times a message has been replayed, useful for identifying messages that repeatedly fail.
- Kafka consumer tests verify that the analytics pipeline correctly parses the event envelope format and routes to the right aggregator.

**Trade-offs:**
- The admin endpoints share the order-service's RabbitMQ channel. Under high saga throughput, DLQ listing could briefly compete for channel bandwidth. In practice this is negligible — admin endpoints are used rarely.
- Integration tests add ~50 testcontainers dependencies to each service's `go.mod`. These are test-only but increase the module graph.
- The index-based replay mechanism is positional, not idempotent — replaying index 0 twice replays two different messages if the first replay succeeded. This is acceptable for manual operator use but would need redesign for automated retry systems.
