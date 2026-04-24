# Kafka Real-Time Analytics Pipeline

- **Date:** 2026-04-22
- **Status:** Accepted
- **Service:** `go/analytics-service/`

## Overview

This document explains how the analytics-service implements a real-time stream processing pipeline over Apache Kafka in Go. The service consumes events from four Kafka topics, applies three types of windowed aggregation (tumbling, sliding, and fixed-slot), persists results to Redis, and exposes them via REST endpoints.

The key Kafka patterns demonstrated are: consumer groups with multi-topic subscription, partition-aware message keying, manual offset management, event timestamp processing (event time vs. processing time), late event handling with grace periods, periodic flush with at-least-once delivery semantics, and idempotent state writes.

This is a pure Go implementation — no stream processing framework (Kafka Streams, Flink, etc.) — which means every concept is visible in application code rather than hidden behind library abstractions.

## Architecture Context

The analytics-service sits at the end of the ecommerce event pipeline:

```
┌─────────────┐     ┌──────────────┐     ┌───────────────┐
│ order-service│     │ cart-service  │     │  ai-service   │
│  (checkout)  │     │ (add/remove) │     │ (product view)│
└──────┬───────┘     └──────┬───────┘     └──────┬────────┘
       │                    │                    │
       │ SafePublish        │ SafePublish        │ SafePublish
       │                    │                    │
       v                    v                    v
  ┌─────────────────────────────────────────────────────┐
  │                   Apache Kafka                       │
  │  Topics: ecommerce.orders, .cart, .views, .payments │
  │  KRaft mode (no Zookeeper), 1 broker                │
  └────────────────────────┬────────────────────────────┘
                           │
                    FetchMessage (consumer group: analytics-group)
                           │
                    ┌──────v──────┐
                    │  analytics- │
                    │   service   │
                    │             │
                    │  ┌────────┐ │    ┌───────┐
                    │  │Windows │─┼───>│ Redis │
                    │  └────────┘ │    └───────┘
                    │  ┌────────┐ │
                    │  │  REST  │ │<── Frontend polls /analytics/*
                    │  └────────┘ │
                    └─────────────┘
```

Three other services produce events. Each uses a fire-and-forget `SafePublish` wrapper that logs errors but doesn't retry — analytics data is best-effort, and losing an occasional event is acceptable. The analytics-service is the sole consumer in the `analytics-group` consumer group.

## Kafka Fundamentals Used

### Topics and Partitions

A Kafka **topic** is a named log of events. Each topic is divided into **partitions** — ordered, immutable sequences of records. Partitions are the unit of parallelism: each partition is consumed by exactly one consumer in a consumer group.

Our four topics:

| Topic | Events | Producer | Partition Key |
|-------|--------|----------|---------------|
| `ecommerce.orders` | `order.completed` | order-service | `userID` |
| `ecommerce.cart` | `cart.item_added`, `cart.item_removed` | cart-service | `userID` |
| `ecommerce.views` | `product.viewed` | ai-service | `productID` |
| `ecommerce.payments` | `payment.succeeded`, `payment.failed` | (future) | — |

### Why Partition Keys Matter

The partition key determines which partition a message lands on. Kafka hashes the key and assigns: `partition = hash(key) % numPartitions`. Messages with the same key always go to the same partition, which means they are consumed by the same consumer instance and arrive in order.

We key `ecommerce.orders` and `ecommerce.cart` by `userID` so that a given user's cart events and order events land on the same partition. This is critical for the cart abandonment aggregator, which needs to correlate "user added to cart" with "user completed order" — if those events were on different partitions, a multi-consumer deployment couldn't aggregate them locally.

The order-service originally keyed by `orderID`:

```go
// BEFORE: each order gets a random partition
kafka.SafePublish(ctx, o.kafkaPub, "ecommerce.orders", order.ID.String(), ...)

// AFTER: same user's orders and cart events share a partition
kafka.SafePublish(ctx, o.kafkaPub, "ecommerce.orders", order.UserID.String(), ...)
```

This is a deliberate trade-off: we lose per-order ordering guarantees (two orders from different users on the same partition may interleave) but gain user-level locality for cross-topic correlation. In production systems with multiple consumers, this is the standard approach for session-oriented analytics.

### Consumer Groups

A **consumer group** is a set of consumers that cooperate to consume a topic. Kafka assigns each partition to exactly one consumer in the group, ensuring no message is processed twice (within the group). If you add more consumers than partitions, the extras sit idle.

```go
reader := kafka.NewReader(kafka.ReaderConfig{
    Brokers:     brokers,
    GroupID:     "analytics-group",
    GroupTopics: []string{TopicOrders, TopicCart, TopicViews, TopicPayments},
    MinBytes:    1,
    MaxBytes:    10e6, // 10MB
})
```

Key configuration decisions:

- **GroupID `analytics-group`**: This is the identity of our consumer group. Kafka tracks which offsets this group has consumed. If the service restarts, it resumes from the last committed offset.
- **GroupTopics**: We subscribe to all four topics in a single consumer. The `segmentio/kafka-go` Reader handles partition assignment and rebalancing internally.
- **MinBytes: 1**: Fetch as soon as any data is available (low latency). In high-throughput systems you'd increase this to batch fetches.
- **MaxBytes: 10MB**: Cap the fetch size to prevent memory spikes from large batches.

### Offset Management

Every message in a partition has an **offset** — a sequential number. The consumer group tracks which offset it has processed for each partition. This is how Kafka provides at-least-once delivery: if a consumer crashes before committing an offset, the next consumer picks up from the last committed position and reprocesses those messages.

```go
// Commit after processing each message.
if err := c.reader.CommitMessages(ctx, msg); err != nil {
    slog.Error("kafka commit error", "error", err)
}
```

We commit after every message (not in batches). This means:
- **At-least-once**: If the service crashes between processing and committing, the message will be reprocessed on restart.
- **Idempotent writes**: Since Redis operations use `HINCRBY` and `ZADD`, reprocessing the same event produces a slightly inflated count. For analytics, this is acceptable. Production systems that need exactly-once typically use Kafka transactions or deduplication tables.

**Alternative considered:** Committing offsets only after a successful flush to Redis (commit-after-flush). This would provide stronger delivery guarantees but adds complexity — you'd need to track the highest offset per partition and only commit when all windows containing those offsets have been flushed. We opted for simplicity since analytics data tolerates small inaccuracies.

### Event Time vs. Processing Time

A critical distinction in stream processing:

- **Event time**: When the event actually happened (embedded in the message payload)
- **Processing time**: When the consumer processes the message (wall clock at consumption time)

Using processing time is simpler but breaks when events arrive late (network delays, producer retries, consumer lag). A revenue-per-hour window using processing time would attribute a 2 PM order to 3 PM if the consumer is an hour behind.

Our events carry a timestamp in the JSON envelope:

```go
type event struct {
    Type      string          `json:"type"`
    Timestamp time.Time       `json:"timestamp"`
    Data      json.RawMessage `json:"data"`
}
```

The consumer extracts event time and passes it to the windowed aggregators:

```go
eventTime := env.Timestamp
if eventTime.IsZero() {
    eventTime = time.Now() // fallback for legacy events without timestamps
}
```

This fallback is important for backward compatibility — old events produced before the timestamp field was added still get processed, just with slightly less accurate window assignment.

## Windowed Aggregation

### What Is Windowing?

Windowing groups an unbounded stream of events into finite, time-bounded chunks for aggregation. Without windowing, you'd either aggregate forever (growing state) or use arbitrary cutoffs. Windowing gives you well-defined boundaries: "revenue from 2 PM to 3 PM" rather than "revenue since startup."

Three window types are used, each with different trade-offs:

### Tumbling Windows

A **tumbling window** divides time into fixed, non-overlapping segments. Every event belongs to exactly one window. Simple, deterministic, and memory-efficient.

```
Time:     |  1:00  |  2:00  |  3:00  |  4:00  |
Windows:  [───W1───][───W2───][───W3───][───W4───]
Events:    * *  *    *  *      *         * * *
```

Used for: **Revenue per hour** (1-hour tumbling) and **Cart abandonment per 30 minutes** (30-minute tumbling).

Implementation:

```go
type TumblingWindow[T any] struct {
    mu         sync.Mutex
    windowSize time.Duration
    grace      time.Duration
    clock      Clock
    zero       func() T
    buckets    map[string]*bucket[T]
}
```

The key insight is how events are assigned to windows:

```go
start := eventTime.UTC().Truncate(tw.windowSize)
```

`Truncate` rounds down to the nearest window boundary. An event at 2:37 PM with a 1-hour window lands in the 2:00-3:00 window. This is deterministic — the same event always maps to the same window regardless of when it's processed.

### Sliding Windows

A **sliding window** overlaps: a 15-minute window that slides every 1 minute produces a new result every minute, each covering the previous 15 minutes. This provides smoother metrics that respond faster to changes than tumbling windows.

```
Time:     |  1:00  |  1:01  |  1:02  |  ...  |  1:15  |
Window 1: [────────────── 15 min ──────────────]
Window 2:   [────────────── 15 min ──────────────]
Window 3:     [────────────── 15 min ──────────────]
```

Used for: **Trending products** (15-minute window, 1-minute slide).

The implementation uses minute-granularity sub-buckets internally. Each event is placed into a sub-bucket keyed by its minute. When the slide interval fires, all sub-buckets in the window range are merged into a single result:

```go
func (sw *SlidingWindow[T]) Tick() []FlushResult[T] {
    // ...
    for !now.Before(sw.lastSlideEnd) {
        slideEnd := sw.lastSlideEnd
        slideStart := slideEnd.Add(-sw.windowSize)

        merged := sw.zero()
        for keyStr, sb := range sw.subBuckets {
            t, _ := time.Parse(time.RFC3339, keyStr)
            if !t.Before(slideStart) && t.Before(slideEnd) {
                sw.merge(&merged, sb)
            }
        }
        // ... produce FlushResult
        sw.lastSlideEnd = sw.lastSlideEnd.Add(sw.slideInterval)
    }
}
```

**Why sub-buckets?** The alternative is to maintain a separate copy of the data for each overlapping window. With a 15-minute window sliding every minute, that's 15 copies of all the data. Sub-buckets store each event once and merge on demand. The trade-off is merge cost at flush time, but with minute-granularity the number of sub-buckets is bounded (at most `windowSize / 1min + grace / 1min`).

### Late Events and Grace Periods

In any distributed system, events can arrive late. A producer might buffer events during a network partition, or the consumer might fall behind. The question is: how long do you wait before closing a window?

A **grace period** extends the window's lifetime beyond its nominal end time. Events arriving within the grace period update the window; events arriving after are dropped.

```go
// Drop if the event's window end + grace has already passed.
if tw.clock.Now().After(end.Add(tw.grace)) {
    return key, true // dropped
}
```

Our grace periods:
- Revenue: 5 minutes (orders are important, tolerate some lateness)
- Trending: 5 minutes (views are high-volume, same tolerance)
- Abandonment: 5 minutes (correlating cart + orders needs some slack)

Dropped events are counted in a Prometheus metric (`analytics_late_events_dropped_total`) so you can monitor whether the grace period is too short.

**Trade-off**: Longer grace periods mean more accurate results but higher memory usage (windows stay open longer) and higher latency (results are delayed by the grace period). In a production system processing millions of events per second, you'd tune this carefully. Five minutes is generous for our event volume.

### Generic Window Framework

Both window types use Go generics to be data-type agnostic:

```go
type TumblingWindow[T any] struct { ... }
type SlidingWindow[T any]  struct { ... }
```

The aggregator defines its own data type and provides callbacks:

```go
// Revenue uses TumblingWindow[revenueData]
type revenueData struct {
    TotalCents int64
    OrderCount int64
}

window := NewTumblingWindow(1*time.Hour, 5*time.Minute, clock, func() revenueData {
    return revenueData{} // zero-value factory
})

// Add an event
window.Add(eventTime, func(d *revenueData) {
    d.TotalCents += totalCents  // callback mutates the bucket
    d.OrderCount++
})
```

This design means the window framework knows nothing about revenue, trending scores, or abandonment — it just manages time bucketing. The `func(*T)` callback pattern lets the aggregator mutate the data in place without the window needing to understand the data type. This is a Go-specific pattern that avoids the interface boxing overhead of a more abstract approach.

## Aggregator Design

### Revenue Aggregator

The simplest aggregator. Uses a 1-hour tumbling window to track order revenue and count.

```go
type RevenueAggregator struct {
    window *window.TumblingWindow[revenueData]
    store  store.Store
}

func (a *RevenueAggregator) HandleOrderCompleted(eventTime time.Time, totalCents int64) bool {
    _, dropped := a.window.Add(eventTime, func(d *revenueData) {
        d.TotalCents += totalCents
        d.OrderCount++
    })
    return !dropped
}
```

On flush, it writes to Redis and evicts the window from memory:

```go
func (a *RevenueAggregator) Flush(ctx context.Context) error {
    results := a.window.Tick() // get expired windows
    for _, r := range results {
        if err := a.store.FlushRevenue(ctx, r.Key, r.Data.TotalCents, r.Data.OrderCount); err != nil {
            // continue evicting successfully flushed windows
            continue
        }
        a.window.Evict(r.Key) // free memory
    }
    return firstErr
}
```

**Error handling pattern**: If Redis is down (circuit breaker open), the flush fails but the window stays in memory. On the next tick, it's retried. This means windows accumulate during Redis outages. A production system would want a memory limit here.

### Trending Aggregator

Uses a 15-minute sliding window with 1-minute slide interval. Tracks weighted product scores: views count 1 point, cart-adds count 3 points (stronger purchase intent signal).

```go
const (
    trendingViewWeight    = 1.0
    trendingCartAddWeight = 3.0
)

type trendingData struct {
    Scores map[string]float64 // productID -> weighted score
    Names  map[string]string  // productID -> productName
}
```

The merge function for the sliding window combines scores by summing:

```go
func(dst, src *trendingData) {
    for pid, score := range src.Scores {
        dst.Scores[pid] += score
    }
    for pid, name := range src.Names {
        if name != "" {
            dst.Names[pid] = name
        }
    }
}
```

**Why weighted scores?** A product with 10 views and 2 cart-adds (score: 16) is trending more meaningfully than one with 15 views and 0 cart-adds (score: 15). Cart-adds are a stronger signal of purchase intent. The 3:1 weight ratio is a reasonable starting point — in production you'd tune this based on conversion rate analysis.

**Why track names?** The Kafka view events include `productName`, but the trending store only has `productID` (Redis sorted sets store members as strings). We maintain a separate names map so the REST API can return human-readable product names without a secondary lookup to the product-service.

### Abandonment Aggregator

Uses a 30-minute tumbling window. Tracks two sets per window: users who added items to cart ("started") and users who completed orders ("converted"). The difference is the abandonment count.

```go
type abandonmentData struct {
    StartedUsers   map[string]bool
    ConvertedUsers map[string]bool
}
```

On flush, it computes the metrics from set sizes:

```go
started := int64(len(r.Data.StartedUsers))
converted := int64(len(r.Data.ConvertedUsers))
// abandoned = started - converted, rate = abandoned / started
```

**Design decision: in-memory sets vs. Redis sets.** We track users in Go maps during the window lifetime, then flush the computed counts to Redis. An alternative is to use Redis sets (`SADD`, `SDIFF`) for real-time dedup. We chose in-memory because:
1. The window is bounded (30 minutes), so the set size is bounded
2. Redis round-trips per event would add latency
3. The dedup is inherent to Go's `map[string]bool` — adding the same user twice is a no-op

The trade-off is that if the service restarts mid-window, the in-progress sets are lost. For a 30-minute window, this is acceptable.

## Consumer Architecture

### Message Processing Loop

The consumer runs a simple fetch-process-commit loop:

```go
func (c *Consumer) Run(ctx context.Context) error {
    go c.flushLoop(ctx)  // periodic flush in background

    for {
        msg, err := c.reader.FetchMessage(ctx)
        if err != nil {
            if ctx.Err() != nil {
                c.finalFlush()  // flush before exit
                return nil
            }
            metrics.ConsumerErrors.Inc()
            continue
        }
        c.connected.Store(true)
        c.processing.Store(true)
        c.route(msg)
        c.processing.Store(false)
        c.reader.CommitMessages(ctx, msg)
    }
}
```

Two important patterns here:

**1. Flush loop as a separate goroutine.** The flush ticker runs independently of message processing. This means windows get flushed on schedule even if no new messages arrive (important when traffic is sparse). The flush loop and message loop don't need synchronization because the window types use internal `sync.Mutex` — concurrent Add and Tick calls are safe.

**2. Final flush on shutdown.** When the context is cancelled (SIGTERM), the consumer performs one last flush with a 5-second timeout. This ensures in-progress window data reaches Redis before the process dies. Without this, a graceful shutdown during a quiet period could lose up to `flushInterval` seconds of buffered data.

### Event Routing

Events are routed by topic, then by event type:

```go
switch msg.Topic {
case TopicOrders:
    c.handleOrder(env, eventTime)    // → revenue + abandonment
case TopicCart:
    c.handleCart(env, eventTime)     // → trending + abandonment
case TopicViews:
    c.handleView(env, eventTime)    // → trending
case TopicPayments:
    c.handlePayment(env)            // log only (future use)
}
```

Note that `order.completed` events feed **two** aggregators (revenue and abandonment), and `cart.item_added` events also feed two (trending and abandonment). This fan-out is intentional — the same event carries different signals for different metrics.

### Trace Context Propagation

Every Kafka message can carry W3C `traceparent` headers. The consumer extracts them using the shared `tracing` package:

```go
msgCtx := tracing.ExtractKafka(ctx, msg.Headers)
```

This means a single user checkout creates a trace that spans: frontend → order-service → RabbitMQ saga → Kafka publish → analytics consumer. The full lifecycle is visible in Jaeger.

## Storage Layer

### Why Redis?

The window results need to be queryable by the REST API and survive consumer restarts. Options considered:

| Option | Pros | Cons |
|--------|------|------|
| **Redis** | Fast reads, TTL auto-cleanup, sorted sets for rankings, already in the stack | Not durable (data loss on Redis restart) |
| PostgreSQL | Durable, SQL queries, joins | Write latency, overkill for ephemeral metrics |
| Kafka output topics | Pure streaming, Kafka-native | Requires consumer on the read side too |
| In-memory only | Simplest, fastest | Lost on restart, not queryable from other instances |

Redis was chosen because:
1. Read latency matters — the frontend polls every 30 seconds
2. TTL handles automatic cleanup — no garbage collection code needed
3. Sorted sets are purpose-built for "top N" queries (trending products)
4. The cluster already runs Redis for other services (product cache, rate limiting)
5. Analytics data is ephemeral — losing it on Redis restart is acceptable

### Key Schema

```
analytics:revenue:{ISO8601_hour}              → Hash     TTL 48h
analytics:trending:{ISO8601_hour}             → Sorted Set  TTL 2h
analytics:trending:names:{ISO8601_hour}       → Hash     TTL 2h
analytics:abandonment:{ISO8601_hour}          → Hash     TTL 24h
analytics:abandonment:users:{window}:{bucket} → Set      TTL 24h
```

Different TTLs reflect how far back each metric is useful. Revenue: 48 hours for daily comparison. Trending: 2 hours because only recent trends matter. Abandonment: 24 hours for daily pattern analysis.

### Idempotent Writes

Since we use at-least-once delivery (commit after processing, not after flush), duplicate events can inflate counts. The Redis operations are chosen to minimize this:

- **Revenue**: `HINCRBY` is additive. A duplicate event double-counts. For analytics dashboards this is acceptable. For billing you'd need exactly-once processing.
- **Trending**: `ZADD` with score addition. Duplicates inflate scores slightly. The relative ranking (which product is trending most) is still correct.
- **Abandonment**: User sets use `map[string]bool` in Go — adding the same user ID twice is idempotent. The counts are correct even with duplicates.

### Circuit Breaker

All Redis operations are wrapped with a circuit breaker (`sony/gobreaker`):

```go
_, err := s.breaker.Execute(func() (any, error) {
    // Redis operations here
})
```

When Redis is unreachable, the breaker opens after a threshold of failures, and subsequent calls fail immediately without hitting the network. This prevents cascading failures — the consumer keeps processing events (buffering in windows) while Redis recovers.

## Observability

### Prometheus Metrics

Eight metrics cover the full pipeline:

| Metric | Type | Labels | What it tells you |
|--------|------|--------|-------------------|
| `analytics_events_consumed_total` | Counter | `topic` | Throughput per topic |
| `analytics_aggregation_latency_seconds` | Histogram | `aggregator` | Processing time per event |
| `kafka_consumer_lag` | Gauge | — | How far behind the consumer is |
| `kafka_consumer_errors_total` | Counter | — | Fetch/commit failures |
| `analytics_window_flushes_total` | Counter | `aggregator` | Flush operations per aggregator |
| `analytics_window_flush_latency_seconds` | Histogram | `aggregator` | Redis write latency |
| `analytics_late_events_dropped_total` | Counter | `aggregator` | Events past grace period |
| `analytics_active_windows` | Gauge | `aggregator` | Memory usage proxy |

The most operationally important ones:

- **Consumer lag**: If this grows, the consumer is falling behind. In production you'd alert on lag > 1000 and scale consumers.
- **Late events dropped**: If this spikes, your grace period is too short or producers are severely delayed.
- **Active windows**: If this grows unboundedly, windows aren't being evicted (likely a Redis outage preventing flushes).

### Structured Logging

The service uses `slog` with JSON output and OTel trace context injection. Every log line includes a trace ID when available:

```json
{"time":"2026-04-22T14:30:00Z","level":"INFO","msg":"kafka consumer starting",
 "topics":["ecommerce.orders","ecommerce.cart","ecommerce.views","ecommerce.payments"],
 "flushInterval":"30s","traceID":"abc123..."}
```

## Testing Strategy

### Unit Tests with Mock Clock

The window framework is tested by controlling time:

```go
clock := window.NewMockClock(time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC))
tw := window.NewTumblingWindow(time.Hour, 5*time.Minute, clock, zeroFunc)

// Add event at 14:30
tw.Add(clock.Now().Add(30*time.Minute), mutator)

// Advance past window end + grace
clock.Advance(time.Hour + 6*time.Minute)

// Tick should return the closed window
results := tw.Tick()
```

This pattern is essential for testing time-dependent logic without `time.Sleep`. The `Clock` interface allows tests to be deterministic and fast.

### Mock Store for Aggregator Tests

Aggregators are tested with `MockStore` — an in-memory implementation of the `Store` interface:

```go
mockStore := store.NewMockStore()
agg := aggregator.NewRevenueAggregator(time.Hour, 5*time.Minute, clock, mockStore)
agg.HandleOrderCompleted(eventTime, 5000)
// advance clock, flush
agg.Flush(ctx)
// verify store received the data
assert(mockStore.TotalRevenueCents() == 5000)
```

This keeps unit tests fast and focused on aggregation logic, not Redis connectivity.

### Integration Tests with Testcontainers

End-to-end tests spin up real Kafka via testcontainers:

```go
func TestRevenue_EndToEnd(t *testing.T) {
    infra := getInfra(t) // starts Kafka container
    mockStore, cleanup := setupConsumer(t, infra.KafkaBrokers)
    defer cleanup()

    publishEvent(t, infra.KafkaBrokers, "ecommerce.orders", "order.completed",
        map[string]any{"orderID": "ord-1", "userID": "user-1", "totalCents": 5000})

    // Poll until data appears (windows close after 2s in tests)
    pollUntil(t, 15*time.Second, func() bool {
        return mockStore.TotalRevenueCents() > 0
    })
}
```

Integration tests use 2-second windows with 0 grace to verify the full pipeline quickly.

## Package Choices

### segmentio/kafka-go

**What it does:** Pure Go Kafka client. Provides `Reader` (consumer) and `Writer` (producer) with consumer group support.

**Why chosen over alternatives:**
- **confluent-kafka-go** (librdkafka wrapper): Requires CGO and a C compiler. More feature-complete (transactions, admin API) but harder to build and deploy. segmentio/kafka-go is pure Go — it cross-compiles trivially and has no C dependencies.
- **Shopify/sarama**: Another pure Go option. Older API design with more boilerplate. segmentio/kafka-go has a simpler Reader/Writer API.

**Key APIs used:**
- `kafka.NewReader(config)` — creates a consumer with consumer group support
- `reader.FetchMessage(ctx)` — blocking fetch, returns one message
- `reader.CommitMessages(ctx, msg)` — commits offset for the message
- `reader.Stats()` — returns consumer lag and other metrics

### redis/go-redis/v9

**What it does:** Redis client with pipeline support, connection pooling, and cluster mode.

**Why chosen:** It's the de facto Go Redis client. Used by all other services in the project. Supports pipelines (batch multiple commands in one round-trip) and all data structures we need (hashes, sorted sets, sets).

**Key operations:**
- `HINCRBY` — atomic increment of hash field (revenue counters)
- `ZADD` — add members to sorted set with scores (trending products)
- `ZREVRANGEBYSCORE` — get top N members by score (trending query)
- `SADD` / `SCARD` — set add / count (abandonment user dedup)
- `EXPIRE` — TTL for automatic cleanup

### sony/gobreaker/v2

**What it does:** Circuit breaker pattern implementation. Prevents cascading failures when a dependency is unhealthy.

**Why used:** Every Redis operation is wrapped in a circuit breaker. When Redis fails repeatedly, the breaker opens and fast-fails subsequent calls. This prevents the consumer from blocking on Redis timeouts and accumulating backpressure.

## Deployment

### Kubernetes Configuration

The analytics-service runs as a single-replica deployment in the `go-ecommerce` namespace. Key ConfigMap entries:

```yaml
KAFKA_BROKERS: "kafka.go-ecommerce.svc.cluster.local:9092"
REDIS_URL: "redis://redis.java-tasks.svc.cluster.local:6379/0"
WINDOW_FLUSH_INTERVAL: "30s"
REVENUE_WINDOW_SIZE: "1h"
TRENDING_WINDOW_SIZE: "15m"
TRENDING_SLIDE_INTERVAL: "1m"
ABANDONMENT_WINDOW_SIZE: "30m"
LATE_EVENT_GRACE: "5m"
```

Window sizes and the flush interval are configurable via environment variables. This means you can tune them per environment without code changes — QA might use shorter windows for faster feedback.

### Scaling Considerations

Currently runs as a single consumer. To scale horizontally:

1. Increase Kafka topic partitions (currently 1 per topic)
2. Deploy multiple analytics-service replicas
3. Kafka automatically assigns partitions across consumers in the group
4. Each consumer processes its assigned partitions independently
5. Window state is per-consumer (not shared), so Redis keys from different consumers don't conflict — they're keyed by event time, and events on different partitions have different timestamps

The main constraint is the abandonment aggregator, which correlates cart + order events for the same user. With partition-aware keying (both keyed by userID), the same user's events always go to the same consumer, so local aggregation works even with multiple consumers.

## Consequences

**Positive:**
- Real-time dashboards with windowed precision (hourly revenue, 15-minute trending, 30-minute abandonment)
- Pure Go implementation makes all Kafka patterns visible and debuggable
- Configurable window sizes allow tuning per environment
- Circuit breaker + graceful shutdown prevent data loss during infrastructure issues
- Generic window framework is reusable for future aggregation types

**Trade-offs:**
- At-least-once delivery means duplicate events can slightly inflate counters
- In-memory window state is lost on hard crash (SIGKILL) — graceful shutdown handles SIGTERM
- Single consumer group means all processing is sequential (sufficient for current volume)
- Redis is ephemeral — analytics data doesn't survive Redis restart (acceptable for real-time dashboards, not for compliance reporting)

**Future improvements:**
- Event sourcing for orders (Kafka compacted topics for order state snapshots)
- Inventory sync pipeline with multi-consumer fan-out
- CDC pipeline for database change streaming
- Exactly-once processing with idempotency keys per event
