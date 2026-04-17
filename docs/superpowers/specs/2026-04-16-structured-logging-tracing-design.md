# Structured Logging & Distributed Tracing for Python Services

**Date:** 2026-04-16
**Status:** Draft
**Author:** Kyle Bradshaw + Claude

## Context

The Go ai-service has structured logging (`slog`) and OpenTelemetry tracing to Jaeger. The Python services (chat, ingestion, debug) use unstructured `logging.getLogger()` with no trace propagation. When the Go ai-service calls Python via the new MCP-RAG bridge, the distributed trace ends at the Go boundary — there's no way to correlate a slow `/search` response back through the Python service's retrieval, embedding, and Qdrant calls.

This makes debugging cross-service issues impossible in production. It also leaves the Python services without machine-parseable logs, meaning log aggregation tools can't filter, search, or alert on structured fields.

## Decision

Add `structlog` for structured JSON logging and `opentelemetry` for distributed trace propagation to all three Python services (chat, ingestion, debug). Shared configuration lives in `services/shared/` so all services get identical setup.

## Scope

### Structured logging — `services/shared/logging.py`

New module that configures structlog for all services:

**Automatic fields on every log line:**
- `timestamp` (ISO 8601)
- `service` (service name, e.g., "chat", "ingestion", "debug")
- `level` (info, warning, error, etc.)
- `trace_id` (from OpenTelemetry context, if present)
- `span_id` (from OpenTelemetry context, if present)
- `request_id` (UUID generated per request)
- `logger` (module name)

**Output modes:**
- `LOG_FORMAT=json` (default, production) — one JSON object per line
- `LOG_FORMAT=text` (development) — colored, human-readable console output

**Configuration function:**
```python
def configure_logging(service_name: str, log_format: str = "json") -> None:
```

Called once at service startup in each `main.py`.

### Request logging middleware — `services/shared/logging.py`

A Starlette middleware that:
1. Generates a `request_id` (UUID4) per request
2. Binds `request_id`, `method`, `path` to structlog context
3. Extracts `trace_id` and `span_id` from OpenTelemetry context (if active)
4. Binds trace fields to structlog context
5. Logs request start: `{"event": "request_started", "method": "POST", "path": "/search", ...}`
6. Logs request end: `{"event": "request_finished", "status_code": 200, "duration_ms": 42, ...}`
7. On exception: logs error with traceback before re-raising

### OpenTelemetry tracing — `services/shared/tracing.py`

New module that initializes OpenTelemetry for all services:

**Configuration function:**
```python
def configure_tracing(service_name: str) -> None:
```

**Behavior:**
- Sets up `TracerProvider` with the service name as a resource attribute
- Configures `W3CTraceContextPropagator` (reads/writes `traceparent` header)
- When `OTEL_EXPORTER_OTLP_ENDPOINT` is set: exports spans via OTLP gRPC (to Jaeger or any OTLP-compatible collector)
- When not set: no-op tracer (zero overhead, no exports)
- Instruments FastAPI automatically via `opentelemetry-instrumentation-fastapi` — every endpoint gets a span
- Instruments `httpx` via `opentelemetry-instrumentation-httpx` — outbound calls to Ollama propagate trace context

**Trace flow after this change:**
```
Go ai-service (span: POST /chat) 
  → [traceparent header] 
  → Python chat service (child span: POST /chat)
    → [httpx to Ollama] (child span: POST /api/generate)
    → [Qdrant client] (child span: qdrant.search)
```

The Go ai-service already sends `traceparent` via `otelhttp.NewTransport`. The Python services just need to extract it.

### Per-service changes

Each service (`chat`, `ingestion`, `debug`) needs:

1. **`app/main.py`** — Replace `logging.getLogger(__name__)` with `structlog.get_logger()`. Call `configure_logging()` and `configure_tracing()` at startup. Add the request logging middleware.

2. **All `app/*.py` files** — Update logger calls from positional format strings to keyword arguments:
   ```python
   # Before
   logger.error("Backend service error: %s", e)
   
   # After  
   logger.error("backend_service_error", error=str(e), service="ollama")
   ```

3. **`requirements.txt`** — Add: `structlog`, `opentelemetry-api`, `opentelemetry-sdk`, `opentelemetry-instrumentation-fastapi`, `opentelemetry-instrumentation-httpx`, `opentelemetry-exporter-otlp-proto-grpc`

### Configuration

New environment variables (all optional, with sensible defaults):

| Variable | Default | Purpose |
|----------|---------|---------|
| `LOG_FORMAT` | `json` | `json` for production, `text` for development |
| `LOG_LEVEL` | `INFO` | Minimum log level |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (empty) | Jaeger/OTLP endpoint. Empty = tracing disabled. |
| `OTEL_SERVICE_NAME` | (from code) | Override service name for tracing |

These match the Go services' existing OTel configuration pattern.

### ADR

`docs/adr/structured-logging-tracing.md` covering:
1. **Decision** — Why structlog over python-json-logger, why OTel over manual trace extraction
2. **Structured logging** — What structured logging is, why it matters (machine-parseable, filterable, alertable), the processor pipeline concept
3. **Distributed tracing** — What trace context is, how W3C `traceparent` works, how spans form a tree across services
4. **Correlation** — How trace_id + request_id connect logs to traces to user-visible errors
5. **Production considerations** — Sampling strategies, log volume management, PII in logs

## Files modified

### New files
- `services/shared/logging.py` — structlog configuration + request middleware
- `services/shared/tracing.py` — OpenTelemetry initialization
- `docs/adr/structured-logging-tracing.md` — Learning ADR

### Modified files
- `services/chat/app/main.py` — logging + tracing init, middleware, structlog logger
- `services/chat/app/chain.py` — structlog logger, keyword log args
- `services/chat/app/retriever.py` — structlog logger
- `services/ingestion/app/main.py` — logging + tracing init, middleware, structlog logger
- `services/ingestion/app/embedder.py` — structlog logger
- `services/ingestion/app/store.py` — structlog logger
- `services/debug/app/main.py` — logging + tracing init, middleware, structlog logger
- `services/debug/app/agent.py` — structlog logger
- `services/debug/app/indexer.py` — structlog logger
- `services/debug/app/tools.py` — structlog logger
- `services/chat/requirements.txt` — new dependencies
- `services/ingestion/requirements.txt` — new dependencies
- `services/debug/requirements.txt` — new dependencies

### Test files
- `services/shared/tests/test_logging.py` — verify JSON output, field presence, request_id binding
- `services/shared/tests/test_tracing.py` — verify trace context extraction, no-op when disabled
- Updates to existing test files if logger mocking patterns change

### K8s / Config
- K8s ConfigMaps may need `LOG_FORMAT` and `OTEL_EXPORTER_OTLP_ENDPOINT` added (if Jaeger is running in the monitoring namespace)

## Verification

1. **Preflight:** `make preflight-python` — all existing tests still pass
2. **Local smoke test:** Start chat service with `LOG_FORMAT=text`, send a `/search` request, verify colored structured output in console with request_id
3. **JSON mode:** Start with `LOG_FORMAT=json`, verify each log line is valid JSON with all expected fields
4. **Trace propagation:** Start Go ai-service + Python chat service, send a request through Go, verify Python logs include the same `trace_id` that Go generated
5. **No-op tracing:** Start without `OTEL_EXPORTER_OTLP_ENDPOINT`, verify no errors and no performance impact
6. **CI:** All existing tests pass, no new dependencies break pip-audit

## Out of scope

- Jaeger infrastructure changes (already deployed in monitoring namespace)
- Log aggregation setup (Loki, ELK, etc.)
- Qdrant client instrumentation (Qdrant's Python client doesn't have OTel support; would need manual spans — follow-up)
- Go service logging changes (already using slog)
- Sampling configuration (default: export all spans; tune later based on volume)
