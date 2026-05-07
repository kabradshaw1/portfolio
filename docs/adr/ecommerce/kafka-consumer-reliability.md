# Kafka Consumer Reliability Hardening

- **Date:** 2026-05-07
- **Status:** Accepted
- **Services:** `go/order-projector/`, `go/analytics-service/`, `go/pkg/kafkaconsumer/`

## Context

The ecommerce Kafka consumers had different reliability requirements but shared the
same operational risks:

- Poison records could block or disappear without a durable quarantine path.
- Transient fetch, commit, and persistence failures had limited metrics.
- The order-projector advanced offsets even when projection writes failed, which
  weakened the read model's at-least-once processing contract.
- Replayed order events could double-count hourly stats unless the sink was
  idempotent.
- The analytics-service is intentionally best effort, but invalid records and
  flush health still need explicit visibility.

Kafka only guarantees replay when offsets are committed after successful
processing. That pushes idempotency into the consumer sinks. For the projector,
the read model is user-facing state and must prefer correctness over throughput.
For analytics, small aggregation drift is acceptable, but malformed input should
still be observable and quarantined.

## Decision

Add a shared `go/pkg/kafkaconsumer` package with:

- `DLQPublisher`, which writes a JSON envelope containing source topic,
  partition, offset, key, value, headers, source timestamp, consumer group, error
  class, error message, and failure timestamp.
- Retry and context-aware sleep helpers backed by the existing shared
  `go/pkg/resilience` retry policy.

Harden `order-projector` as the strict consumer:

- Fetch one Kafka record, deserialize it, process all projections, then commit
  the source offset only after successful processing.
- Publish malformed records to `ecommerce.order-events.dlq`; commit the source
  offset only after the DLQ publish succeeds.
- Return processing errors without committing, so Kafka redelivers the event.
- Apply timeline, summary, and stats through a repository-backed processor with
  per-projection duration and error metrics.
- Add `processed_projection_events` and make stats updates atomic with a
  `WITH inserted AS (...)` guard keyed by `(projection_name, event_id)`.
- Protect `order_summary` from stale events with
  `WHERE order_summary.updated_at <= EXCLUDED.updated_at`.

Harden `analytics-service` as bounded best effort:

- Keep committing records after routing because the analytics read model tolerates
  small inaccuracies and should not block on individual aggregation issues.
- Publish invalid envelopes to `ecommerce.analytics.dlq` and count invalid events.
- Add commit error, DLQ publish, DLQ publish error, and last successful flush
  metrics.
- Add configurable consumer group, retry, DLQ topic, and fetch backoff values to
  the service ConfigMaps.

## Consequences

Positive outcomes:

- Order-projector now has a clear at-least-once contract: source offsets move
  only after successful projection writes or successful poison-record
  quarantine.
- Duplicate delivery does not inflate hourly stats because the processed-event
  guard and stats update happen in one SQL statement.
- Stale summary events cannot overwrite newer order state.
- DLQ envelopes preserve enough source metadata to diagnose and replay bad
  records without needing Kafka offset archaeology.
- Consumer reliability is visible through fetch, commit, DLQ, duplicate,
  projection duration, invalid-event, and flush timestamp metrics.

Trade-offs:

- The projector does more work before each offset commit, so throughput is lower
  than a fire-and-forget projection loop. This is acceptable because correctness
  is the primary requirement for the read model.
- Only stats currently use a processed-event guard table. Timeline is already
  idempotent by event ID, and summary is protected by event timestamp ordering.
- DLQ topics must be provisioned and monitored like any other Kafka topic.
- Analytics remains best effort. It now quarantines invalid envelopes and exposes
  flush health, but valid events with malformed inner payloads may still be
  logged and committed rather than retried.
