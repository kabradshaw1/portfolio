# Structured Logging and Distributed Tracing

- **Date:** 2026-04-16
- **Status:** Accepted

## Context

The Python AI services (ingestion, chat, debug) emit log output that feeds into observability tooling. Without structure, those logs are format strings — you can read them in a terminal but can't filter them by field, correlate them across services, or join them with distributed traces. When a `/search` request is slow, finding the cause requires correlating logs from the Go ai-service, the Python chat service, and Qdrant. Without a shared trace identifier, that correlation is guess-work.

Two problems to solve:

1. **Log structure** — logs must be machine-parseable so they can be filtered and searched in aggregators (Loki, ELK, CloudWatch) without regex parsing.
2. **Request correlation** — a single logical request that crosses three services must produce logs and traces that can be joined on a shared identifier.

## 1. structlog Over python-json-logger

`python-json-logger` is a formatter — it serializes the log message and any extra kwargs as JSON. Each call site must pass every field it wants to appear. There's no built-in concept of per-request context.

`structlog` solves this differently:

- **Context variables** — call `structlog.contextvars.bind_contextvars(request_id=..., path=...)` once at the start of a request and every log line emitted during that request automatically carries those fields. No manual passing.
- **Processor pipeline** — each log record passes through a chain of transforms. Individual processors are small, composable, and testable. The final processor renders to JSON or colored console text depending on environment.
- **OpenTelemetry integration** — a custom processor can pull `trace_id` and `span_id` from the active OTel span and inject them into every log record automatically.

`services/shared/logging.py` uses structlog exclusively. The processor chain is configured in `configure_logging()` and both structlog-native and stdlib `logging.getLogger()` calls flow through it via `ProcessorFormatter`.

## 2. Structured Logging

A structured log line is a JSON object, not a format string:

```json
{
  "event": "request_finished",
  "level": "info",
  "timestamp": "2026-04-16T12:34:56.789Z",
  "service": "chat",
  "request_id": "a1b2c3d4-...",
  "method": "POST",
  "path": "/search",
  "status_code": 200,
  "duration_ms": 142.3,
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7"
}
```

Every field is a first-class key. A log aggregator can filter `duration_ms > 1000` or group by `service` without parsing the message string. One structured log line at `request_finished` carries what would otherwise require multiple unstructured lines, a separate metrics call, and regex post-processing.

### Processor Pipeline

The pipeline in `configure_logging()` (`services/shared/logging.py`, lines 79–91) runs every log record through these processors in order:

1. `structlog.contextvars.merge_contextvars` — merges per-request context (request_id, method, path) bound by `RequestLoggingMiddleware`
2. `structlog.processors.add_log_level` — adds `"level": "info"` etc.
3. `structlog.processors.TimeStamper(fmt="iso")` — adds ISO-8601 timestamp
4. `_make_add_service_name(service_name)` — stamps the service name (e.g. `"chat"`, `"ingestion"`)
5. `_add_otel_context` — pulls `trace_id` and `span_id` from the active OTel span
6. `structlog.processors.StackInfoRenderer` — serializes stack info if present
7. `structlog.processors.format_exc_info` — serializes exceptions
8. Final renderer — `JSONRenderer` in production, `ConsoleRenderer` in development

The `foreign_pre_chain` in `ProcessorFormatter` (lines 96–115) applies the same chain to records from third-party libraries that use `logging.getLogger()` directly, so uvicorn, httpx, and LangChain logs also appear as structured JSON.

### RequestLoggingMiddleware

`RequestLoggingMiddleware` (`services/shared/logging.py`, lines 132–170) runs on every incoming HTTP request:

- Generates a UUID4 `request_id` and binds it along with `method` and `path` to structlog's contextvars
- Logs `request_finished` with `status_code` and `duration_ms` on success
- Logs `request_failed` with `exc_info=True` on unhandled exception, then re-raises
- Sets `x-request-id` response header so clients can correlate their own logs

Because context is stored in async-safe contextvars (not a thread-local), this works correctly under uvicorn's async request handling.

## 3. Distributed Tracing

### What Trace Context Is

A **trace** is a tree of **spans**. Each span represents one unit of work (an HTTP handler, a database query, an LLM call). Spans share a `trace_id` — a 128-bit identifier assigned when the first service receives a request. Child spans add their own `span_id`. Together they form a causal tree you can visualize in Jaeger or Grafana Tempo.

### W3C traceparent

The W3C Trace Context standard defines the `traceparent` HTTP header:

```
traceparent: 00-{trace_id}-{span_id}-{flags}
```

- `00` — version
- `trace_id` — 32 hex chars (128 bits), same across the entire request chain
- `span_id` — 16 hex chars (64 bits), unique per span
- `flags` — `01` = sampled, `00` = not sampled

When service A calls service B, it injects its current span context into the outbound request headers. Service B extracts the header and creates a child span under service A's span. The trace_id is preserved.

### Span Tree Across Services

```
Go ai-service (parent span, span_id=aaa)
  ├─ traceparent: 00-{trace_id}-aaa-01
  └─ Python chat service (child span, span_id=bbb)
       ├─ traceparent: 00-{trace_id}-bbb-01
       └─ Qdrant HTTP call (child span, span_id=ccc)
```

**Go side** (`go/pkg/tracing/tracing.go`): `tracing.Init()` registers a global `TracerProvider` and sets `propagation.TraceContext{}` as the global text map propagator. `otelgin` middleware (wired in `go/ai-service/cmd/server/main.go`, line 180) auto-instruments every Gin handler, creating a span and extracting any incoming `traceparent`. Outbound HTTP calls from the ai-service use `otelhttp.NewTransport` which injects `traceparent` automatically.

**Python side** (`services/shared/tracing.py`): `configure_tracing()` sets `TraceContextTextMapPropagator` as the global propagator (line 38). `FastAPIInstrumentor` creates a child span for every incoming request, reading the `traceparent` header to continue the trace. `HTTPXClientInstrumentor` propagates trace context on outbound httpx calls (e.g. to Ollama), linking the full chain.

When `OTEL_EXPORTER_OTLP_ENDPOINT` is not set (local dev, CI), `configure_tracing()` returns `None` — no exporter, zero overhead — but propagation still works so `trace_id` and `span_id` still appear in log lines.

### Logs Carry Trace Context

The `_add_otel_context` processor (`services/shared/logging.py`, lines 20–42) reads the active span from the OTel context at log time and injects `trace_id` and `span_id` into the event dict. Because `FastAPIInstrumentor` creates a span for every request, every log line emitted during that request carries the same `trace_id` as the OTel span — the same ID that appears in Jaeger.

## 4. Correlation: request_id + trace_id

Two identifiers appear on every log line:

| Field | Scope | Generated By |
|-------|-------|--------------|
| `request_id` | Per-service | `RequestLoggingMiddleware` (UUID4 on each incoming request) |
| `trace_id` | Cross-service | OTel instrumentation (propagated via `traceparent` header) |

`request_id` is useful for isolating all logs from a single service handling a single request. `trace_id` is useful for following a logical request across service boundaries.

**Example — debugging a slow `/search` response:**

1. Find the slow request in the Go ai-service logs: `trace_id=4bf92f3...`, `duration_ms=3240`
2. Search Python chat service logs for the same `trace_id` — find the Qdrant search took 2800ms
3. In Jaeger, look up the trace by `trace_id` to see the full span tree and where time was spent

Without `trace_id` on every log line, step 2 requires guessing which chat service request corresponds to which ai-service request based on timestamps.

## 5. Production Considerations

**Sampling.** Exporting every span in high-traffic production generates significant volume. The OTLP exporter in `go/pkg/tracing/tracing.go` uses `sdktrace.WithBatcher` — a `BatchSpanProcessor` with default sampling (100%). In production, add a `ParentBased(TraceIDRatioBased(0.1))` sampler to export ~10% of traces while preserving full traces for errors and slow requests.

**Log volume.** Structured logs are larger per line (JSON overhead vs a bare string) but fewer lines are needed. One `request_finished` line with `status_code`, `duration_ms`, `trace_id`, `request_id`, `method`, and `path` replaces what would otherwise be multiple unstructured lines plus a separate metrics call. The net effect is typically lower total bytes with much higher utility.

**PII.** Never log request bodies, auth tokens, or user-identifying fields. The structlog processor pipeline is the right place to enforce this — a `_redact_sensitive_fields` processor inserted before `JSONRenderer` can strip or mask any key matching a blocklist (e.g. `authorization`, `password`, `token`, `email`). This runs at log time, before anything is written to stdout or shipped to an aggregator.

**Log aggregation.** Structured JSON logs on stdout are ready for:
- **Loki** — label by `service` and `level`, query with LogQL (`{service="chat"} | json | duration_ms > 1000`)
- **ELK** — Filebeat ships stdout, Elasticsearch indexes each JSON key, Kibana filters by field
- **CloudWatch Logs Insights** — native JSON field querying without parsing config

No regex parsing configuration is needed for any of these because every field is already a JSON key.
