# Distributed Systems Coding Exercises

### 1. In-Memory Saga State Machine

Prompt:

> In-Memory Saga State Machine


Time target: 45 minutes.

Build:

- States: `created`, `items_reserved`, `payment_confirmed`, `completed`,
  `failed`, `compensating`, `compensated`.
- Events: `ItemsReserved`, `PaymentConfirmed`, `PaymentFailed`,
  `ItemsReleased`.
- Invalid transitions return errors.
- Duplicate terminal events are no-ops.

Fast design:

- State transition table.
- Idempotent event handling.
- Compensation.
- Persisting state in a database.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 2. Outbox Poller

Prompt:

> Outbox Poller


Time target: 45 minutes.

Build:

- Fetch unpublished messages.
- Publish each message through an interface.
- Mark published only after publish succeeds.
- Continue processing other messages after one failure.
- Stop on context cancellation.

Fast design:

- At-least-once delivery.
- Duplicate publishes.
- Backoff on database or broker failure.
- Metrics for unpublished age and publish errors.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 3. Consumer With DLQ

Prompt:

> Consumer With DLQ


Time target: 45 minutes.

Build:

- Consume jobs from a channel.
- Retry retryable errors up to N attempts.
- Send poison messages to a DLQ channel.
- Track success/error/DLQ counts.

Fast design:

- Ack/nack timing.
- Retry count storage.
- Poison messages.
- Replaying DLQ safely.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 4. Windowed Aggregator

Prompt:

> Windowed Aggregator


Time target: 45 minutes.

Build:

- Accept events with `eventTime`.
- Aggregate counts into one-minute tumbling windows.
- Allow a 30-second grace period.
- Drop events that are too late.
- Flush expired windows.

Fast design:

- Event time versus processing time.
- Grace periods.
- Late data policy.
- Memory eviction.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ

### 5. Trace Context Carrier

Prompt:

> Trace Context Carrier


Time target: 30 minutes.

Build:

- A simple message struct with headers.
- `InjectTrace(ctx, msg)` and `ExtractTrace(ctx, msg)`.
- Tests that a trace ID survives publish/consume boundaries.

Fast design:

- Why async trace propagation matters.
- Header formats.
- Missing context fallback.
- Logs with trace IDs.

Repo anchors:
- `go/order-service/internal/saga` - checkout saga orchestration, RabbitMQ
