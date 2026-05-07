# Kafka Consumer Reliability Design

Date: 2026-05-07

## TL;DR

Harden the Go Kafka consumers around at-least-once delivery with idempotent
sinks. Make `order-projector` the strict correctness showcase: process each
message durably, make every projection idempotent, and commit offsets only
after success or deliberate DLQ quarantine. Keep `analytics-service` bounded
best effort because it aggregates in memory, but add poison-message DLQ support
and clearer reliability metrics. Java currently has RabbitMQ listeners, not
Kafka consumers, so no Java Kafka work is in scope.

## Current State

The repo currently has two Kafka consumers in Go:

- `go/analytics-service/internal/consumer/consumer.go`
- `go/order-projector/internal/consumer/consumer.go`

Java event consumers under `java/activity-service` and
`java/notification-service` use Spring AMQP `@RabbitListener`, not Kafka.

Both Go Kafka consumers already use explicit `FetchMessage` followed by
`CommitMessages`, which is the right base for at-least-once semantics. Both
also expose some health and metrics. The reliability gaps are in what happens
between fetch and commit, and whether duplicate delivery is harmless.

## Goals

- Commit Kafka offsets only after durable success or deliberate quarantine.
- Treat duplicate delivery as expected and make projection writes idempotent.
- Prevent poison records from blocking a partition forever.
- Add enough metrics to make failures, retries, DLQ activity, and duplicates
  visible during review and operation.
- Keep the implementation focused and portfolio-grade without introducing
  Kafka transactions or paid cloud dependencies.

## Non-Goals

- Do not convert Java RabbitMQ listeners to Kafka.
- Do not implement exactly-once Kafka transactions.
- Do not make `analytics-service` a fully durable stream processor in this
  pass.
- Do not add broad architecture docs outside this generated spec.

## Consumer Semantics

Use the existing explicit fetch/process/commit loop as the foundation.

For each fetched message:

1. Decode and validate the envelope.
2. Process the message.
3. Commit the Kafka offset only after one of these is true:
   - the message was durably applied, or
   - the message was identified as poison, published to DLQ, and should not be
     retried from the source topic.
4. If processing fails transiently, retry with bounded exponential backoff and
   jitter.
5. If DLQ publishing fails, do not commit the source offset. The message should
   be retried because neither processing nor quarantine succeeded.

Fetch errors should also use backoff so a broker outage does not create a tight
log and CPU loop.

## Order Projector

`order-projector` should be the strict at-least-once and idempotency showcase.

### Idempotent Projections

The timeline projection already has the right shape because
`order_timeline.event_id` is inserted with `ON CONFLICT DO NOTHING`.

The summary projection should guard against stale or duplicate events changing
state incorrectly. The upsert should preserve existing newer state when the
incoming event timestamp is older than the stored `updated_at`.

The stats projection needs the main correctness fix. It currently increments
hourly counters, so duplicate delivery can overcount. Add a processed-event
guard keyed by projection and event ID, for example:

- `projection_name`
- `event_id`
- `processed_at`

The stats projection should insert the guard row and only increment counters
when the insert succeeds. Redelivered events then become no-ops.

### Transaction Boundary

Each message should apply all required projector writes in a database
transaction where practical:

1. Insert per-projection processed-event guard rows.
2. Apply timeline, summary, and stats changes.
3. Commit the database transaction.
4. Commit the Kafka offset.

If the database transaction fails, do not commit the Kafka offset. Redelivery is
acceptable because successful prior writes are idempotent.

### Error Handling

Malformed JSON, unsupported event shapes that cannot be upgraded, and missing
required IDs should go to `ecommerce.order-events.dlq`. The DLQ payload should
include:

- original topic, partition, offset, key, headers, and value
- consumer group
- error class and error message
- failure timestamp

Projection database errors should be treated as transient unless classified
otherwise. They should retry with backoff and only go to DLQ if the failure is
clearly deterministic and message-specific.

## Analytics Service

`analytics-service` aggregates in memory and flushes windows later. Making it
strictly durable would require a larger state-store redesign, so this pass
should document and improve its current bounded best-effort behavior.

The service should still improve poison-message behavior:

- Invalid envelopes or payloads should publish to `ecommerce.analytics.dlq`.
- After successful DLQ publish, commit the source offset.
- If DLQ publish fails, do not commit.

For valid events, the consumer may continue to commit after routing into the
in-memory aggregators. The tradeoff is explicit: a process crash can lose the
current unflushed window. That is acceptable for this service because these are
approximate real-time analytics, while `order-projector` handles strict read
model correctness.

Add visibility for this tradeoff:

- flush failures by aggregator
- last successful flush timestamp by aggregator
- invalid event count by source topic
- DLQ publish count and DLQ publish failures

## Observability

Add or refine metrics with service, group, and topic labels where cardinality
stays bounded:

- `kafka_consumer_fetch_errors_total`
- `kafka_consumer_commit_errors_total`
- `kafka_consumer_processing_retries_total`
- `kafka_consumer_dlq_published_total`
- `kafka_consumer_dlq_publish_errors_total`
- `projector_duplicate_events_total`
- `projector_projection_duration_seconds`
- `analytics_last_successful_flush_timestamp`

Health responses should distinguish read availability from consumer freshness.
For example, `order-projector` can still serve existing read models while Kafka
processing is degraded, but that degraded state should be visible in health and
metrics.

## Configuration

Move hard-coded group names, retry limits, retry backoff, and DLQ topic names
behind config with sensible defaults. Keep local development simple by
preserving current defaults:

- `analytics-group`
- `order-projector-group`
- `ecommerce.order-events.dlq`
- `ecommerce.analytics.dlq`

## Tests

Add focused tests before implementation:

- duplicate delivery does not double-count `order_stats`
- duplicate delivery does not duplicate timeline rows
- stale or out-of-order events do not downgrade `order_summary`
- projection database failure prevents offset commit
- malformed projector event is published to DLQ and then committed
- DLQ publish failure prevents source offset commit
- fetch errors back off instead of tight-looping
- analytics invalid payload is quarantined without crashing the consumer
- analytics flush failure updates metrics and does not hide stale data risk

## Implementation Approach

Implement in small slices:

1. Add projector processed-event persistence and migration.
2. Refactor projector processing so projection writes can be made idempotent
   and committed before Kafka offsets.
3. Add DLQ publisher abstraction and tests.
4. Add retry/backoff around transient processing and fetch errors.
5. Add analytics DLQ handling and flush observability.
6. Update config, Kubernetes config maps, and tests.

Run `make preflight-go` before committing implementation changes. Java
preflight is not required unless Java files are changed.
