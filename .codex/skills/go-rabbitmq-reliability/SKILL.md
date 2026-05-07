---
name: go-rabbitmq-reliability
description: Use when creating, modifying, reviewing, or debugging Go RabbitMQ publishers or consumers, especially ecommerce saga commands/replies, amqp091-go channels, DLQs, retry headers, publisher confirms, mandatory publishing, prefetch/QoS, or duplicate-safe saga handling.
---

# Go RabbitMQ Reliability

This skill captures the default reliability standard for Go RabbitMQ paths in this repo. Apply it to workflow-critical RabbitMQ publishing and consuming, especially the ecommerce saga path.

## Required Semantics

- **Publisher confirms** - workflow-critical publishes are successful only after broker ack.
- **Mandatory persistent messages** - publish with `mandatory=true` and persistent delivery for saga commands, replies, and outbox messages.
- **Returned-message handling** - unroutable mandatory publishes are errors and must be surfaced.
- **Bounded consumer retries** - classify failures, increment retry headers, retry a finite number of times, then dead-letter.
- **No hot requeue loops** - do not repeatedly `Nack(..., requeue=true)` for retryable business failures.
- **Explicit DLQ routing** - permanent and exhausted failures route to a durable DLQ.
- **Duplicate safety** - handlers must tolerate redelivery after broker or process failure.
- **Operational visibility** - expose bounded-label metrics for publish, consume, retry, DLQ, and duplicate outcomes.

## Shared Package Pattern

Prefer shared helpers under `go/pkg/rabbitmq` when multiple services need the pattern:

- retry header constants and parsing, including AMQP integer variants
- `PermanentError` and `RetryableError` classification helpers
- retry header increment helpers
- confirmed publisher wrapper with mandatory publish, returned-message detection, persistent defaults, and context-aware confirm waits

Run `go mod tidy` in `go/pkg/` and affected services after changing shared RabbitMQ helpers.

## Consumer Checklist

- [ ] Set explicit QoS/prefetch before consuming.
- [ ] Validate message payloads and classify malformed messages as permanent failures.
- [ ] Classify transient dependency errors as retryable failures.
- [ ] Increment `x-retry-count` on retry attempts.
- [ ] Republish retry messages with publisher confirms before acking the original delivery.
- [ ] Route permanent or retry-exhausted messages to DLQ.
- [ ] Ack only after durable processing, confirmed retry republish, or confirmed DLQ routing succeeds.
- [ ] Leave messages unacked or fail the consumer when retry/DLQ publishing cannot be confirmed.
- [ ] Preserve trace context through RabbitMQ headers.
- [ ] Reconnect AMQP connections/channels after broker or channel failure when the service owns a long-running worker.

## Publisher Checklist

- [ ] Use persistent delivery mode unless the message is explicitly ephemeral.
- [ ] Use mandatory publishing and treat returned messages as publish failures.
- [ ] Wait for publisher confirms before marking work complete.
- [ ] For outbox pollers, mark rows published only after broker confirm.
- [ ] Surface nack, return, and context timeout errors to callers and metrics.

## Metrics And Tests

Add Prometheus metrics for:

- publish attempts by outcome
- returned messages and publish nacks
- consumed messages by outcome
- retry attempts and retry exhaustion
- DLQ routing success/failure
- duplicate/redelivery outcomes

Add focused tests for retry header parsing, failure classification, confirm publisher ack/nack/return/timeout behavior, consumer failure decisions, permanent-message DLQ behavior, and duplicate delivery safety. Use Testcontainers RabbitMQ where broker behavior matters.
