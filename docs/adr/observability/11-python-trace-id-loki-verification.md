# Python Trace ID Loki Verification

- **Date:** 2026-05-07
- **Status:** Accepted

## Context

Python AI services already emit OpenTelemetry `trace_id` and `span_id` fields
in structured logs when an active span exists. Promtail was configured to read
both Go `traceID` and Python `trace_id`, but end-to-end verification showed two
gaps in the operational path:

- `scripts/loki-query --hours 24` could exceed Loki's configured one-day query
  range by a few seconds and return HTTP 500.
- Python structlog records use `event` and `trace_id`, while `scripts/loki-query`
  only displayed `msg` or `message` and only printed `traceID`.

Runtime verification also found two separate deployment gaps outside this
decision: the Go AI service was configured with stale Python RAG service names,
and Python chat did not export spans to Jaeger because its deployment did not
set an OTLP exporter endpoint.

## Decision

Normalize Python trace IDs at the Loki query and Promtail pipeline boundaries.

`scripts/loki-query` should:

- keep `--hours 24` inside Loki's one-day `query_range` limit;
- reject windows larger than 24 hours with a clear error;
- fail cleanly when Loki returns an error response;
- display Python structlog `event` records;
- print either `traceID` or Python `trace_id` as `traceID`.

Promtail should preserve the normalized `traceID` value as structured metadata
after mapping Python `trace_id` into the common field. This keeps the normalized
trace value available for newly ingested Python logs without increasing stream
cardinality through a trace ID label.

## Consequences

Operators can query Python AI-service logs by trace ID through the existing
`scripts/loki-query` workflow and see the same `traceID` field used by Go logs.
The script also avoids a known Loki query-limit failure mode during 24-hour
lookbacks.

Keeping `traceID` as structured metadata avoids high-cardinality Loki labels,
but it means previously ingested logs still depend on payload parsing. The
Promtail metadata change only affects logs ingested after the updated config is
deployed.

This decision does not fix stale Go RAG service URLs or Python span export to
Jaeger. Those remain separate deployment/configuration follow-ups.
