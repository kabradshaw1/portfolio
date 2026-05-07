---
name: java-rabbitmq-reliability
description: Use when creating, modifying, reviewing, or debugging Java Spring Boot RabbitMQ publishers or consumers, Spring AMQP RabbitTemplate/RabbitListener configuration, task event DLQs, bounded retry, mandatory publishing, publisher confirms/returns, Micrometer listener metrics, or event deduplication.
---

# Java RabbitMQ Reliability

This skill captures the default reliability standard for Spring AMQP task-event paths in this repo. Apply it when Java services publish or consume RabbitMQ task events.

## Required Semantics

- **Publisher confirms and returns** - publishing code must surface failed confirms and returned mandatory messages.
- **Persistent mandatory events** - task events must be persistent and use mandatory routing.
- **Explicit DLQ topology** - each consumer queue has a DLX and DLQ.
- **Bounded retries** - listener containers use finite retry with exponential backoff.
- **Permanent failure classification** - malformed or semantically invalid messages are rejected without endless retry.
- **Idempotent consumers** - event IDs must dedupe activity and notification side effects.
- **Micrometer visibility** - publish and listener outcomes must be counted with bounded labels.

## Publisher Checklist

- [ ] Ensure task-event payloads include a stable `eventId`.
- [ ] Enable Spring RabbitMQ correlated publisher confirms.
- [ ] Enable publisher returns.
- [ ] Set template mandatory publishing.
- [ ] Make event messages persistent.
- [ ] Configure `RabbitTemplate` confirm and returns callbacks.
- [ ] Log and count failed confirms and returned messages.
- [ ] Propagate publish failures so callers do not treat failed broker delivery as success.

Expected configuration shape:

```yaml
spring:
  rabbitmq:
    publisher-confirm-type: correlated
    publisher-returns: true
    template:
      mandatory: true
```

## Consumer Checklist

- [ ] Declare the service queue, DLQ, and DLX in Spring AMQP config.
- [ ] Set queue argument `x-dead-letter-exchange` to the service DLX.
- [ ] Configure listener prefetch, baseline concurrency, max concurrency, retry attempts, and exponential backoff.
- [ ] Validate required identifiers before side effects.
- [ ] Reject missing `eventId`, required entity IDs, or event type as permanent failures.
- [ ] Dedupe by `eventId` before writing activity records or notifications.
- [ ] Count success, duplicate, ignored, and failure outcomes with Micrometer.

Recommended defaults unless local requirements say otherwise:

- prefetch: `10`
- default concurrency: `1`
- max concurrency: `4`
- retry attempts: `3`
- backoff: exponential

## Service Notes

- `task-service` owns publisher reliability for task events.
- `activity-service` should dedupe persisted activity events by source event ID.
- `notification-service` should dedupe notification writes by recipient and event ID, using Redis `SETNX` or an equivalent atomic guard.

## Tests

Add focused tests for:

- published events include stable IDs
- `RabbitTemplate` mandatory/persistent/confirm/return configuration
- malformed messages fail permanently
- duplicate event IDs do not duplicate side effects
- listener metrics count success, duplicate, ignored, and failure paths

Use Testcontainers RabbitMQ when broker confirms, returns, DLQ routing, or retry behavior cannot be verified reliably with unit tests.
