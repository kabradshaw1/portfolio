# Python Trace ID Loki Verification Spec

Date: 2026-05-07

## Problem

Python AI services emit `trace_id` and `span_id` from OpenTelemetry context, and
Promtail is configured to normalize Python `trace_id` into Loki's `traceID`
field. The remaining gap is not a known code defect; it is the absence of
end-to-end QA proof that a Go AI-service request can be correlated with Python
service logs in Loki and the same trace in Jaeger.

The last verification attempt was blocked by Loki returning HTTP 500 for:

```bash
scripts/loki-query --ns ai-services --app chat --hours 24
```

Treat that as the first diagnostic step. Do not assume the Promtail mapping is
broken until Loki query health is confirmed.

## Goal

Prove that Python service logs can be queried by distributed `traceID` in Loki
and correlated with the same request trace in Jaeger.

## Non-Goals

- Do not redesign logging across all Python services.
- Do not replace Promtail, Loki, or Jaeger.
- Do not add manual spans unless verification proves auto-instrumentation is
  insufficient for correlation.
- Do not combine this work with RabbitMQ broker metrics, DLQ alerts, or Go
  AI-service timing changes.

## Current Implementation

- `services/shared/logging.py` adds Python `trace_id` and `span_id` to every
  structured log record when an active OpenTelemetry span exists.
- `services/shared/tracing.py` installs W3C trace-context propagation and
  instruments FastAPI and HTTPX.
- `k8s/monitoring/configmaps/promtail-config.yml` parses Go `traceID` and
  Python `trace_id`, then normalizes the value into `traceID`.
- `scripts/loki-query` queries Loki through the Debian runtime host and prints
  `traceID` when it exists in the parsed log payload.

## Open Questions

- Is Loki healthy enough to answer normal `query_range` requests for
  `ai-services`?
- Are Python container labels exposed as `app="chat"`, or does the script need
  to query by a different app/container label?
- Are Python logs reaching Loki with the JSON payload intact after the Promtail
  `output` stage?
- Does the Go AI-service actually send a `traceparent` header to the Python
  service path used during QA verification?
- Does Jaeger receive both Go and Python spans for the same request, or are
  logs correlated even when Python spans are absent?

## Proposed Approach

1. Verify Loki query health before changing code.
   - Run a broad, low-cardinality query against the `ai-services` namespace.
   - If Loki returns HTTP 500, inspect Loki pod health, recent logs, and query
     limits before touching Promtail or Python code.

2. Identify a real Go-to-Python request path.
   - Prefer a shopping assistant request that calls the `chat` service.
   - Capture the Go AI-service log line and its `traceID`.
   - Confirm the request time window so Loki queries stay narrow.

3. Query Python logs for the same trace.
   - Query `ai-services` for the Python app/container in the same time window.
   - Filter by the captured `traceID`.
   - Confirm a non-empty normalized `traceID` appears in the script output or
     the raw Loki payload.

4. Confirm Jaeger correlation.
   - Open or query Jaeger for the same trace ID.
   - Confirm the trace includes the Go AI-service span.
   - Prefer evidence of the Python service span, but if spans are absent while
     logs still correlate by trace ID, document the precise gap.

5. Patch only proven defects.
   - If the script parses Python logs but fails to print `traceID`, fix
     `scripts/loki-query`.
   - If Promtail drops or fails to normalize Python `trace_id`, fix
     `k8s/monitoring/configmaps/promtail-config.yml`.
   - If Python logs do not contain an active span context during instrumented
     requests, investigate `services/shared/logging.py`,
     `services/shared/tracing.py`, and service startup ordering.

## Acceptance Criteria

- Loki `query_range` works for a narrow `ai-services` query without HTTP 500.
- `scripts/loki-query` can print at least one Python service log line with a
  non-empty `traceID`.
- A Go AI-service request and a Python AI-service log line share the same
  `traceID`.
- Jaeger can return the same trace ID, or the spec follow-up clearly documents
  why Jaeger lacks the Python span while Loki correlation works.
- Any code or config fix is limited to the files required by the evidence.

## Likely Files

- `k8s/monitoring/configmaps/promtail-config.yml`
- `services/shared/logging.py`
- `services/shared/tracing.py`
- `scripts/loki-query`

## Verification Commands

Use Debian only for runtime diagnostics, because Loki and Jaeger live in the
cluster:

```bash
scripts/loki-query --ns ai-services --hours 1 --limit 10
scripts/loki-query --ns ai-services --app chat --hours 1 --limit 20
scripts/loki-query --ns go-ecommerce --app go-ai-service --hours 1 --limit 20
```

If repo files change, run the relevant local preflight before commit:

```bash
make preflight-python
```

If only Kubernetes monitoring config or shell script behavior changes, run the
smallest available local validation path from the repo preflight. Leave live
cluster proof to QA deployment unless Kyle explicitly asks for an immediate
runtime verification.

## Handoff Notes

The expected output of this work is mostly evidence, not necessarily a patch.
A successful run should leave behind:

- the Go request timestamp and `traceID`;
- the Python log query that returned the same `traceID`;
- the Jaeger trace lookup result;
- any targeted fix required to make those checks pass.
