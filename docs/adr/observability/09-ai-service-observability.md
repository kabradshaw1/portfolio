# AI-Service Observability: Full Agent Loop Instrumentation

- **Date:** 2026-04-23
- **Status:** Accepted

## Context

The observability stack (ADRs 01-06) and debuggability improvements (ADR 07) brought all decomposed ecommerce services to a solid instrumentation baseline — every gRPC call metered, service-layer decisions logged, saga steps timed. But the ai-service was left out. It had only 4 `slog` calls in the entire codebase (turn-level summaries in `agent.go`), despite being the most complex service to debug:

- The agent loop makes multiple LLM calls per request, each taking 2-30 seconds
- Each LLM call can trigger 1-N tool calls, each calling external APIs
- Tools call the ecommerce services (REST), Python RAG services (HTTP), and Redis (cache)
- A single user request can generate 3-8 LLM roundtrips with interleaved tool executions
- When something goes wrong, the question is always "which step failed, and why?"

The Loki infrastructure was fully deployed (Promtail DaemonSet, JSON parsing pipeline, Grafana datasource with traceID-to-Jaeger derived fields), but there was nothing useful to query. The ai-service's JSON logs only showed turn summaries — no visibility into the per-step behavior that makes AI agents hard to debug.

Separately, the Python AI services (ingestion, chat, debug) had drifted on dependency versions and had inconsistent error response formats — minor issues that undermine the production polish expected in a portfolio project.

### What prompted this

A production readiness evaluation of the Python services revealed the dependency drift. Investigating GitHub issue #78 (Phase 3: Log aggregation) revealed that the Loki infrastructure was already deployed but underutilized — the ai-service was the biggest gap. The two concerns were independent but both pointed at "observability polish."

## Decision

### 1. Six-layer instrumentation of the ai-service

Added structured `slog` logging at every decision point in the ai-service, organized by layer:

**Layer 1 — HTTP handler** (`internal/http/chat.go`):
- Request entry: user_id, message_count, auth_source (header/cookie/anonymous)
- Request completion: duration_ms, events_emitted (SSE event count)

**Layer 2 — Agent loop** (`internal/agent/agent.go`):
- Turn start: turn_id, user_id, message_count, model, max_steps
- Per-LLM call: step, prompt_tokens, completion_tokens, duration_ms, tool_call_count
- Per-tool call: tool name, args_preview (truncated to 200 chars), duration_ms, success
- Unknown tool warning, panic recovery error

**Layer 3 — LLM clients** (`internal/llm/{ollama,openai,anthropic}.go`):
- Successful response: provider, model, token counts, duration_ms, tool_call_count
- HTTP error: provider, model, status code, body_preview (truncated to 200 chars)

**Layer 4 — Cache** (`internal/cache/cache.go`):
- WARN on fail-open paths (breaker open or Redis error) with key_prefix and error

**Layer 5 — Guardrails** (`internal/guardrails/`):
- Rate limit rejection: client_ip, retry_after_s
- Rate limiter fail-open: key, error
- History truncation: messages_before, messages_after, dropped

**Layer 6 — Tools** (`internal/tools/{catalog,orders,cart,returns,rag}.go`):
- All 12 tools: INFO on success with timing and tool-specific fields, WARN on error
- Consistent message format: `"tool result"` / `"tool error"` with `"tool"` field for Loki filtering

### 2. Context propagation for traceID injection

All `slog` calls in the agent loop use `slog.InfoContext(ctx, ...)` / `slog.WarnContext(ctx, ...)` instead of the context-free variants. This is critical because `tracing.NewLogHandler()` (from `go/pkg/tracing/logging.go`) extracts the traceID from the span in the context and injects it into every log record. Without `ctx`, the most important logs in the system would be missing the field that ties them together in Loki.

The one exception is panic recovery in `safeCall()`, which uses `slog.Error()` without context — a panic may have corrupted the context, and losing the traceID on a panic log is an acceptable trade-off vs risking a secondary panic.

**Alternative considered:** Adding a `slog.Logger` field to the `Agent` struct with pre-bound attributes (turn_id, user_id). Rejected because Go's `slog` context propagation already handles this cleanly — `ctx` carries both the OTel span (for traceID) and any request-scoped values.

### 3. OTel span parity across LLM providers

The OpenAI and Anthropic clients were missing OTel spans and `otelhttp.NewTransport`, while the Ollama client had both. Added:

- `otel.Tracer("llm").Start(ctx, "openai.chat")` / `"anthropic.chat"` spans with model attribute
- `otelhttp.NewTransport(http.DefaultTransport)` for automatic HTTP trace propagation
- `span.SetAttributes` for token counts (same fields as Ollama)

This means all three providers now produce child spans under the agent turn span, visible in Jaeger as:

```
agent.turn
  ├── agent.llm_call (step=0)
  │     └── openai.chat (or ollama.chat / anthropic.chat)
  ├── agent.tool_call (tool=search_products)
  ├── agent.llm_call (step=1)
  │     └── openai.chat
  └── agent.tool_call (tool=get_product)
```

### 4. Truncation discipline for log safety

User-provided strings (queries, questions, tool args) are truncated to 200 characters in all log statements. A shared `truncate(s string, n int) string` helper in `internal/tools/catalog.go` handles this for all tool files. The agent loop uses inline truncation for `argsPreview`.

This prevents:
- Log bloat from large tool arguments (e.g., full product descriptions)
- Potential log injection from user-controlled input
- Prometheus/Loki label cardinality issues if log fields were ever promoted to labels

### 5. Python production polish

Three minor but important consistency fixes:

- **Dependency unification:** uvicorn 0.30.0 → 0.44.0 (ingestion, debug), prometheus-instrumentator 7.0.2 → 7.1.0 (debug). All three services now share identical versions.
- **Error response standardization:** Rate limit handlers changed from `{"error": "Rate limit exceeded"}` to `{"detail": "Rate limit exceeded"}`, matching FastAPI's default convention for Pydantic validation errors.
- **Config fail-fast validation:** Added `validate()` to each service's `Settings` class. If `llm_provider` or `embedding_provider` is set to `openai` or `anthropic` but the corresponding API key is empty, the service raises `ValueError` at startup instead of silently failing at request time.

## Consequences

**Positive:**

- A single Loki query `{app="ai-service"} | json | traceID="<id>"` now shows the complete request lifecycle — from HTTP entry through every LLM call and tool invocation to streaming completion
- Token counts per LLM call are logged, enabling cost analysis and performance profiling without external tooling
- Tool execution timing is separately observable from LLM inference timing — "slow because Ollama is thinking" vs "slow because the product API is down" is now answerable from logs alone
- Cache fail-open events are no longer silent — Redis outages produce WARN logs instead of invisible cache misses
- All three LLM providers have identical observability (spans + logs), making provider comparison possible in Jaeger and Loki
- Python services fail fast on misconfiguration instead of returning opaque 500s at request time

**Trade-offs:**

- ~40 new log points increase log volume. Mitigated by logging decisions and results (not raw request/response bodies) and using DEBUG level for cache hits (not visible at default INFO level)
- `truncate()` helper at 200 chars is a judgment call — could miss useful context in very long queries. Acceptable because the full data is in the OTel span attributes (Jaeger), and Loki is for quick triage, not forensics
- Tool log messages use a generic `"tool result"` / `"tool error"` pattern rather than tool-specific messages. This optimizes for Loki query consistency (`|= "tool result"`) at the cost of slightly less readable raw logs

**Remaining gaps:**

- No Grafana dashboard for AI agent debugging — deferred until logs are flowing in QA and we know which queries are most useful
- `summarize_orders` tool's sub-LLM call timing is folded into overall tool duration rather than being separately logged — a minor gap for this specific tool
- `TruncateHistory()` in `guardrails/history.go` logs without context (no traceID) because it doesn't receive `ctx` in its signature — acceptable since it's called once per turn and the agent loop's turn-start log provides correlation
