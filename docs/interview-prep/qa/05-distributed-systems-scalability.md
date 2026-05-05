# Distributed Systems And Scalability Rehearsal

Use this section for system design, async workflows, consistency, scaling,
observability, and production reliability questions.

## Repo Anchors

- `go/order-service/internal/saga`: checkout saga orchestration, RabbitMQ
  commands/events, compensation, DLQ, replay, recovery, and metrics.
- `go/cart-service/internal/worker/saga_handler.go`: cart reservation/release
  command consumer and reply publisher.
- `go/payment-service/internal/service/webhook.go`: payment webhook processing
  that writes saga events through an outbox.
- `go/payment-service/internal/outbox/poller.go`: outbox poller that publishes
  unpublished messages to RabbitMQ.
- `go/order-service/internal/events`: domain events published to Kafka for
  analytics.
- `go/analytics-service/internal/consumer`: Kafka consumer for order, cart, and
  view events.
- `go/analytics-service/internal/window` and `internal/aggregator`: tumbling and
  sliding window aggregations for revenue, abandonment, and trending.
- `go/pkg/resilience`: retry and circuit breaker helpers.
- `go/pkg/tracing`: OpenTelemetry trace propagation for Kafka, AMQP, Redis, and
  structured logs.
- `go/pkg/shutdown`: graceful shutdown and drain helpers.
- `go/k8s`: deployments, services, HPAs, PDBs, migration jobs, Kafka, ingress,
  configmaps, and network policy.

## High-Frequency Questions

### 1. How do you handle transactions across multiple microservices?

Fast answer:

> I avoid distributed transactions unless the consistency requirement truly
> demands them. Most microservice workflows use a saga: each service owns its
> local transaction, publishes an event or command, and defines compensation for
> failures. The hard parts are idempotency, retry behavior, observability, and
> recovery from partial progress. In this repo, checkout is saga-oriented across
> order, cart, and payment concerns, with RabbitMQ commands/events and explicit
> saga steps stored on the order.

Follow-ups:

#### Follow-up: Why not two-phase commit?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is a compensating transaction?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Where do you store saga state?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you recover incomplete sagas?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 2. What is eventual consistency, and when is it acceptable?

Fast answer:

> Eventual consistency means different services or read models may temporarily
> disagree, but converge once asynchronous work completes. It is acceptable when
> the business can tolerate a short delay, such as analytics, projections, or
> order status moving through pending states. It is not acceptable for invariants
> that must be checked synchronously, like whether a user is authorized or
> whether a local database transaction preserves a constraint.

Follow-ups:

#### Follow-up: How do you explain pending states to users?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you detect stuck workflows?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if events arrive out of order?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if convergence never happens?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 3. How do you make message consumers reliable?

Fast answer:

> A reliable consumer needs idempotent processing, bounded concurrency,
> context-aware shutdown, clear ack/nack rules, dead-letter handling, and metrics
> for lag, errors, and processing latency. I assume duplicate delivery can happen
> and design handlers so reprocessing is safe. This repo has RabbitMQ saga
> consumers with DLQ/replay and Kafka analytics consumers with lag/error metrics.

Follow-ups:

#### Follow-up: When do you ack a message?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle poison messages?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid duplicate side effects?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you scale consumers horizontally?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 4. What is the outbox pattern?

Fast answer:

> The outbox pattern stores a domain change and the message that must be emitted
> in the same database transaction. A separate poller publishes unpublished
> messages and marks them published. This prevents the classic failure where the
> database commit succeeds but the message publish fails, or vice versa. In this
> repo, payment webhook processing writes saga events to an outbox, and a poller
> publishes those messages to RabbitMQ.

Follow-ups:

#### Follow-up: Does outbox guarantee exactly-once delivery?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you prevent duplicate publishes?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you order outbox messages?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What metrics should the poller expose?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 5. How do retries and idempotency work together?

Fast answer:

> Retries make transient failures survivable, but they also create duplicates and
> amplify load. Idempotency makes repeated attempts safe by giving the operation
> a stable identity and recording whether it is pending or completed. For message
> consumers, idempotency may be a processed-event table or a unique constraint.
> For APIs, it may be an idempotency key. For external providers, it may be a
> provider idempotency key.

Follow-ups:

#### Follow-up: What should be retried?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What should never be retried?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you choose retry limits?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid retry storms?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 6. How do circuit breakers help distributed systems?

Fast answer:

> Circuit breakers stop repeated calls to a failing dependency so the caller can
> fail fast and preserve its own resources. Retries help with occasional
> transient failures; circuit breakers help when the dependency is unhealthy for
> a period of time. This repo has a shared resilience package with retry and
> circuit breaker wrappers, and breaker state is exposed as Prometheus metrics.

Follow-ups:

#### Follow-up: What causes the breaker to open?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is half-open state?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Where should you put a breaker?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What do users see while the breaker is open?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 7. How do you design for backpressure?

Fast answer:

> Backpressure means the system has a way to slow or reject upstream work before
> it exhausts resources. In Go, that can be bounded channels, worker pools,
> request rate limits, queue limits, context deadlines, and load shedding. In a
> message system, lag is a backpressure signal. The key is to avoid unbounded
> memory growth and invisible queues.

Follow-ups:

#### Follow-up: What happens if producers are faster than consumers?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you size a worker pool?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What do you do when Kafka lag grows?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Do you drop, delay, or reject work?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 8. How do you scale a Go service horizontally?

Fast answer:

> The service needs to be stateless at the request layer, use external shared
> state for databases/cache/queues, expose health/readiness checks, handle
> graceful shutdown, and avoid local in-memory coordination that breaks across
> replicas. Then Kubernetes can scale replicas behind a service, and autoscaling
> can use CPU, memory, or custom metrics. This repo includes Kubernetes
> deployments, services, HPAs, PDBs, probes, and migration jobs for Go services.

Follow-ups:

#### Follow-up: What state cannot live in memory?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle deployments without dropping traffic?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How does readiness differ from liveness?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What metrics would drive autoscaling?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 9. How do you debug high latency in a distributed system?

Fast answer:

> I start by finding where the time is spent: gateway, service handler, database,
> cache, queue, or external dependency. Traces identify slow spans; metrics show
> whether it is systemic; logs with trace IDs explain representative failures.
> Then I look for saturation: connection pools, goroutine count, lock contention,
> retry storms, queue lag, circuit breaker state, and downstream error rate.

Follow-ups:

#### Follow-up: What if traces are missing?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you debug queue latency?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do retries hide the root cause?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is p99 telling you that average latency hides?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 10. How do you preserve trace context across async boundaries?

Fast answer:

> You have to inject trace context into message headers when publishing and
> extract it when consuming. Otherwise traces break at the queue boundary and you
> lose the end-to-end path. This repo has tracing helpers for AMQP and Kafka that
> inject and extract W3C trace context, plus a log handler that adds trace IDs to
> structured logs.

Follow-ups:

#### Follow-up: Why is async tracing harder than HTTP tracing?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What headers do you propagate?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle missing trace context?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What should logs include?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 11. How do you handle late or out-of-order events?

Fast answer:

> I design consumers to tolerate event-time disorder. For analytics, that means
> using event timestamps, windows, grace periods, and deciding what to do with
> events that arrive too late. For business workflows, I use explicit state
> transitions and ignore or compensate invalid transitions. The analytics service
> uses tumbling and sliding windows with grace periods, and tests cover dropped
> late events and window eviction.

Follow-ups:

#### Follow-up: Event time versus processing time?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is a watermark?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid double counting?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: When do you drop late events?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 12. How do you design graceful shutdown?

Fast answer:

> A service should stop accepting new work, let in-flight work finish within a
> deadline, stop consumers, close network/database clients, and flush telemetry.
> Goroutines need cancellation signals so they do not leak. This repo has shared
> shutdown helpers for HTTP and gRPC draining, waiting for in-flight work, and
> running shutdown hooks in priority order.

Follow-ups:

#### Follow-up: What happens to in-flight requests?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you stop message consumers?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if shutdown times out?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Why does Kubernetes readiness matter before termination?

Fast answer:

> A strong answer should extend the parent answer with the concrete
> tradeoff, failure mode, or implementation detail, and explain how it
> applies in this repo rather than answering generically.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

## Scenario Drills

### Scenario 1: Checkout Saga With Partial Failure

Prompt:

> A user checks out. Cart reservation succeeds, but payment fails. What should
> happen?

Strong answer checklist:

- Order starts in pending/processing state.
- Cart reservation is a local operation owned by cart service.
- Payment failure emits or triggers a failure event.
- Saga updates order to failed.
- Compensation releases reserved cart items.
- Duplicate failure events are safe.
- User sees a clear failed or retryable order state.
- Metrics/logs/traces show which step failed.

Repo tie-in:

- Order saga has explicit steps, cart command handling, payment webhook events,
  compensation paths, and saga metrics.

### Scenario 2: Kafka Analytics Lag Is Growing

Prompt:

> Analytics dashboards are stale because Kafka consumer lag keeps increasing.
> How do you investigate and scale it?

Strong answer checklist:

- Confirm lag by topic/partition/consumer group.
- Check consumer errors, processing latency, Redis latency, and CPU/memory.
- Determine whether events are skewed by key or partition.
- Increase consumers only up to partition count.
- Optimize aggregation or storage writes.
- Add backpressure/degradation if downstream store is slow.
- Alert on lag age, not only message count.

Repo tie-in:

- Analytics service consumes ecommerce topics, tracks consumer metrics, writes
  windowed aggregates to Redis, and exposes analytics endpoints.

### Scenario 3: Duplicate Webhook And Duplicate Queue Message

Prompt:

> A payment webhook is retried by the provider, and the outbox message is also
> published twice. How do you prevent duplicate downstream effects?

Strong answer checklist:

- Webhook processing records provider event ID.
- Outbox publish can be at-least-once, not exactly-once.
- Downstream saga consumer must be idempotent by order/payment event identity.
- State transitions should reject duplicates or no-op safely.
- Observability should count duplicate/dropped events.

Repo tie-in:

- Payment service has processed events and outbox; saga state is stored on the
  order so repeated events can be handled by state transition logic.

### Scenario 4: Rolling Deploy Without Dropping Work

Prompt:

> You are deploying a Go service that handles HTTP requests and consumes queue
> messages. How do you avoid dropping work?

Strong answer checklist:

- Readiness turns false before shutdown or traffic drain.
- Stop accepting new HTTP traffic.
- Cancel consumer context or stop pulling new messages.
- Finish or nack in-flight messages according to broker semantics.
- Drain HTTP/gRPC servers within deadline.
- Close DB/Redis/broker clients.
- Flush telemetry.
- Use PDBs and rolling update strategy in Kubernetes.

Repo tie-in:

- Go services configure HTTP timeouts, health endpoints, shutdown hooks, PDBs,
  and Kubernetes deployments.
