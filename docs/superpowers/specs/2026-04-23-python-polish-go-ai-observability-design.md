# Python Production Polish + Go AI-Service Observability

**Date:** 2026-04-23
**Status:** Draft
**Related:** GitHub Issue #78 (Phase 3: Log aggregation — Loki + Grafana)

## Context

The Python AI services (ingestion, chat, debug) are production-ready at a foundational level but have dependency inconsistencies and error response format mismatches that undermine the polish expected in a portfolio project. Separately, the Go ai-service has full observability infrastructure wired (Loki + Promtail + Grafana with traceID-to-Jaeger derived fields) but only ~4 slog calls in the entire service — making it impossible to debug AI agent behavior through Loki despite the plumbing being in place.

This work addresses both gaps in two independent PRs, enabling end-to-end debugging of the AI agent loop and bringing the Python services to a consistent production standard.

## PR 1: Python Production Polish

### 1.1 Dependency Unification

Unify package versions across all three Python services:

| Package | Current (ingestion/debug) | Current (chat) | Target |
|---------|--------------------------|-----------------|--------|
| `uvicorn[standard]` | 0.30.0 | 0.44.0 | 0.44.0 |
| `prometheus-fastapi-instrumentator` | 7.0.2 (debug) / 7.1.0 (ingestion) | 7.1.0 | 7.1.0 |
| `opentelemetry-instrumentation-fastapi` | 0.62b0 | 0.62b0 | Latest stable |
| `opentelemetry-instrumentation-httpx` | 0.62b0 | 0.62b0 | Latest stable |

**Files:**
- `services/ingestion/requirements.txt`
- `services/chat/requirements.txt`
- `services/debug/requirements.txt`

### 1.2 Error Response Standardization

Standardize all error responses to use FastAPI's `{"detail": "..."}` convention, which is already used by Pydantic validation errors (422s).

**Approach:** Audit all `JSONResponse` and manual error dict returns across the three services. Replace any `{"error": "..."}` with `{"detail": "..."}`. This is a search-and-replace audit — no new abstractions needed.

**Files:**
- `services/ingestion/app/main.py`
- `services/chat/app/main.py`
- `services/debug/app/main.py`

### 1.3 Config Fail-Fast Validation

Add a `validate()` method to the shared Settings that runs during the FastAPI lifespan. Validates that provider-required secrets are set at startup rather than failing silently at request time.

**Rules:**
- If `llm_provider == "openai"` and `llm_api_key` is empty → `ValueError` at startup
- If `embedding_provider == "openai"` and `llm_api_key` is empty → `ValueError` at startup
- If `llm_provider == "anthropic"` and `llm_api_key` is empty → `ValueError` at startup
- `ollama` provider requires no API key (local inference)

**Files:**
- `services/ingestion/app/config.py` — each service has its own Settings class
- `services/chat/app/config.py`
- `services/debug/app/config.py`
- Each service's `main.py` lifespan function (call `settings.validate()`)

Note: There is no shared config module. Each service defines its own `Settings(BaseSettings)` in `app/config.py`. The `validate()` method must be added to each independently, or extracted to `services/shared/` as a mixin/function that all three import.

### 1.4 Verification

- `make preflight-python` passes (ruff lint/format, pytest)
- `make preflight-security` passes (bandit, pip-audit)
- All three services start successfully with current env vars
- Intentionally misconfigure provider/key combo and verify startup fails with clear error

---

## PR 2: Go AI-Service Full Instrumentation

### 2.1 Design Principles

- Use `slog` with the existing JSON handler + `tracing.NewLogHandler()` (auto-injects traceID)
- INFO level for normal operations, WARN for degraded paths, DEBUG for cache/verbose detail
- Truncate user-provided strings (args, queries) to 200 chars in logs to prevent log bloat
- Pass `turnID` through the agent loop so all logs in a turn correlate
- No new dependencies — everything uses the existing `log/slog` stdlib package

### 2.2 Layer 1: HTTP Handler (`internal/http/chat.go`)

**Function: `RegisterChatRoutes` handler closure**

Add logging at:
- **Request entry** (INFO): user_id, message_count, auth_source ("header"/"cookie"/"anonymous")
- **Auth parse failure** (WARN): error detail (currently silent)
- **Streaming complete** (INFO): turn_id (from first agent event or generate in handler), duration_ms, events_emitted count

### 2.3 Layer 2: Agent Loop (`internal/agent/agent.go`)

**Function: `Run()`**

Expand existing turn-level logging with per-step detail:

- **Turn start** (INFO): turn_id, user_id, message_count, model
- **History truncation** (INFO, only when truncation occurs): messages_before, messages_after, dropped_count
- **Per LLM call** (INFO): turn_id, step, prompt_eval_count, eval_count, eval_duration_ms, request_duration_ms
- **Per tool call** (INFO): turn_id, step, tool_name, args_preview (first 200 chars), duration_ms, success (bool), result_size (bytes)
- **Tool not found** (WARN): turn_id, step, requested_tool_name
- **Tool panic recovered** (ERROR): turn_id, tool_name, panic_value
- **Context cancelled** (WARN): turn_id, step, reason ("timeout"/"cancelled")

Existing turn summary logs (lines 93-100, 110-117, 183-190) remain unchanged.

### 2.4 Layer 3: LLM Clients (`internal/llm/`)

**ollama.go — `Chat()`:**
- **Request complete** (INFO): model, prompt_tokens, completion_tokens, duration_ms, status_code, tool_call_count
- **HTTP error** (WARN): model, status_code, response_body_preview (first 200 chars)
- **Retry attempt** (INFO): model, attempt_number, delay_ms (requires hook into resilience wrapper or log after retry)

**openai.go — `Chat()`:**
- Same fields as Ollama
- Add OTel span (currently missing — parity with Ollama)
- Add `otelhttp.NewTransport()` (currently missing)

**anthropic.go — `Chat()`:**
- Same fields as Ollama
- Add OTel span (currently missing)
- Add `otelhttp.NewTransport()` (currently missing)

### 2.5 Layer 4: Tool Execution (`internal/tools/`)

Add entry/exit logging to each tool's `Call()` method. Pattern:

```go
slog.InfoContext(ctx, "tool call",
    "tool", t.Name(),
    "user_id", userID,
    "duration_ms", elapsed.Milliseconds(),
    // tool-specific fields below
)
```

**Per-tool fields:**

| Tool | Additional Fields |
|------|-------------------|
| `get_product` | product_id |
| `search_products` | query (truncated), limit, result_count |
| `check_inventory` | product_id, in_stock |
| `list_orders` | order_count |
| `get_order` | order_id |
| `summarize_orders` | order_count, sub_llm_tokens, sub_llm_duration_ms |
| `view_cart` | item_count, cart_total |
| `add_to_cart` | product_id, quantity |
| `initiate_return` | order_id, item_count |
| `search_documents` | query (truncated), collection, result_count |
| `ask_document` | question (truncated), collection |
| `list_collections` | collection_count |

**Error logging** (WARN level) for all tools on failure, with error message.

### 2.6 Layer 5: Cache (`internal/cache/cache.go`)

**`Get()`:**
- DEBUG: key_prefix (first 16 chars of hash), hit/miss

**`Set()` failure:**
- WARN: key_prefix, error (currently fails silently — correct for resilience, but needs visibility)

### 2.7 Layer 6: Guardrails (`internal/guardrails/`)

**ratelimit.go — `Allow()`:**
- WARN (on rejection only): client_ip, current_count, max_allowed, retry_after_seconds
- WARN (on breaker open): client_ip, reason ("circuit_breaker_open"), action ("allowing_request")

**history.go — `TruncateHistory()`:**
- INFO (only when truncation occurs): messages_before, messages_after, dropped_count

### 2.8 What This Enables

After this work, a single Loki query like:

```logql
{app="ai-service"} | json | traceID="<id>"
```

Shows the full request lifecycle:
1. HTTP entry (user_id, auth source)
2. Agent turn start (model, message count)
3. LLM call 1 (tokens, timing)
4. Tool call: search_products (query, results)
5. LLM call 2 (tokens, timing — now with tool context)
6. Tool call: get_product (product_id)
7. LLM call 3 (final response)
8. Agent turn complete (steps, duration, outcome)
9. HTTP streaming complete

Clicking the traceID links to Jaeger for the distributed trace view.

### 2.9 Verification

- `make preflight-go` passes (golangci-lint, go test)
- Deploy to QA and verify logs appear in Loki with correct JSON structure
- Verify traceID is present in all log lines during an agent turn
- Query Loki for `{app="ai-service"} | json | msg="tool call"` and confirm tool-specific fields are present
- Verify no sensitive data in logs (no full user messages, no API keys, no JWTs)

---

## Scope Exclusions

- **Grafana dashboard creation** — the Loki datasource and derived fields are already configured. Dashboard work can follow once the logs are flowing and we know what queries are most useful.
- **Python service logging changes** — Python structured logging is already excellent (structlog + OTel). No changes needed.
- **Multi-stage Docker builds** — valuable but orthogonal to observability.
- **Test coverage targets** — worth addressing but separate from this effort.
