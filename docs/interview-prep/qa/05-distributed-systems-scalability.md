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

> Two-phase commit gives stronger atomicity, but it couples services to a
> coordinator and can block availability when one participant is slow or down.
> For checkout, I would rather model the business process explicitly with saga
> steps, retries, and compensation. In this repo, order, cart, and payment each
> own local state, while RabbitMQ messages and stored saga state make partial
> progress recoverable.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is a compensating transaction?

Fast answer:

> A compensating transaction is the business action that reverses or neutralizes
> a completed step when a later step fails. It is not a database rollback across
> services; it is a new domain action, like releasing a reserved cart after
> payment fails. The important part is making compensation idempotent, because
> the same failure event may be retried or replayed.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Where do you store saga state?

Fast answer:

> Saga state should be durable, queryable, and owned by the orchestrating
> service, not kept only in memory. I would store the current step, status,
> correlation IDs, retry counts, timestamps, and enough detail to recover or
> compensate. In this repo, the order service is the natural owner because the
> checkout outcome is part of the order lifecycle.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you recover incomplete sagas?

Fast answer:

> I recover incomplete sagas with a durable state scan or replay process that
> finds sagas stuck past an expected deadline and resumes, retries, or
> compensates based on the last completed step. Messages should be idempotent so
> replay is safe. In this repo, recovery should combine stored order saga state,
> RabbitMQ DLQ/replay, metrics for stuck steps, and trace/log correlation.

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

> I make pending states explicit instead of pretending the whole workflow is
> synchronous. For checkout, the user can see that the order is processing,
> failed, paid, or requires retry, and the API should return a stable status
> they can poll or receive through updates. That keeps eventual consistency
> honest while the saga coordinates cart and payment work behind the scenes.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you detect stuck workflows?

Fast answer:

> I track how long each workflow spends in each state and alert when a saga or
> message stays pending beyond its expected SLA. Metrics should include active
> sagas by state, step duration, retry count, DLQ depth, and recovery attempts.
> In this repo, the order saga metrics and RabbitMQ DLQ/replay path are the
> right anchors for finding checkout workflows that stopped progressing.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if events arrive out of order?

Fast answer:

> Consumers should not assume delivery order unless the broker and partitioning
> strategy guarantee it for that key. I use event IDs, timestamps, versions, and
> explicit state-transition rules so an old or invalid event becomes a no-op or
> goes to investigation. For checkout, a payment success after an order was
> already canceled should not blindly move the order to paid.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if convergence never happens?

Fast answer:

> Eventual consistency still needs an operational deadline. If a workflow does
> not converge, I want automated recovery for known cases, compensation for
> failed business paths, and alerts for manual repair when the system cannot
> decide safely. In this repo, that means stuck saga detection, DLQ replay,
> clear failed order states, and runbook-style evidence in logs, metrics, and
> traces.

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

> I ack only after the side effect that matters is durably complete, or after I
> have safely recorded enough state to finish it later. Acking before the
> database write risks losing work; acking too late can create duplicate
> deliveries. In the saga consumers, that means handlers must be idempotent and
> ack/nack behavior should match whether the failure is transient or permanent.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle poison messages?

Fast answer:

> A poison message is one that repeatedly fails because the payload or state is
> bad, not because the dependency is briefly down. I retry it a bounded number
> of times, then send it to a DLQ with the error class, message ID, correlation
> ID, and enough context to replay or repair. This repo's RabbitMQ saga flow
> should treat DLQ depth and repeated failures as operational signals.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid duplicate side effects?

Fast answer:

> I assume at-least-once delivery and make handlers idempotent. That can mean a
> processed-message table, unique constraints, provider event IDs, idempotency
> keys, or state transitions that no-op when the target state is already
> reached. In this repo, payment webhook IDs, outbox message identity, and order
> saga state should prevent duplicate charges, duplicate releases, or duplicate
> status transitions.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you scale consumers horizontally?

Fast answer:

> I scale consumers by adding replicas or workers while respecting broker
> semantics, partition count, prefetch, ordering requirements, and downstream
> capacity. More consumers do not help if the database, Redis, or a hot
> partition is the bottleneck. For this repo, Kafka analytics can scale up to
> partition limits, while RabbitMQ saga consumers need prefetch and idempotency
> tuned so parallelism does not break workflow correctness.

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

> No. The outbox guarantees the state change and intent to publish are committed
> together, but publishing is still usually at-least-once. The poller can crash
> after publishing but before marking the row published, so duplicates are
> possible. That is why downstream saga consumers still need idempotency based
> on event identity or state transition rules.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you prevent duplicate publishes?

Fast answer:

> I reduce duplicate publishes with row locking, a published marker, publish
> attempts, and stable message IDs, but I still design consumers as if duplicate
> publishes can happen. A poller should claim a batch, publish with a consistent
> message identity, then mark rows published transactionally where possible. In
> this repo, the payment outbox poller should expose duplicate and retry metrics
> rather than pretending publish is exactly-once.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you order outbox messages?

Fast answer:

> I order outbox messages by aggregate when order matters, not globally across
> the entire system. Usually that means ordering by aggregate ID plus sequence
> or creation time and partitioning messages so related events are consumed in
> order. For checkout, order-specific events need consistent ordering; analytics
> events can often tolerate out-of-order arrival with event-time handling.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What metrics should the poller expose?

Fast answer:

> I want outbox depth, oldest unpublished age, batch size, publish latency,
> publish failures, retry attempts, rows marked published, and DLQ or permanent
> failure counts. Oldest unpublished age is especially important because a small
> queue with one stuck message can still mean stale business state. In this
> repo, those metrics would show whether payment webhook events are flowing into
> the order saga promptly.

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

> Retry transient failures: timeouts, temporary network failures, 429s with
> budget, 5xx dependency errors, serialization failures, and broker publish
> failures where the operation is idempotent. The retry policy should preserve
> the original context deadline and use backoff with jitter. In this repo,
> `go/pkg/resilience` is the shared place to keep that classification
> consistent.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What should never be retried?

Fast answer:

> Do not retry permanent validation errors, authorization failures, malformed
> messages, business rule conflicts, or non-idempotent side effects where a
> second attempt could double charge or corrupt state. Those should fail fast or
> go to a DLQ with enough detail to repair. For checkout, a malformed cart
> command should not loop forever and block other saga messages.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you choose retry limits?

Fast answer:

> I choose retry limits from the caller's latency budget, the dependency's
> recovery behavior, and the business cost of delay. User-facing requests get a
> small number of fast retries or none; background workflows can retry longer
> with backoff and DLQ. In this repo, API calls and saga consumers should have
> different retry budgets, but both need metrics for attempt count and final
> outcome.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid retry storms?

Fast answer:

> I use bounded retries, exponential backoff with jitter, circuit breakers,
> concurrency limits, and respect for context deadlines and `Retry-After`
> headers. The failure mode is every replica retrying the same failing
> dependency at once and making the outage worse. In this repo, retry and
> breaker metrics should show when resilience behavior is protecting the system
> versus amplifying load.

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

> A breaker opens when recent calls cross a configured failure threshold, such
> as too many timeouts, connection failures, or 5xx responses. It should be
> based on a rolling window and enough volume to avoid opening on one random
> failure. In this repo, dependency clients should classify failures
> consistently so breaker state reflects real downstream health.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is half-open state?

Fast answer:

> Half-open is the probe state after the breaker has been open for a cooldown
> period. The caller allows a small number of test requests through; if they
> succeed, the breaker closes, and if they fail, it opens again. That prevents a
> recovering dependency from being overwhelmed by all traffic at once.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Where should you put a breaker?

Fast answer:

> Put the breaker at the outbound dependency boundary: HTTP clients, broker
> publishers, external providers, Redis, or service-to-service clients. That is
> where the caller can fail fast and preserve its own worker pool, DB pool, and
> goroutines. I would not put a breaker around pure local business logic; the
> repo's shared resilience package belongs around real dependency calls.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What do users see while the breaker is open?

Fast answer:

> Users should see a fast, honest degraded response instead of waiting through
> doomed timeouts. For a read path, that might be cached or partial data; for a
> checkout path, it may be a retryable failure or a pending state if work was
> safely queued. The API should return stable error codes, and metrics should
> show the breaker is open.

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

> Queues grow, latency increases, memory or disk pressure rises, and eventually
> messages expire or the broker becomes unstable. Backpressure has to show up
> before that point through rate limits, bounded buffers, queue lag alerts, or
> producer throttling. In this repo, growing Kafka lag means analytics is stale;
> growing RabbitMQ saga backlog means checkout state may be delayed.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you size a worker pool?

Fast answer:

> I size it from the bottleneck: CPU for compute-heavy work, connection pool
> size for database-heavy work, provider rate limits for external calls, and
> broker partitioning for Kafka. Then I test under load and watch queue depth,
> processing latency, error rate, and downstream saturation. The worker count
> should be a resource limit, not just the biggest number that improves local
> throughput.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What do you do when Kafka lag grows?

Fast answer:

> I break lag down by topic, partition, and consumer group, then check consumer
> errors, processing latency, downstream Redis/database latency, CPU, memory,
> and key skew. Scaling consumers only helps up to the partition count and only
> if the downstream store can handle it. For this repo's analytics service, I
> would also check window aggregation cost and stale dashboard age.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Do you drop, delay, or reject work?

Fast answer:

> It depends on the business value of the work. Payments and orders should be
> delayed, retried, or moved to a recoverable state because correctness matters.
> Low-value analytics or derived events may be sampled, dropped after a grace
> period, or degraded. I want that decision explicit in code and metrics so
> backpressure is a controlled tradeoff rather than accidental data loss.

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

> Anything required for correctness across requests or replicas cannot live
> only in memory: sessions if they need cross-replica access, idempotency keys,
> saga state, processed event IDs, rate-limit counters for distributed limits,
> and durable workflow status. In this repo, order saga state and payment event
> dedupe need durable storage; local memory is fine only for caches that can be
> rebuilt.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle deployments without dropping traffic?

Fast answer:

> I use readiness probes, graceful shutdown, rolling updates, and enough
> replicas or PDBs to keep capacity during the rollout. A pod should become not
> ready before termination, stop accepting new work, drain in-flight requests,
> and stop pulling new messages before closing dependencies. This repo's
> shutdown helpers and Kubernetes probes are the key pieces for that behavior.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How does readiness differ from liveness?

Fast answer:

> Liveness asks whether the process should be restarted. Readiness asks whether
> the pod should receive traffic right now. A service can be alive but not ready
> during startup, migration dependency failure, draining, or temporary
> downstream unavailability. For Go services in Kubernetes, readiness protects
> users from traffic going to pods that cannot safely serve it.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What metrics would drive autoscaling?

Fast answer:

> CPU and memory are useful but often incomplete. For backend services I would
> consider request rate, p95/p99 latency, in-flight requests, queue lag, oldest
> message age, worker saturation, DB pool wait time, and error rate. In this
> repo, analytics scaling should care about Kafka lag, while API services may
> care more about latency, CPU, and pool pressure.

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

> I fall back to metrics and structured logs, then check why trace propagation
> broke: missing middleware, missing async header injection, sampling, or a
> dependency client not using the request context. Logs should still include
> request ID and trace ID when available. In this repo, `go/pkg/tracing` should
> be checked at HTTP, AMQP, Kafka, Redis, and log boundaries.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you debug queue latency?

Fast answer:

> I separate time waiting in the queue from time spent processing. I look at
> publish rate, consume rate, oldest message age, consumer errors, retry/DLQ
> counts, prefetch or partition limits, and downstream dependency latency. In
> this repo, RabbitMQ saga queues and Kafka analytics topics need different
> views, but oldest unprocessed age is more actionable than just message count.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do retries hide the root cause?

Fast answer:

> Retries can turn one dependency failure into a slow request that eventually
> succeeds, making averages look acceptable while p99 and load get worse. They
> can also hide the original error if only the final attempt is logged. I want
> metrics and traces to show attempt count, backoff time, first error class, and
> final outcome, especially around shared resilience helpers.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is p99 telling you that average latency hides?

Fast answer:

> p99 shows the slowest user-visible tail, where saturation, retries, lock
> contention, GC pauses, queue waits, or dependency spikes usually appear first.
> Average latency can look fine while a small but important group of users sees
> timeouts. For checkout or AI-agent paths, p99 matters because a few slow
> dependency calls can dominate the user experience.

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

> HTTP tracing has a direct request/response chain, but async work crosses
> queues, processes, retries, and delayed consumers. The original caller may be
> gone by the time processing happens, so the trace context has to be stored in
> message headers and extracted later. In this repo, AMQP and Kafka tracing
> helpers are what keep saga and analytics flows connected.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What headers do you propagate?

Fast answer:

> I propagate W3C `traceparent` and `tracestate`, plus baggage only when the
> contents are safe and intentionally bounded. I also keep business correlation
> IDs like order ID or saga ID separate from tracing headers. In this repo,
> those headers belong on AMQP and Kafka messages so consumers can attach spans
> to the right distributed trace.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you handle missing trace context?

Fast answer:

> Missing context should not break processing. I start a new root span, mark
> that the incoming trace context was missing or invalid, and still include
> business correlation IDs in logs and metrics. That lets the system process old
> messages or third-party events while making propagation gaps visible enough to
> fix.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What should logs include?

Fast answer:

> Logs should include trace ID, span ID when available, request or message ID,
> service name, operation, correlation IDs, state transition, latency, and error
> class. They should not include secrets, raw tokens, or sensitive payloads. In
> this repo, logs should let me move from a frontend report to the HTTP span,
> RabbitMQ saga message, Kafka event, or repository call involved.

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

> Event time is when the business event actually happened; processing time is
> when the consumer saw it. For analytics, event time is usually what users
> expect in windows, but processing time tells you about pipeline delay. In this
> repo's analytics windows, event timestamps should drive revenue or abandonment
> buckets, while lag metrics show how stale processing has become.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What is a watermark?

Fast answer:

> A watermark is the system's estimate that it has seen all events up to a
> certain event time, allowing it to close or emit window results. It is a
> tradeoff: wait longer and results are more complete, wait less and dashboards
> are fresher but may miss late events. The analytics service's grace periods
> are the practical version of that tradeoff.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you avoid double counting?

Fast answer:

> I give events stable IDs and make aggregations idempotent by tracking what has
> already been applied or by using deterministic upserts keyed by event/window.
> At-least-once delivery means a Kafka message or webhook can be seen more than
> once. In this repo, analytics consumers should avoid counting duplicate order
> or cart events, and saga consumers should no-op duplicate state transitions.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: When do you drop late events?

Fast answer:

> I drop late events only after an explicit grace period where accepting them
> would make results unstable or too expensive to correct. The rule should be
> different for analytics versus business workflows: late analytics events may
> be dropped or counted in a correction metric, but late payment events may need
> manual investigation or compensation. The decision should be visible in
> metrics.

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

> In-flight requests should get a chance to finish within a shutdown deadline
> after the server stops accepting new traffic. If the deadline expires, the
> request context is canceled and the handler should stop database calls,
> external calls, or streaming writes. In this repo, graceful shutdown helpers
> should coordinate HTTP/gRPC draining and dependency cleanup.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: How do you stop message consumers?

Fast answer:

> I stop pulling new messages, cancel the consumer context, let in-flight
> handlers finish within a deadline, then ack completed work or nack/requeue
> incomplete work according to broker semantics. The handler must be idempotent
> because shutdown can cause redelivery. For RabbitMQ saga consumers and Kafka
> analytics consumers, shutdown behavior should be tested with cancellation and
> in-flight work.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: What if shutdown times out?

Fast answer:

> If shutdown times out, the process should stop waiting and exit, but the
> system design must make that safe: unfinished messages get redelivered,
> idempotency prevents duplicate effects, and partial sagas are recoverable from
> durable state. I would log the timeout, increment a metric, and investigate if
> it happens repeatedly because it usually means handlers are ignoring context
> or dependencies are hanging.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

#### Follow-up: Why does Kubernetes readiness matter before termination?

Fast answer:

> Readiness controls whether Kubernetes sends traffic to the pod. Before
> termination, the pod should become not ready so load balancers stop routing
> new requests while existing work drains. Without that, a pod can receive new
> traffic during shutdown and drop requests or messages. In this repo, readiness
> plus graceful shutdown and PDBs is what makes rolling deploys safe.

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
