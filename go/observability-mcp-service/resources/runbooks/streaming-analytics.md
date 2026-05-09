# Streaming Analytics Investigation

Use `investigate_streaming_analytics` when Kafka events or analytics projections
look delayed, missing, or unhealthy.

Signals checked:

- Kafka consumer lag for the analytics group.
- Analytics events consumed by topic.
- Kafka consumer error rate.
- Analytics service RED metrics.
- Recent analytics-service error, warn, and exception logs.

Interpretation:

- Non-zero Kafka lag means analytics is behind the stream and may need service
  evidence before assuming data loss.
- Consumer errors with normal request metrics point toward broker, topic, or
  payload handling problems.
- No consumed events with normal producer traffic can indicate topic or group
  configuration drift.

Useful next tools:

- `get_service_evidence` for `go-analytics-service`.
- `search_logs` for focused consumer group or topic errors.
