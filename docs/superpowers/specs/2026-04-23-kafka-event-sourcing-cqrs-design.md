# Kafka Event Sourcing + CQRS for Orders

**Date:** 2026-04-23
**Status:** Draft
**Issue:** #140

## Context

The ecommerce order saga currently publishes a single `order.completed` event to Kafka for analytics consumption. All intermediate state transitions (created, reserved, payment initiated, etc.) are invisible to downstream consumers. This limits the system to point-in-time snapshots rather than a full event history.

This spec adds event sourcing to the order domain — publishing every state transition as an immutable event to a compacted Kafka topic — and a separate CQRS projection service that builds read-optimized views from the event stream. The goal is depth over breadth: demonstrating production-grade event sourcing with schema evolution, event replay, and eventual consistency handling.

## Scope

### In Scope

- Order-service publishes granular domain events to a new Kafka topic
- New `order-projector` Go service consuming events and building read models
- PostgreSQL-backed denormalized read models (timeline, summary, stats)
- Event replay (rebuild projections from scratch)
- Schema evolution with versioned JSON events
- Frontend: order event timeline UI, stats dashboard, replay/consistency indicators
- K8s deployment manifests, CI integration, monitoring

### Out of Scope

- Schema registry (Confluent/Avro/Protobuf) — using JSON with version field instead
- Changes to the existing RabbitMQ saga — event publishing is additive
- CDC (Change Data Capture) — separate initiative (#143)
- Multi-consumer fan-out patterns — separate initiative (#141)

## Event Schema

### Topic

- **Name:** `ecommerce.order-events`
- **Key:** `orderID` (string UUID)
- **Cleanup policy:** `compact` — retains latest event per key for state snapshots
- **Retention:** 7 days for non-compacted segments (configurable)

### Event Envelope

```json
{
  "id": "uuid-v4",
  "type": "order.created",
  "version": 1,
  "source": "order-service",
  "order_id": "uuid",
  "timestamp": "2026-04-23T14:01:03Z",
  "trace_id": "otel-trace-id",
  "data": {}
}
```

### Event Types

| Saga Step | Event Type | Version | Key Data Fields |
|-----------|-----------|---------|-----------------|
| Order placed | `order.created` | 1 | userID, items[], totalCents |
| Stock reserved | `order.reserved` | 1 | reservedItems[] |
| Payment initiated | `order.payment_initiated` | 1 | checkoutURL, paymentProvider |
| Payment confirmed | `order.payment_completed` | 1 | paymentID, amountCents |
| Order fulfilled | `order.completed` | 1 | completedAt |
| Saga failed | `order.failed` | 1 | failureReason, failedStep |
| Compensation | `order.cancelled` | 1 | cancelReason, refundStatus |

### Schema Evolution Strategy

Events carry a `version` field. The projector's deserializer has a registry of version upgraders that chain: v1 → v2 → v3. The projector always processes events in the latest version internally.

To demonstrate the pattern concretely, we ship v1 events initially and include one deliberate v2 upgrade — adding a `currency` field to `order.created` (defaulting to `"USD"` for v1 events). This ensures the upgrade path is exercised, not theoretical.

```go
// Example: v1 → v2 upgrader
func upgradeOrderCreatedV1toV2(data map[string]any) map[string]any {
    if _, ok := data["currency"]; !ok {
        data["currency"] = "USD"
    }
    return data
}
```

## Order-Service Changes

The order-service saga orchestrator publishes events at each state transition. This is **additive** — the existing RabbitMQ saga flow and the `ecommerce.orders` analytics event are unchanged.

**Changes:**
- New `internal/events/publisher.go` — wraps Kafka producer with event envelope construction
- New `internal/events/types.go` — event type constants and data structs
- Saga orchestrator calls `events.Publish()` at each step (after the step succeeds, before moving to the next)
- Uses `kafka.SafePublish()` pattern (fire-and-forget, logs errors) — event publishing must not block the saga

**Existing analytics event:** The current `order.completed` event on `ecommerce.orders` remains for backward compatibility with analytics-service. The new `ecommerce.order-events` topic is separate.

## Order-Projector Service

### Architecture

```
go/order-projector/
├── cmd/server/main.go              # Entrypoint, wires consumer + HTTP server
├── internal/
│   ├── consumer/
│   │   ├── consumer.go             # Kafka consumer, message loop, offset management
│   │   └── deserializer.go         # Version-aware JSON deserialization, upgrade chain
│   ├── projection/
│   │   ├── timeline.go             # Order timeline projection (full event history)
│   │   ├── summary.go              # Order summary projection (latest state)
│   │   └── stats.go                # Aggregate stats projection (hourly buckets)
│   ├── handler/
│   │   ├── timeline.go             # GET /orders/:id/timeline
│   │   ├── summary.go              # GET /orders/:id, GET /orders
│   │   └── stats.go                # GET /stats/orders
│   ├── replay/
│   │   └── replayer.go             # Reset offsets, truncate tables, rebuild projections
│   └── repository/
│       └── projector.go            # PostgreSQL read model storage, upserts
├── migrations/
│   ├── 001_create_read_models.up.sql
│   └── 001_create_read_models.down.sql
└── Dockerfile
```

### Consumer

- **Consumer group:** `order-projector-group` (independent from `analytics-group`)
- **Topic:** `ecommerce.order-events`
- **Library:** `segmentio/kafka-go` (consistent with analytics-service)
- **Commit strategy:** Commit after each batch is persisted to Postgres (at-least-once delivery)
- **Graceful shutdown:** Flush in-progress batch, commit offset, then exit

### Read Model Tables (projectordb)

**`order_timeline`** — one row per event, full audit trail:
```sql
CREATE TABLE order_timeline (
    event_id     UUID PRIMARY KEY,
    order_id     UUID NOT NULL,
    event_type   TEXT NOT NULL,
    event_version INT NOT NULL,
    data_json    JSONB NOT NULL,
    timestamp    TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_timeline_order_id ON order_timeline(order_id, timestamp);
```

**`order_summary`** — one row per order, latest state:
```sql
CREATE TABLE order_summary (
    order_id       UUID PRIMARY KEY,
    user_id        UUID NOT NULL,
    status         TEXT NOT NULL,
    total_cents    BIGINT NOT NULL,
    currency       TEXT NOT NULL DEFAULT 'USD',
    items_json     JSONB,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    completed_at   TIMESTAMPTZ,
    failure_reason TEXT
);
CREATE INDEX idx_summary_user_id ON order_summary(user_id);
CREATE INDEX idx_summary_status ON order_summary(status);
```

**`order_stats`** — hourly aggregation:
```sql
CREATE TABLE order_stats (
    hour_bucket           TIMESTAMPTZ PRIMARY KEY,
    orders_created        INT DEFAULT 0,
    orders_completed      INT DEFAULT 0,
    orders_failed         INT DEFAULT 0,
    avg_completion_seconds FLOAT,
    total_revenue_cents   BIGINT DEFAULT 0
);
```

### Idempotency

Each event's `id` (UUID) is the primary key in `order_timeline`. Duplicate events are skipped via `ON CONFLICT (event_id) DO NOTHING`. Summary and stats projections use upserts.

### Event Replay

**Endpoint:** `POST /admin/replay`

**Parameters:**
- `projection` (optional) — target specific projection (`timeline`, `summary`, `stats`, or `all`)

**Steps:**
1. Set `is_replaying = true` in `replay_status` table
2. Pause consumer
3. Truncate target read tables
4. Reset consumer group offset to earliest
5. Resume consumer — processes all events from the beginning
6. Track progress in `replay_status`: `events_processed`, `total_events`
7. Set `is_replaying = false` when caught up to latest offset

**`replay_status` table:**
```sql
CREATE TABLE replay_status (
    id                SERIAL PRIMARY KEY,
    is_replaying      BOOLEAN DEFAULT FALSE,
    projection        TEXT,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    events_processed  BIGINT DEFAULT 0,
    total_events      BIGINT DEFAULT 0
);
```

### Eventual Consistency

- Read endpoints return `X-Projection-Lag` header: difference between latest consumed event timestamp and current time
- Health endpoint reports replay state and consumer lag
- Frontend uses these signals to show staleness indicators

### HTTP Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/orders/:id/timeline` | Full event history for an order |
| GET | `/orders/:id` | Current order summary (projected state) |
| GET | `/orders` | List order summaries (paginated, filterable by status/user) |
| GET | `/stats/orders` | Hourly aggregated order statistics |
| GET | `/health` | Consumer lag, replay status, DB connectivity |
| POST | `/admin/replay` | Trigger event replay (query param: `projection`) |

**Port:** 8097

## Frontend Integration

### Order Event Timeline

On the order detail page (accessible from `/go/ecommerce/orders`), add an "Event Timeline" section showing every state transition:

```
● Order Created          2026-04-23 14:01:03
  3 items, $45.97

● Stock Reserved         2026-04-23 14:01:04
  All items available

● Payment Initiated      2026-04-23 14:01:05
  Stripe checkout created

● Payment Completed      2026-04-23 14:01:42
  Payment confirmed, $45.97

● Order Completed        2026-04-23 14:01:43
  Fulfilled
```

Each event node is color-coded by type (green for success steps, yellow for in-progress, red for failures).

### Order Stats Dashboard

A stats card on the orders list page showing metrics from `/stats/orders`:
- Orders today / completion rate / avg fulfillment time
- Small sparkline or bar chart for orders over the last 24 hours

### Consistency Indicators

- If `X-Projection-Lag` exceeds a threshold (e.g., 5 seconds), show a subtle banner: "Data may be slightly behind — projections updating"
- During replay, show: "Read models rebuilding — data may be incomplete"

### API Routing

- NGINX/Ingress path prefix: `/go-projector/*` → `order-projector:8097`
- Frontend API client: new `projectorApi` in `frontend/src/lib/`

## Infrastructure

### Kubernetes

- Deployment in `go-ecommerce` namespace (and `go-ecommerce-qa` for QA)
- New manifests: deployment, service, configmap, migration job
- Kafka topic creation: init container or startup script in the projector deployment
- New database: `projectordb` on the shared PostgreSQL instance
- mTLS: not needed (no gRPC — HTTP-only service)

### CI/CD

- Add to `ci.yml`: lint, test, build, deploy steps (follows existing Go service pattern)
- Add to `deploy.sh`: apply projector manifests
- Preflight: `make preflight-go` covers the new service

### Monitoring

- Prometheus metrics: `projector_events_consumed_total`, `projector_projection_lag_seconds`, `projector_replay_in_progress`
- Grafana dashboard panel in the existing ecommerce dashboard
- Loki: structured logging consistent with other Go services

## Verification Plan

1. **Unit tests:** Deserializer version upgraders, projection logic, idempotency
2. **Integration tests:** Consumer → Postgres pipeline with test Kafka broker
3. **Preflight:** `make preflight-go` passes (lint + tests)
4. **E2E flow:**
   - Create an order through the checkout flow
   - Verify events appear in `ecommerce.order-events` topic
   - Verify timeline endpoint returns all saga steps
   - Verify summary endpoint reflects latest order state
   - Trigger replay and verify read models rebuild correctly
5. **Frontend:** Timeline renders on order detail page, stats card shows on orders list
6. **Schema evolution:** Publish a v1 event, upgrade schema to v2, verify projector handles both
