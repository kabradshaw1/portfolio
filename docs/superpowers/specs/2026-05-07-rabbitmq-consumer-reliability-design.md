# RabbitMQ Consumer Reliability Design

Date: 2026-05-07

## Goal

Upgrade RabbitMQ usage in the Go ecommerce saga path and Java task-event path
to production-level at-least-once messaging behavior. The implementation should
make failure handling explicit, prevent poison-message loops, preserve
operational visibility, and include tests that prove the intended reliability
contract.

## Current State

The Go saga path already has a stronger foundation than the Java task-event
path. `order-service` and `cart-service` use manual acknowledgements, durable
saga queues, a shared dead-letter exchange, trace propagation through AMQP
headers, graceful shutdown waits for in-flight work, and DLQ list/replay
support.

The remaining Go gaps are explicit prefetch/backpressure, bounded retries,
typed failure classification, publisher confirms, mandatory return handling,
connection/channel recovery, persistent reply messages, and message-level
idempotency for duplicate deliveries.

The Java consumers currently rely mostly on Spring AMQP defaults. The activity
and notification queues are durable, but there is no explicit DLX/DLQ, retry
policy, fatal-exception handling, prefetch/concurrency tuning, consumer
metrics, publisher confirms, return handling, or idempotency guard.

## Reliability Contract

RabbitMQ consumers use at-least-once delivery. Handlers must be safe when the
same message is delivered more than once. Retryable failures should be retried
with bounded attempts and backoff. Permanent failures should go to a DLQ without
cycling through the hot queue. Operators should be able to see and replay DLQ
messages deliberately.

Publishers must treat RabbitMQ publish success as a broker-confirmed event for
paths where message loss would break workflow correctness. Unroutable messages
must be surfaced as errors through mandatory publishing and return handling.

## Go Design

Add a small RabbitMQ reliability layer in the Go services that own RabbitMQ
connections. Keep it local and focused rather than introducing a broad
framework. The layer should provide connection/channel setup, topology
declaration, QoS configuration, publish confirms, returned-message handling,
and reconnect loops.

For consumers:

- Set explicit `Qos` prefetch values before consuming saga queues.
- Classify handler failures as retryable or permanent using typed errors, not
  string matching.
- Reject permanent failures with `requeue=false` so they dead-letter.
- Retry transient failures with a bounded policy. The initial implementation
  can use DLQ replay/manual recovery if a delayed retry exchange is not added,
  but the hot queue must not spin indefinitely.
- Preserve graceful shutdown by stopping consumption, waiting for in-flight
  work, and closing channels after consumers have drained.
- Add consumer metrics for received, acked, nacked, retried, dead-lettered,
  handler duration, and handler error class.

For publishers:

- Publish saga commands, saga replies, and payment outbox events with
  persistent delivery mode.
- Enable publisher confirms on AMQP channels used for workflow-critical
  messages.
- Use mandatory publishing and handle returned messages as publish failures.
- Keep trace propagation through AMQP headers.
- Keep the payment service outbox pattern, but only mark an outbox row
  published after the broker confirm is received.

For idempotency:

- Use saga state transitions as the first guard, and add message identifiers
  where they are missing.
- Ensure duplicate saga reply delivery cannot move terminal orders backward or
  emit duplicate irreversible side effects.
- Ensure cart command handlers are safe under duplicate reserve, release, and
  clear-cart commands.

## Java Design

Add explicit Spring AMQP reliability configuration to `activity-service`,
`notification-service`, and the `task-service` publisher.

For consumers:

- Declare each consumer queue with a DLX and service-specific DLQ.
- Configure listener containers with bounded concurrency, prefetch count, and
  manual or clearly documented auto acknowledgement behavior.
- Add retry advice with bounded attempts and exponential backoff.
- Route exhausted or fatal messages to DLQ instead of requeueing indefinitely.
- Treat malformed payloads, conversion errors, missing required identifiers,
  and unsupported message versions as permanent failures.
- Add Micrometer counters/timers for consumed, succeeded, failed, retried, and
  dead-lettered messages.

For publishers:

- Enable publisher confirms and returns in Spring AMQP.
- Configure mandatory publishing for task events.
- Use persistent message delivery mode.
- Surface unroutable publish failures in logs and metrics.

For idempotency:

- Add an event identifier to task event messages if one is not already present
  in the domain event.
- Activity writes should deduplicate by event identifier before inserting an
  `ActivityEvent`.
- Notification writes should deduplicate by event identifier and recipient to
  prevent duplicate notifications after redelivery.

## Testing

Go tests should cover:

- Happy-path saga command and reply flow.
- Malformed JSON, invalid UUID, and unknown commands going to DLQ without hot
  requeue loops.
- Retryable handler failures following the bounded retry path.
- Duplicate saga events and cart commands being idempotent.
- Publisher confirm success and returned-message failure behavior.
- Payment outbox rows being marked published only after confirm.

Java tests should cover:

- Listener happy paths for activity and notification consumers.
- Conversion or validation failures being classified as permanent.
- Retry exhaustion routing messages to DLQ.
- Duplicate event delivery not creating duplicate activity records or
  notifications.
- Publisher confirm and returned-message handling in task-service.

Prefer focused unit tests for failure classification and idempotency, plus
targeted Testcontainers integration tests where broker behavior matters.

## Rollout

Implement in two slices:

1. Go ecommerce saga reliability: `order-service`, `cart-service`, and
   `payment-service`.
2. Java task event reliability: `task-service`, `activity-service`, and
   `notification-service`.

Each slice should preserve existing local and Kubernetes behavior. Configuration
values such as prefetch, retry attempts, and retry backoff should have safe
defaults and environment overrides where operationally useful.

## Out Of Scope

This design does not migrate RabbitMQ to a managed cloud service, add a new
message broker, or redesign the ecommerce saga. It also does not require a
separate event-sourcing system. The goal is to harden the existing RabbitMQ
paths with production-grade delivery, retry, and observability practices.
