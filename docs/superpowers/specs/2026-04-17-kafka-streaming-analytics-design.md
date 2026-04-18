# Kafka Streaming Analytics Pipeline — Design Spec

## Context

Kyle is applying for a Gen AI Engineer role that values Kafka streaming experience. The existing Go ecommerce stack uses RabbitMQ for order processing. This spec adds Kafka as a streaming layer for real-time analytics — a new use case that complements (not replaces) RabbitMQ, demonstrating end-to-end streaming pipeline skills: producers, consumer groups, topic partitioning, and windowed aggregation.

## Goals

- Demonstrate Kafka streaming pipeline skills in a production-like Go microservices context
- Add real-time ecommerce analytics (trending products, order velocity, revenue) that don't exist today
- Keep RabbitMQ for order processing (proven, working)
- Follow all existing project patterns (resilience, tracing, K8s manifests, testing)

## Non-Goals

- Replacing RabbitMQ with Kafka
- Historical analytics / persistent storage (the Java activity-service covers project-level historical stats)
- Multi-broker Kafka cluster (single KRaft node is sufficient for a portfolio project)

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│ ecommerce-svc   │────▶│  Kafka (KRaft)   │────▶│  analytics-service  │
│ (producer)       │     │  Single broker   │     │  (consumer group)   │
└─────────────────┘     │                  │     │                     │
┌─────────────────┐     │  Topics:         │     │  In-memory windows  │
│ ai-service      │────▶│  ecommerce.orders│     │  REST endpoints     │
│ (producer)       │     │  ecommerce.cart  │     │  Prometheus metrics │
└─────────────────┘     │  ecommerce.views │     └─────────────────────┘
                        └──────────────────┘               │
                                                           ▼
                                                  ┌─────────────────┐
                                                  │ Frontend         │
                                                  │ /go/analytics    │
                                                  └─────────────────┘
```

RabbitMQ continues to handle `order.created` → order processing worker (stock decrement, status updates). Kafka handles the analytics event stream in parallel.

---

## Kafka Topics

| Topic | Partition Key | Producer | Events |
|-------|--------------|----------|--------|
| `ecommerce.orders` | `orderID` | ecommerce-service | `order.created`, `order.completed`, `order.failed` |
| `ecommerce.cart` | `userID` | ecommerce-service | `cart.item_added`, `cart.item_removed` |
| `ecommerce.views` | `productID` | ai-service | `product.viewed` |

Each topic starts with 3 partitions — enough to demonstrate partitioning without overhead.

### Event Envelope

All events follow a common envelope:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "order.created",
  "source": "ecommerce-service",
  "timestamp": "2026-04-17T12:00:00Z",
  "traceID": "abc123",
  "data": {
    "orderID": "...",
    "userID": "...",
    "totalCents": 4999,
    "items": [{"productID": "...", "quantity": 2, "priceCents": 2499}]
  }
}
```

The `data` field varies by event type:
- **order.created/completed/failed:** orderID, userID, totalCents, items (array of productID/quantity/priceCents), status
- **cart.item_added/removed:** userID, productID, quantity, productName
- **product.viewed:** productID, userID, productName, source ("search" or "detail")

---

## Producer Integration

### ecommerce-service

New `internal/kafka` package:

```go
type Producer interface {
    Publish(ctx context.Context, topic string, key string, event Event) error
    Close() error
}
```

Implementation uses `github.com/segmentio/kafka-go` Writer with:
- Async writes (non-blocking, errors logged)
- Batch size: 100 messages or 1 second flush interval
- OpenTelemetry trace context injected into Kafka headers (mirrors existing `tracing.InjectAMQP`)

**Integration points:**
- `OrderService.Checkout()` — after publishing to RabbitMQ, also publish `order.created` to Kafka
- `OrderProcessor.ProcessOrder()` — publish `order.completed` or `order.failed` after processing
- `CartService.AddItem()` / `RemoveItem()` — publish cart events
- Producer is injected via interface (same pattern as `OrderPublisher` for RabbitMQ)

**Graceful degradation:** If Kafka is unavailable, events are dropped with error logging. The primary flow (RabbitMQ order processing) is never affected.

### ai-service

- `search_products` and `get_product` tools publish `product.viewed` events
- Same Producer interface, injected into tool constructors
- Optional — if Kafka unavailable, tool still works normally

---

## Analytics Service

### Service Structure

```
go/analytics-service/
├── cmd/
│   └── main.go              # Startup, DI, graceful shutdown
├── internal/
│   ├── consumer/
│   │   ├── consumer.go       # Kafka consumer group, message routing
│   │   └── consumer_test.go
│   ├── aggregator/
│   │   ├── orders.go         # Order volume, revenue, status breakdown
│   │   ├── trending.go       # Trending products by views + purchases
│   │   ├── carts.go          # Active cart tracking
│   │   ├── window.go         # Generic sliding window data structure
│   │   └── *_test.go
│   ├── handler/
│   │   ├── analytics.go      # HTTP handlers
│   │   └── analytics_test.go
│   └── metrics/
│       └── prometheus.go     # Custom collectors
├── Dockerfile
├── go.mod
└── go.sum
```

### Consumer Group

- Group ID: `analytics-group`
- Subscribes to all three topics
- Uses `kafka-go` ConsumerGroup with manual commit (commit after successful processing)
- Messages are routed by topic to the appropriate aggregator
- OpenTelemetry trace context extracted from Kafka headers

### Aggregation

**Sliding window implementation:**
- Generic `Window[T]` struct: ring buffer of time-bucketed slots
- Each slot covers 1 minute
- Configurable total window duration (1h for trending, 24h for order stats)
- Old slots are lazily evicted on read
- Thread-safe via `sync.RWMutex`

**Aggregators:**

1. **OrderAggregator** (24h window):
   - Counts: orders created, completed, failed per minute
   - Revenue: total cents per minute
   - Exposes: orders/hour, revenue/hour, completion rate

2. **TrendingAggregator** (1h window):
   - Tracks product views + purchases
   - Scores: `views + (purchases × 5)` weighted
   - Exposes: top 10 products sorted by score

3. **CartAggregator** (1h window):
   - Tracks active carts (item_added increments, item_removed decrements)
   - Exposes: active cart count, most-added products

### REST Endpoints (port 8094)

| Endpoint | Response |
|----------|----------|
| `GET /analytics/dashboard` | `{ordersPerHour, revenuePerHour, activeCarts, completionRate}` |
| `GET /analytics/trending` | `{products: [{id, name, score, views, purchases}]}` |
| `GET /analytics/orders` | `{hourly: [{hour, count, revenue}], statusBreakdown: {completed, failed, pending}}` |
| `GET /health` | `{status: "ok", kafka: "connected"}` |
| `GET /metrics` | Prometheus format |

### Resilience

- Circuit breaker on Kafka consumer reconnection
- Graceful shutdown: stop consumer, flush pending commits, then stop HTTP server
- If Kafka is down, health endpoint reports degraded, analytics endpoints return last known data with a `stale: true` flag

---

## Infrastructure

### Kafka Broker

**KRaft mode** (no Zookeeper) — single node, sufficient for portfolio demonstration.

**Docker Compose** (`go/docker-compose.yml`):
```yaml
kafka:
  image: apache/kafka:3.7.0
  ports:
    - "9092:9092"
  environment:
    KAFKA_NODE_ID: 1
    KAFKA_PROCESS_ROLES: broker,controller
    KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093
    KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092
    KAFKA_CONTROLLER_QUORUM_VOTERS: 1@kafka:9093
    KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
    KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
    KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    KAFKA_LOG_DIRS: /tmp/kraft-combined-logs
  volumes:
    - kafka-data:/tmp/kraft-combined-logs
  healthcheck:
    test: ["CMD-SHELL", "/opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 > /dev/null 2>&1"]
    interval: 10s
    timeout: 5s
    retries: 5
```

**Kubernetes** (`go/k8s/statefulsets/kafka.yml`):
- StatefulSet, 1 replica, PVC for log storage
- Same KRaft configuration
- Service: `kafka.go-ecommerce.svc.cluster.local:9092`

### Analytics Service K8s Manifests

Following existing patterns exactly:
- `go/k8s/deployments/analytics-service.yml` — 1 replica, security context, probes on `/health`
- `go/k8s/services/analytics-service.yml` — ClusterIP port 8094
- `go/k8s/configmaps/analytics-service-config.yml` — `KAFKA_BROKERS`, `PORT`
- `go/k8s/pdb/analytics-pdb.yml` — maxUnavailable: 1
- Update `go/k8s/ingress.yml` — add `/go-analytics(/|$)(.*)` → analytics-service:8094
- Update `go/k8s/kustomization.yaml` — include all new resources

### CI/CD Updates

- New Dockerfile: `go/analytics-service/Dockerfile` (same multi-stage pattern as other Go services)
- Add `analytics-service` to CI build matrix in `.github/workflows/ci.yml`
- Add to `k8s/deploy.sh` for automated deployment
- Preflight: `make preflight-go` already covers `go/...` — new service is automatically included

---

## Frontend

New page at `/go/analytics` showing a real-time dashboard:
- **Order Velocity** — orders/hour line chart (polling every 5s)
- **Revenue** — revenue/hour display
- **Trending Products** — ranked list with view/purchase counts
- **Active Carts** — live count
- Uses existing shadcn/ui components + a lightweight chart library (recharts, already common with shadcn)
- Follows existing `/go/` page patterns (layout, navigation, styling)

---

## Testing Strategy

### Unit Tests
- `aggregator/*_test.go` — window bucketing, eviction, concurrent reads/writes, edge cases (empty window, single event)
- `handler/*_test.go` — HTTP response shapes, stale data flag
- `consumer/*_test.go` — message routing, envelope parsing

### Integration Tests
- testcontainers: spin up Kafka broker → produce events → verify analytics endpoints return correct aggregations
- Follows existing ecommerce-service integration test patterns (testcontainers for postgres/redis/rabbitmq)

### Benchmarks
- Aggregator throughput: events/second ingestion rate
- Window query performance under concurrent reads

---

## Verification Plan

1. **Local dev:** `docker compose up` in `go/` — Kafka starts, ecommerce produces events, analytics-service consumes and serves endpoints
2. **Manual test:** Create orders via ecommerce API → check `/analytics/dashboard` shows real-time updates
3. **Unit tests:** `go test ./analytics-service/...`
4. **Integration tests:** `go test -tags=integration ./analytics-service/...`
5. **K8s deploy:** Deploy to Minikube, verify analytics endpoints via ingress
6. **Frontend:** Visit `/go/analytics`, confirm live data updates
7. **CI:** All preflight checks pass (`make preflight-go`, `make preflight-frontend`)

---

## Key Files to Modify

| File | Change |
|------|--------|
| `go/ecommerce-service/internal/kafka/` | New producer package |
| `go/ecommerce-service/internal/service/order.go` | Add Kafka publish after RabbitMQ |
| `go/ecommerce-service/internal/service/cart.go` | Add Kafka publish on cart actions |
| `go/ecommerce-service/cmd/main.go` | Wire Kafka producer |
| `go/ecommerce-service/go.mod` | Add `segmentio/kafka-go` |
| `go/ai-service/internal/tools/ecommerce.go` | Publish product.viewed events |
| `go/ai-service/go.mod` | Add `segmentio/kafka-go` |
| `go/analytics-service/` | Entire new service |
| `go/docker-compose.yml` | Add Kafka + analytics-service containers |
| `go/k8s/` | New manifests for Kafka + analytics-service |
| `.github/workflows/ci.yml` | Add analytics-service to build matrix |
| `frontend/src/app/go/analytics/` | New analytics dashboard page |

## Existing Code to Reuse

| What | Where |
|------|-------|
| `apperror` package | `go/pkg/apperror/` — error handling for analytics handlers |
| `resilience` package | `go/pkg/resilience/` — circuit breaker for Kafka connections |
| `tracing` package | `go/pkg/tracing/` — extend with Kafka header inject/extract |
| RabbitMQ producer pattern | `go/ecommerce-service/internal/rabbitmq/` — mirror the interface/injection pattern for Kafka |
| K8s manifest templates | `go/k8s/deployments/ecommerce-service.yml` — copy and adapt for analytics-service |
| Dockerfile pattern | `go/ecommerce-service/Dockerfile` — same multi-stage build |
| testcontainers setup | `go/ecommerce-service/internal/repository/*_test.go` — same pattern for Kafka integration tests |
