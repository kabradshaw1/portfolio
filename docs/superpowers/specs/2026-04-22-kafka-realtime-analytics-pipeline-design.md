# Real-Time Analytics Pipeline — Kafka Stream Processing

**Date:** 2026-04-22
**Status:** Proposed
**Scope:** `go/analytics-service/`

## Context

The analytics-service currently consumes from 4 Kafka topics (`ecommerce.orders`, `.cart`, `.views`, `.payments`) but only logs events or maintains simple in-memory counters. There's no windowed aggregation, no partition-aware keying, no late event handling, and no durable state. This is the weakest part of the Kafka story — production ecommerce systems need real-time metrics for dashboards, alerting, and business intelligence.

This spec transforms the analytics-service into a stateful stream processor with three windowed aggregations, Redis-backed persistence, and REST endpoints for consumption.

## Goals

- Demonstrate production Kafka patterns: windowed aggregation, partition-aware keying, manual offset management, late event handling, consumer group rebalancing
- Replace in-memory counters with time-bucketed windows flushed to Redis
- Expose real-time analytics via REST endpoints
- Maintain the existing Prometheus metrics (consumer lag, events consumed, aggregation latency)

## Non-Goals

- Kafka Streams or third-party stream processing libraries (pure Go implementation)
- Frontend UI for the analytics dashboards (future work)
- Replacing the existing materialized views in order-service (those serve different reporting queries)

## Architecture

### Data Flow

```
Kafka Topics                    Analytics Service                     Redis
─────────────                   ─────────────────                     ─────
ecommerce.orders ──┐
ecommerce.cart   ──┤            ┌──────────────────┐
ecommerce.views  ──┼──────────> │  Kafka Consumer   │
ecommerce.payments─┘            │  (consumer group)  │
                                └────────┬─────────┘
                                         │ route by topic
                            ┌────────────┼────────────┐
                            v            v            v
                    ┌──────────┐  ┌──────────┐  ┌──────────┐
                    │ Revenue  │  │ Trending │  │  Cart    │
                    │ Window   │  │ Window   │  │ Abandon  │
                    │ (1h      │  │ (15min   │  │ Window   │
                    │ tumbling)│  │ sliding) │  │ (30min   │
                    └────┬─────┘  └────┬─────┘  │ session) │
                         │             │        └────┬─────┘
                         v             v             v
                    ┌─────────────────────────────────────┐
                    │           Redis Store                │
                    │  Hashes, Sorted Sets, TTL-managed   │
                    └──────────────────┬──────────────────┘
                                       │
                                       v
                              ┌─────────────────┐
                              │  REST Endpoints  │
                              │  /analytics/*    │
                              └─────────────────┘
```

### Window Types

#### 1. Revenue Per Hour (Tumbling Window)

- **Source topic:** `ecommerce.orders`
- **Event type:** `order.completed`
- **Window size:** 1 hour, aligned to clock hours (e.g., 14:00-15:00)
- **Key:** Event timestamp bucketed to hour
- **Aggregation:** Total revenue (cents), order count, average order value
- **Late event grace period:** 5 minutes after window close
- **Redis key:** `analytics:revenue:{YYYY-MM-DDTHH:00Z}` (Hash)
- **Redis fields:** `total_cents`, `order_count`, `avg_cents`
- **TTL:** 48 hours
- **Flush trigger:** Window close (when current event time exceeds window end + grace period), or periodic tick every 30 seconds for the active window

#### 2. Trending Products (Sliding Window)

- **Source topics:** `ecommerce.views` (weight 1) + `ecommerce.cart` `cart.item_added` events (weight 3)
- **Window size:** 15 minutes
- **Slide interval:** 1 minute (new window every minute, overlapping)
- **Key:** `productID`
- **Aggregation:** Weighted score = views + (3 * add-to-cart). Higher weight on cart adds since they signal stronger purchase intent.
- **Late event grace period:** 2 minutes
- **Redis key:** `analytics:trending:{YYYY-MM-DDTHH:MM:00Z}` (Sorted Set, score = weighted count)
- **TTL:** 2 hours
- **Flush trigger:** Every slide interval (1 minute)

#### 3. Cart Abandonment Rate (Fixed-Slot Window)

- **Source topics:** `ecommerce.cart` (`cart.item_added`) + `ecommerce.orders` (`order.completed`)
- **Window size:** 30 minutes
- **Key:** Event timestamp bucketed to 30-minute slots
- **Aggregation:** Count of unique users who added cart items (`started`), count who completed orders (`converted`), computed `abandoned = started - converted`, `rate = abandoned / started`
- **Late event grace period:** 5 minutes
- **Redis key:** `analytics:abandonment:{YYYY-MM-DDTHH:MM:00Z}` where MM is 00 or 30 (Hash)
- **Redis fields:** `started` (HyperLogLog or Set count of userIDs), `converted` (same), `abandoned`, `rate`
- **TTL:** 24 hours
- **Flush trigger:** Window close + periodic 30-second tick

### Kafka Patterns

#### Partition-Aware Keying

Current producers use arbitrary keys (order ID, user ID). For effective local aggregation:

- **order-service producer:** Key `ecommerce.orders` messages by `userID` (enables per-user session tracking in cart abandonment)
- **cart-service producer:** Key `ecommerce.cart` messages by `userID` (same user's cart events land on same partition)
- **ai-service producer:** Key `ecommerce.views` messages by `productID` (trending aggregation locality)

This is a **breaking change** to message key semantics. Existing messages in Kafka will have old keys, but since analytics doesn't maintain durable offsets across deploys (consumer group resets), this is safe.

#### Manual Offset Management

Current consumer commits after every message. New behavior:

- Commit offsets only after window state is flushed to Redis
- On flush success: commit the highest offset processed in that window
- On flush failure: do not advance offset (events replayed on restart)
- This gives **at-least-once processing** — duplicate events are handled by idempotent Redis writes

#### Late Event Handling

Each window type has a grace period. After the window's time boundary passes:

1. Window stays open for the grace period duration
2. Late events within grace period update the window and trigger a correction flush to Redis
3. After grace period expires, the window is closed and evicted from memory
4. Events arriving after grace period are dropped and counted in a `analytics_late_events_dropped_total` metric

#### Consumer Group Rebalancing

When partitions are reassigned (scale up/down, consumer crash):

1. On partition revoked: flush all in-progress windows for that partition to Redis, commit offsets
2. On partition assigned: load any existing window state from Redis for the new partitions (warm start)
3. This prevents data loss during rebalancing and minimizes duplicate processing

### Redis Storage

#### Key Schema

```
analytics:revenue:{ISO8601_hour}          → Hash {total_cents, order_count, avg_cents}
analytics:trending:{ISO8601_minute}       → Sorted Set {productID: score}
analytics:abandonment:{ISO8601_30min}     → Hash {started, converted, abandoned, rate}
analytics:abandonment:users:{window}:started   → Set of userIDs (for dedup)
analytics:abandonment:users:{window}:converted → Set of userIDs (for dedup)
```

#### TTL Management

- Revenue: 48h (enough for daily comparison dashboards)
- Trending: 2h (only recent trends matter)
- Abandonment: 24h (daily pattern analysis)
- User tracking sets: same TTL as parent abandonment window

### REST Endpoints

All endpoints on the analytics-service HTTP server (existing port 8094):

#### `GET /analytics/revenue?hours=24`

Returns hourly revenue for the last N hours.

```json
{
  "windows": [
    {
      "window_start": "2026-04-22T14:00:00Z",
      "window_end": "2026-04-22T15:00:00Z",
      "total_cents": 452300,
      "order_count": 47,
      "avg_order_value_cents": 9623
    }
  ]
}
```

#### `GET /analytics/trending?limit=10`

Returns top N trending products from the most recent completed window.

```json
{
  "window_end": "2026-04-22T15:45:00Z",
  "products": [
    {"product_id": "...", "score": 42, "views": 30, "cart_adds": 4},
    {"product_id": "...", "score": 28, "views": 22, "cart_adds": 2}
  ]
}
```

#### `GET /analytics/cart-abandonment?hours=12`

Returns abandonment rate per 30-minute window.

```json
{
  "windows": [
    {
      "window_start": "2026-04-22T14:30:00Z",
      "window_end": "2026-04-22T15:00:00Z",
      "carts_started": 85,
      "carts_converted": 47,
      "carts_abandoned": 38,
      "abandonment_rate": 0.447
    }
  ]
}
```

### New Packages

```
go/analytics-service/
├── internal/
│   ├── window/
│   │   ├── tumbling.go      # Tumbling window framework
│   │   ├── sliding.go       # Sliding window framework
│   │   ├── session.go       # Session/fixed-slot window framework
│   │   ├── clock.go         # Abstracted clock for testing
│   │   └── window_test.go   # Unit tests with mock clock
│   ├── aggregator/
│   │   ├── revenue.go       # Revenue per hour aggregator
│   │   ├── trending.go      # Trending products aggregator
│   │   ├── abandonment.go   # Cart abandonment aggregator
│   │   └── *_test.go        # Unit tests per aggregator
│   ├── store/
│   │   ├── redis.go         # Redis read/write for all window types
│   │   └── redis_test.go
│   ├── handler/
│   │   ├── analytics.go     # REST endpoints
│   │   └── analytics_test.go
│   ├── consumer/
│   │   ├── consumer.go      # Modified: routes to aggregators
│   │   └── consumer_test.go # Updated tests
│   └── metrics/
│       └── prometheus.go    # Extended: late events, flush latency, window counts
```

### Prometheus Metrics (new)

- `analytics_window_flushes_total{aggregator}` — Counter of window flushes to Redis
- `analytics_window_flush_latency_seconds{aggregator}` — Histogram of flush duration
- `analytics_late_events_dropped_total{aggregator}` — Counter of events past grace period
- `analytics_active_windows{aggregator}` — Gauge of currently open windows
- `analytics_events_processed_total{aggregator,event_type}` — Counter per aggregator (replaces existing per-topic counter)

### Producer Changes

**order-service** (`go/order-service/internal/kafka/producer.go`):
- Change `SafePublish` key for `ecommerce.orders` from `order.ID.String()` to `order.UserID.String()`

**cart-service** (`go/cart-service/internal/kafka/producer.go`):
- Change `SafePublish` key for `ecommerce.cart` from `userID.String()` to `userID.String()` (already correct)

**ai-service** (`go/ai-service/internal/kafka/producer.go`):
- Change `SafePublish` key for `ecommerce.views` from `p.ID` (product ID string) to `p.ID` (already correct for trending)

Only the order-service key changes.

### Configuration

New environment variables for analytics-service:

- `REDIS_URL` — Redis connection string (required, new dependency)
- `WINDOW_FLUSH_INTERVAL` — Periodic flush interval (default: 30s)
- `REVENUE_WINDOW_SIZE` — Revenue window duration (default: 1h)
- `TRENDING_WINDOW_SIZE` — Trending window duration (default: 15m)
- `TRENDING_SLIDE_INTERVAL` — Trending slide interval (default: 1m)
- `ABANDONMENT_WINDOW_SIZE` — Abandonment window duration (default: 30m)
- `LATE_EVENT_GRACE` — Grace period for late events (default: 5m)

## Testing Strategy

### Unit Tests

- **Window logic:** Open, close, late event admission, grace period expiry, eviction
- **Aggregators:** Each aggregator with mock clock and mock store — verify correct bucketing, scoring, rate calculation
- **Redis store:** Mock Redis client — verify key format, TTL, idempotent writes
- **REST handlers:** Mock store — verify JSON response format, query parameter parsing

### Integration Tests (testcontainers)

- **End-to-end:** Produce events to Kafka → consumer processes → verify Redis state → query REST endpoint
- **Late events:** Produce event after window close but within grace → verify correction flush
- **Rebalancing:** Start two consumers, stop one, verify window state preserved
- **Idempotency:** Produce duplicate events → verify Redis state unchanged after second processing

### Benchmarks

- **Throughput:** Events per second through each aggregator
- **Window management:** Memory usage with 100+ concurrent windows
- **Redis flush latency:** Under load with multiple aggregators flushing simultaneously

## Verification

1. Run `make preflight-go` — all lint and unit tests pass
2. Run integration tests with testcontainers (Kafka + Redis)
3. Produce test events via the ecommerce frontend (add to cart, checkout)
4. Query `/analytics/revenue`, `/analytics/trending`, `/analytics/cart-abandonment` endpoints
5. Verify Prometheus metrics appear in Grafana
6. Check consumer lag metric stays near zero under test load

## Roadmap Context

This is the first of four planned Kafka deepening initiatives:

1. **Real-time analytics pipeline** (this spec) — windowed aggregation, stateful processing
2. **Event sourcing + CQRS for orders** — immutable event log, read-side projections, compacted topics
3. **Inventory sync pipeline** — multi-consumer fan-out, exactly-once stock decrements, low-stock alerts
4. **CDC pipeline** — WAL-based change data capture, cross-service data synchronization

GitHub issues will be created for items 2-4 as future work.
