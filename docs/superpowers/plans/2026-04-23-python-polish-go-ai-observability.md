# Python Production Polish + Go AI-Service Observability

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish Python services to production consistency and add comprehensive structured logging to the Go ai-service so the entire agent loop is debuggable in Loki.

**Architecture:** Two independent PRs. PR 1 unifies Python dependency versions, standardizes HTTP error responses, and adds config validation. PR 2 adds `slog` calls at every decision point in the Go ai-service (HTTP handler, agent loop, LLM clients, tools, cache, guardrails) — using the existing JSON handler + traceID injection.

**Tech Stack:** Python (FastAPI, Pydantic, structlog), Go (slog, OTel, Gin, gobreaker)

---

## PR 1: Python Production Polish

### Task 1: Unify Python dependency versions

**Files:**
- Modify: `services/ingestion/requirements.txt:2` (uvicorn)
- Modify: `services/debug/requirements.txt:2` (uvicorn)
- Modify: `services/debug/requirements.txt:11` (prometheus-instrumentator)

Note: OTel instrumentation 0.62b0 is already the latest release (all OTel instrumentation packages use pre-release versioning). No upgrade needed.

- [ ] **Step 1: Update ingestion/requirements.txt**

Change line 2 from `uvicorn[standard]==0.30.0` to `uvicorn[standard]==0.44.0`.

```
uvicorn[standard]==0.44.0
```

- [ ] **Step 2: Update debug/requirements.txt**

Change line 2 from `uvicorn[standard]==0.30.0` to `uvicorn[standard]==0.44.0`.
Change line 11 from `prometheus-fastapi-instrumentator==7.0.2` to `prometheus-fastapi-instrumentator==7.1.0`.

```
uvicorn[standard]==0.44.0
```
```
prometheus-fastapi-instrumentator==7.1.0
```

- [ ] **Step 3: Verify all three files are consistent**

Run:
```bash
diff <(grep -E '^(uvicorn|prometheus|opentelemetry)' services/ingestion/requirements.txt | sort) \
     <(grep -E '^(uvicorn|prometheus|opentelemetry)' services/chat/requirements.txt | sort)
diff <(grep -E '^(uvicorn|prometheus|opentelemetry)' services/ingestion/requirements.txt | sort) \
     <(grep -E '^(uvicorn|prometheus|opentelemetry)' services/debug/requirements.txt | sort)
```

Expected: No output (files match on these packages).

- [ ] **Step 4: Run preflight**

Run: `make preflight-python`
Expected: All ruff lint/format + pytest pass.

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/requirements.txt services/debug/requirements.txt
git commit -m "fix(python): unify uvicorn and prometheus-instrumentator versions across services"
```

---

### Task 2: Standardize error response format

**Files:**
- Modify: `services/ingestion/app/main.py:55`
- Modify: `services/chat/app/main.py:50`
- Modify: `services/debug/app/main.py:51`

The only HTTP-level `{"error": ...}` responses are the rate limit handlers. SSE streaming error events (`{"error": "Service unavailable"}` in chat) are part of the SSE wire format, not HTTP error responses — leave those unchanged.

- [ ] **Step 1: Update ingestion rate limit handler**

In `services/ingestion/app/main.py`, line 55, change:

```python
return JSONResponse(status_code=429, content={"error": "Rate limit exceeded"})
```

to:

```python
return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})
```

- [ ] **Step 2: Update chat rate limit handler**

In `services/chat/app/main.py`, line 50, change:

```python
return JSONResponse(status_code=429, content={"error": "Rate limit exceeded"})
```

to:

```python
return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})
```

- [ ] **Step 3: Update debug rate limit handler**

In `services/debug/app/main.py`, line 51, change:

```python
return JSONResponse(status_code=429, content={"error": "Rate limit exceeded"})
```

to:

```python
return JSONResponse(status_code=429, content={"detail": "Rate limit exceeded"})
```

- [ ] **Step 4: Update any tests that assert on the error key**

Search for tests that check for `"error"` in rate limit responses:

```bash
grep -rn '"error"' services/*/tests/ | grep -i rate
```

Update any matches to assert `"detail"` instead of `"error"`.

- [ ] **Step 5: Run preflight**

Run: `make preflight-python`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/ingestion/app/main.py services/chat/app/main.py services/debug/app/main.py
# Also add any updated test files
git commit -m "fix(python): standardize error responses to use FastAPI detail convention"
```

---

### Task 3: Add config fail-fast validation

**Files:**
- Modify: `services/ingestion/app/config.py`
- Modify: `services/chat/app/config.py`
- Modify: `services/debug/app/config.py`

Each service has its own `Settings` class. Add a `validate()` method to each that checks provider-specific secrets at import time. Since the `settings = Settings()` singleton is created at module level and all three services use it at import time, call `validate()` right after instantiation.

- [ ] **Step 1: Add validate() to ingestion config**

In `services/ingestion/app/config.py`, add after the `get_embedding_base_url` method:

```python
    def validate(self) -> None:
        """Fail fast if provider-required secrets are missing."""
        api_key_providers = ("openai", "anthropic")
        if self.llm_provider in api_key_providers and not self.llm_api_key:
            raise ValueError(
                f"llm_api_key is required when llm_provider is '{self.llm_provider}'"
            )
        if self.embedding_provider in api_key_providers and not self.embedding_api_key:
            raise ValueError(
                f"embedding_api_key is required when embedding_provider is '{self.embedding_provider}'"
            )
```

Then change the bottom of the file from:

```python
settings = Settings()
```

to:

```python
settings = Settings()
settings.validate()
```

- [ ] **Step 2: Add validate() to chat config**

In `services/chat/app/config.py`, add the same `validate()` method after `get_embedding_base_url`:

```python
    def validate(self) -> None:
        """Fail fast if provider-required secrets are missing."""
        api_key_providers = ("openai", "anthropic")
        if self.llm_provider in api_key_providers and not self.llm_api_key:
            raise ValueError(
                f"llm_api_key is required when llm_provider is '{self.llm_provider}'"
            )
        if self.embedding_provider in api_key_providers and not self.embedding_api_key:
            raise ValueError(
                f"embedding_api_key is required when embedding_provider is '{self.embedding_provider}'"
            )
```

Then change:

```python
settings = Settings()
```

to:

```python
settings = Settings()
settings.validate()
```

- [ ] **Step 3: Add validate() to debug config**

In `services/debug/app/config.py`, add the same `validate()` method after `get_embedding_base_url`:

```python
    def validate(self) -> None:
        """Fail fast if provider-required secrets are missing."""
        api_key_providers = ("openai", "anthropic")
        if self.llm_provider in api_key_providers and not self.llm_api_key:
            raise ValueError(
                f"llm_api_key is required when llm_provider is '{self.llm_provider}'"
            )
        if self.embedding_provider in api_key_providers and not self.embedding_api_key:
            raise ValueError(
                f"embedding_api_key is required when embedding_provider is '{self.embedding_provider}'"
            )
```

Then change:

```python
settings = Settings()
```

to:

```python
settings = Settings()
settings.validate()
```

- [ ] **Step 4: Verify default config passes validation**

The default provider is `"ollama"` which requires no API key — existing tests and local dev should work unchanged.

Run: `make preflight-python`
Expected: All tests pass (default ollama provider skips key validation).

- [ ] **Step 5: Commit**

```bash
git add services/ingestion/app/config.py services/chat/app/config.py services/debug/app/config.py
git commit -m "feat(python): fail fast on missing API keys for non-ollama providers"
```

- [ ] **Step 6: Run full preflight**

Run: `make preflight-python && make preflight-security`
Expected: All checks pass.

---

## PR 2: Go AI-Service Full Instrumentation

### Task 4: Add logging to HTTP chat handler

**Files:**
- Modify: `go/ai-service/internal/http/chat.go`

- [ ] **Step 1: Add slog import**

In `go/ai-service/internal/http/chat.go`, add `"log/slog"` and `"time"` to the import block.

```go
import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kabradshaw1/portfolio/go/ai-service/internal/agent"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/auth"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/guardrails"
	"github.com/kabradshaw1/portfolio/go/ai-service/internal/llm"
	"github.com/kabradshaw1/portfolio/go/pkg/apperror"
)
```

- [ ] **Step 2: Add request logging at handler entry and completion**

In the handler closure inside `RegisterChatRoutes`, after the auth block (after line 72) and before SSE headers (line 74), add:

```go
		authSource := "anonymous"
		if authHeader != "" {
			authSource = "header"
		} else if cookieErr == nil && cookieToken != "" {
			authSource = "cookie"
		}
		slog.InfoContext(c.Request.Context(), "chat request",
			"user_id", userID,
			"message_count", len(req.Messages),
			"auth_source", authSource,
		)
		requestStart := time.Now()
		eventsEmitted := 0
```

Then wrap the `emit` callback to count events. Replace the existing emit definition (lines 82-90) with:

```go
		emit := func(e agent.Event) {
			eventsEmitted++
			name, payload := eventName(e)
			data, _ := json.Marshal(payload)
			_, _ = c.Writer.WriteString("event: " + name + "\n")
			_, _ = c.Writer.WriteString("data: " + string(data) + "\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
```

After the `runner.Run()` call and its error emit (after line 102), add:

```go
		slog.InfoContext(c.Request.Context(), "chat complete",
			"user_id", userID,
			"duration_ms", time.Since(requestStart).Milliseconds(),
			"events_emitted", eventsEmitted,
		)
```

- [ ] **Step 3: Run tests**

Run: `cd go/ai-service && go test ./internal/http/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add go/ai-service/internal/http/chat.go
git commit -m "feat(ai-service): add structured logging to chat HTTP handler"
```

---

### Task 5: Add per-step logging to agent loop

**Files:**
- Modify: `go/ai-service/internal/agent/agent.go`

- [ ] **Step 1: Add turn start log**

In `Run()`, add a turn start log right after the turnSpan setup (after line 71, before `stepsCompleted`). History truncation logging is handled in Task 8 (inside `guardrails/history.go`).

```go
	slog.Info("agent turn start",
		"turn_id", turnID,
		"user_id", turn.UserID,
		"message_count", len(turn.Messages),
		"model", a.model,
		"max_steps", a.maxSteps,
	)
```

- [ ] **Step 2: Add per-LLM-call logging**

Inside the main loop, after `llmSpan.End()` (line 82) and the metrics recording (lines 83-89), add a log for successful LLM calls:

```go
		if err == nil {
			operation := "chat"
			if step == a.maxSteps-1 {
				operation = "chat_final"
			}
			a.rec.RecordOllamaCall(a.model, operation, resp.RequestDuration, resp.PromptEvalCount, resp.EvalCount, resp.EvalDurationNs)
			slog.Info("llm call",
				"turn_id", turnID,
				"step", step,
				"prompt_tokens", resp.PromptEvalCount,
				"completion_tokens", resp.EvalCount,
				"duration_ms", resp.RequestDuration.Milliseconds(),
				"tool_call_count", len(resp.ToolCalls),
			)
		}
```

- [ ] **Step 3: Add per-tool-call logging**

Inside the tool call loop (lines 131-178), after the tool metric recording (line 155-156), add logging.

After `a.rec.RecordTool(call.Name, outcome, time.Since(toolStart))` and `toolsCalled = append(...)`, add:

```go
			argsPreview := string(call.Args)
			if len(argsPreview) > 200 {
				argsPreview = argsPreview[:200] + "..."
			}
			slog.Info("tool call",
				"turn_id", turnID,
				"step", step,
				"tool", call.Name,
				"args_preview", argsPreview,
				"duration_ms", time.Since(toolStart).Milliseconds(),
				"success", toolErr == nil,
			)
```

- [ ] **Step 4: Add logging for tool-not-found and panic recovery**

For the tool-not-found case (lines 139-147), add after the existing `emit` and `messages = append`:

```go
			if !ok {
				errMsg := "unknown tool: " + call.Name
				emit(Event{ToolError: &ToolErrorEvent{Name: call.Name, Error: errMsg}})
				msg, _ := llm.ToolResultMessage(call.ID, call.Name, map[string]string{"error": errMsg})
				messages = append(messages, msg)
				a.rec.RecordTool(call.Name, "unknown", 0)
				slog.Warn("unknown tool requested",
					"turn_id", turnID,
					"step", step,
					"tool", call.Name,
				)
				toolSpan.End()
				continue
			}
```

For panic recovery in `safeCall`, add logging:

```go
func safeCall(ctx context.Context, t tools.Tool, args json.RawMessage, userID string) (result tools.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %q panicked: %v", t.Name(), r)
			slog.Error("tool panic recovered",
				"tool", t.Name(),
				"panic", fmt.Sprintf("%v", r),
			)
		}
	}()
	return t.Call(ctx, args, userID)
}
```

- [ ] **Step 5: Run tests**

Run: `cd go/ai-service && go test ./internal/agent/ -v -count=1`
Expected: All tests pass. New log lines appear in test output (harmless).

- [ ] **Step 6: Commit**

```bash
git add go/ai-service/internal/agent/agent.go
git commit -m "feat(ai-service): add per-step structured logging to agent loop"
```

---

### Task 6: Add logging to LLM clients

**Files:**
- Modify: `go/ai-service/internal/llm/ollama.go`
- Modify: `go/ai-service/internal/llm/openai.go`
- Modify: `go/ai-service/internal/llm/anthropic.go`

- [ ] **Step 1: Add logging to Ollama client**

In `go/ai-service/internal/llm/ollama.go`, add `"log/slog"` to the import block.

After the successful response decode (after line 136, before span.SetAttributes), add:

```go
		slog.InfoContext(ctx, "llm response",
			"provider", "ollama",
			"model", c.model,
			"prompt_tokens", parsed.PromptEvalCount,
			"completion_tokens", parsed.EvalCount,
			"duration_ms", time.Since(start).Milliseconds(),
			"tool_call_count", len(parsed.Message.ToolCalls),
		)
```

For the HTTP error case (lines 128-131), add before the return:

```go
		if resp.StatusCode >= 400 {
			payload, _ := io.ReadAll(resp.Body)
			bodyPreview := string(payload)
			if len(bodyPreview) > 200 {
				bodyPreview = bodyPreview[:200] + "..."
			}
			slog.WarnContext(ctx, "llm http error",
				"provider", "ollama",
				"model", c.model,
				"status", resp.StatusCode,
				"body_preview", bodyPreview,
			)
			return ChatResponse{}, fmt.Errorf("ollama status %d: %s", resp.StatusCode, string(payload))
		}
```

- [ ] **Step 2: Add logging + OTel span to OpenAI client**

In `go/ai-service/internal/llm/openai.go`, add imports:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
```

In `NewOpenAIClient`, add `otelhttp.NewTransport`:

```go
func NewOpenAIClient(baseURL, model, apiKey string) *OpenAIClient {
	return &OpenAIClient{
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}
```

In `Chat()`, add a span at the start of the function (before message conversion):

```go
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (ChatResponse, error) {
	ctx, span := otel.Tracer("llm").Start(ctx, "openai.chat",
		trace.WithAttributes(attribute.String("llm.model", c.model)),
	)
	defer span.End()
```

After the successful response decode (after line 148, before building `out`), add:

```go
	span.SetAttributes(
		attribute.Int("llm.prompt_tokens", parsed.Usage.PromptTokens),
		attribute.Int("llm.completion_tokens", parsed.Usage.CompletionTokens),
	)
	slog.InfoContext(ctx, "llm response",
		"provider", "openai",
		"model", c.model,
		"prompt_tokens", parsed.Usage.PromptTokens,
		"completion_tokens", parsed.Usage.CompletionTokens,
		"duration_ms", time.Since(start).Milliseconds(),
		"tool_call_count", len(parsed.Choices[0].Message.ToolCalls),
	)
```

For the HTTP error case (lines 136-138), add logging:

```go
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		bodyPreview := string(payload)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		slog.WarnContext(ctx, "llm http error",
			"provider", "openai",
			"model", c.model,
			"status", resp.StatusCode,
			"body_preview", bodyPreview,
		)
		return ChatResponse{}, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(payload))
	}
```

- [ ] **Step 3: Add logging + OTel span to Anthropic client**

In `go/ai-service/internal/llm/anthropic.go`, add the same imports as OpenAI:

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
```

In `NewAnthropicClient`, add `otelhttp.NewTransport`:

```go
func NewAnthropicClient(model, apiKey string) *AnthropicClient {
	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)},
	}
}
```

In `Chat()`, add a span at the start (after separating system message logic):

```go
func (c *AnthropicClient) Chat(ctx context.Context, messages []Message, tools []ToolSchema) (ChatResponse, error) {
	ctx, span := otel.Tracer("llm").Start(ctx, "anthropic.chat",
		trace.WithAttributes(attribute.String("llm.model", c.model)),
	)
	defer span.End()
```

After the response decode (after line 156), add:

```go
	span.SetAttributes(
		attribute.Int("llm.prompt_tokens", parsed.Usage.InputTokens),
		attribute.Int("llm.completion_tokens", parsed.Usage.OutputTokens),
	)
	slog.InfoContext(ctx, "llm response",
		"provider", "anthropic",
		"model", c.model,
		"prompt_tokens", parsed.Usage.InputTokens,
		"completion_tokens", parsed.Usage.OutputTokens,
		"duration_ms", time.Since(start).Milliseconds(),
		"tool_call_count", countToolUseBlocks(parsed.Content),
	)
```

Add a helper at the bottom of the file:

```go
func countToolUseBlocks(blocks []struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}) int {
	n := 0
	for _, b := range blocks {
		if b.Type == "tool_use" {
			n++
		}
	}
	return n
}
```

Actually, the `anthropicResp` Content field uses an anonymous struct. To avoid referencing the anonymous type, count tool calls from the built `out.ToolCalls` instead. Replace the slog call to use `len(out.ToolCalls)` — but `out` isn't built yet at that point. Move the log after building `out` (after the content block loop, line 174). So place the logging right before `return out, nil`:

```go
	slog.InfoContext(ctx, "llm response",
		"provider", "anthropic",
		"model", c.model,
		"prompt_tokens", parsed.Usage.InputTokens,
		"completion_tokens", parsed.Usage.OutputTokens,
		"duration_ms", time.Since(start).Milliseconds(),
		"tool_call_count", len(out.ToolCalls),
	)
	return out, nil
```

For the HTTP error case (lines 148-151), add logging (same pattern as Ollama/OpenAI):

```go
	if resp.StatusCode >= 400 {
		payload, _ := io.ReadAll(resp.Body)
		bodyPreview := string(payload)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		slog.WarnContext(ctx, "llm http error",
			"provider", "anthropic",
			"model", c.model,
			"status", resp.StatusCode,
			"body_preview", bodyPreview,
		)
		return ChatResponse{}, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(payload))
	}
```

- [ ] **Step 4: Run go mod tidy for new OTel imports**

The OpenAI and Anthropic clients now import `otelhttp` and `otel`. These are already in `go.mod` (used by Ollama), so no new deps needed, but verify:

Run: `cd go/ai-service && go mod tidy`
Expected: No changes (deps already present).

- [ ] **Step 5: Run tests**

Run: `cd go/ai-service && go test ./internal/llm/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add go/ai-service/internal/llm/ollama.go go/ai-service/internal/llm/openai.go go/ai-service/internal/llm/anthropic.go
git commit -m "feat(ai-service): add structured logging and OTel spans to all LLM clients"
```

---

### Task 7: Add logging to cache operations

**Files:**
- Modify: `go/ai-service/internal/cache/cache.go`

- [ ] **Step 1: Add slog import and logging**

In `go/ai-service/internal/cache/cache.go`, add `"log/slog"` to imports.

In `Get()`, after the breaker execute block (line 53-57), add logging for cache misses on error (fail-open path):

```go
	if err != nil {
		// Fail open: breaker open or Redis error → treat as cache miss.
		slog.WarnContext(ctx, "cache get fail-open",
			"key_prefix", key[:min(len(key), 16)],
			"error", err.Error(),
		)
		return nil, false, nil
	}
```

In `Set()`, in the fail-open error path (lines 70-73), add:

```go
	if err != nil {
		// Fail open: skip write silently.
		slog.WarnContext(ctx, "cache set fail-open",
			"key_prefix", key[:min(len(key), 16)],
			"error", err.Error(),
		)
		return nil
	}
```

- [ ] **Step 2: Run tests**

Run: `cd go/ai-service && go test ./internal/cache/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add go/ai-service/internal/cache/cache.go
git commit -m "feat(ai-service): log cache fail-open events for debugging"
```

---

### Task 8: Add logging to guardrails

**Files:**
- Modify: `go/ai-service/internal/guardrails/ratelimit.go`
- Modify: `go/ai-service/internal/guardrails/history.go`

- [ ] **Step 1: Add rate limit rejection logging**

In `go/ai-service/internal/guardrails/ratelimit.go`, add `"log/slog"` to imports.

In the `Middleware` function, in the `!ok` branch (lines 76-79), add logging before the abort:

```go
		if !ok {
			slog.Warn("rate limit exceeded",
				"client_ip", c.ClientIP(),
				"retry_after_s", int(retry.Seconds()),
			)
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
```

In the `Allow()` method, in the fail-open path (lines 55-57), add:

```go
	if err != nil {
		// Fail open on breaker-open or Redis errors.
		slog.Warn("rate limiter fail-open",
			"key", key,
			"error", err.Error(),
		)
		return true, 0, nil
	}
```

- [ ] **Step 2: Add history truncation logging**

In `go/ai-service/internal/guardrails/history.go`, add `"log/slog"` to imports.

Replace the `TruncateHistory` function with a version that logs when truncation occurs:

```go
func TruncateHistory(msgs []llm.Message, n int) []llm.Message {
	if len(msgs) <= n {
		return msgs
	}
	dropped := len(msgs) - n
	slog.Info("history truncated",
		"messages_before", len(msgs),
		"messages_after", n,
		"dropped", dropped,
	)
	if len(msgs) == 0 || msgs[0].Role != llm.RoleSystem {
		return append([]llm.Message(nil), msgs[len(msgs)-n:]...)
	}
	out := make([]llm.Message, 0, n)
	out = append(out, msgs[0])
	out = append(out, msgs[len(msgs)-(n-1):]...)
	return out
}
```

Note: This is the canonical location for truncation logging (captures all callers). Task 5 does not add truncation logging in agent.go.

- [ ] **Step 3: Run tests**

Run: `cd go/ai-service && go test ./internal/guardrails/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add go/ai-service/internal/guardrails/ratelimit.go go/ai-service/internal/guardrails/history.go
git commit -m "feat(ai-service): add logging to rate limiter and history truncation"
```

---

### Task 9: Add logging to tool cache wrapper

**Files:**
- Modify: `go/ai-service/internal/tools/cached.go`

- [ ] **Step 1: Add slog import and logging**

In `go/ai-service/internal/tools/cached.go`, add `"log/slog"` to imports.

In the `Call()` method, add a debug log for cache hits (inside the `if ok` block, line 36-41):

```go
	if err == nil {
		if raw, ok, _ := t.cache.Get(ctx, key); ok {
			var content any
			if json.Unmarshal(raw, &content) == nil {
				metrics.CacheEvents.WithLabelValues("tool", "hit").Inc()
				slog.DebugContext(ctx, "tool cache hit",
					"tool", t.inner.Name(),
					"key_prefix", key[:min(len(key), 16)],
				)
				return Result{Content: content}, nil
			}
		}
	}
```

- [ ] **Step 2: Run tests**

Run: `cd go/ai-service && go test ./internal/tools/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add go/ai-service/internal/tools/cached.go
git commit -m "feat(ai-service): add debug logging to tool cache wrapper"
```

---

### Task 10: Add logging to catalog tools

**Files:**
- Modify: `go/ai-service/internal/tools/catalog.go`

- [ ] **Step 1: Add slog import**

In `go/ai-service/internal/tools/catalog.go`, add `"log/slog"` and `"time"` to imports.

- [ ] **Step 2: Add logging to get_product**

In `getProductTool.Call()`, add timing and logging. Wrap the API call:

```go
func (t *getProductTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	var a getProductArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("get_product: bad args: %w", err)
	}
	if a.ProductID == "" {
		return Result{}, errors.New("get_product: product_id is required")
	}
	p, err := t.api.GetProduct(ctx, a.ProductID)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "get_product", "product_id", a.ProductID, "error", err.Error())
		return Result{}, fmt.Errorf("get_product: %w", err)
	}
	slog.InfoContext(ctx, "tool result", "tool", "get_product", "product_id", a.ProductID, "duration_ms", time.Since(start).Milliseconds())
	if t.kafkaPub != nil {
		kafka.SafePublish(ctx, t.kafkaPub, "ecommerce.views", p.ID, kafka.Event{
			Type: "product.viewed",
			Data: map[string]any{"productID": p.ID, "productName": p.Name, "source": "detail"},
		})
	}
	return Result{
		Content: map[string]any{"id": p.ID, "name": p.Name, "price": p.Price, "stock": p.Stock},
		Display: map[string]any{"kind": "product_card", "product": p},
	}, nil
}
```

- [ ] **Step 3: Add logging to search_products**

In `searchProductsTool.Call()`, add timing and logging:

```go
func (t *searchProductsTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	var a searchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("search_products: bad args: %w", err)
	}
	if a.Query == "" {
		return Result{}, errors.New("search_products: query is required")
	}
	limit := a.Limit
	if limit <= 0 || limit > maxSearchResults {
		limit = maxSearchResults
	}

	prods, err := t.api.ListProducts(ctx, a.Query, limit)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "search_products", "query", truncate(a.Query, 200), "error", err.Error())
		return Result{}, fmt.Errorf("search_products: %w", err)
	}

	out := make([]map[string]any, 0, len(prods))
	for _, p := range prods {
		if a.MaxPrice > 0 && p.Price > int(a.MaxPrice*100) {
			continue
		}
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "price": p.Price, "stock": p.Stock,
		})
		if len(out) >= limit {
			break
		}
	}
	slog.InfoContext(ctx, "tool result", "tool", "search_products", "query", truncate(a.Query, 200), "result_count", len(out), "duration_ms", time.Since(start).Milliseconds())
	if t.kafkaPub != nil {
		for _, p := range prods {
			kafka.SafePublish(ctx, t.kafkaPub, "ecommerce.views", p.ID, kafka.Event{
				Type: "product.viewed",
				Data: map[string]any{"productID": p.ID, "productName": p.Name, "source": "search"},
			})
		}
	}
	return Result{
		Content: out,
		Display: map[string]any{"kind": "product_list", "products": out},
	}, nil
}
```

- [ ] **Step 4: Add logging to check_inventory**

In `checkInventoryTool.Call()`:

```go
func (t *checkInventoryTool) Call(ctx context.Context, args json.RawMessage, userID string) (Result, error) {
	start := time.Now()
	var a getProductArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{}, fmt.Errorf("check_inventory: bad args: %w", err)
	}
	if a.ProductID == "" {
		return Result{}, errors.New("check_inventory: product_id is required")
	}
	p, err := t.api.GetProduct(ctx, a.ProductID)
	if err != nil {
		slog.WarnContext(ctx, "tool error", "tool", "check_inventory", "product_id", a.ProductID, "error", err.Error())
		return Result{}, fmt.Errorf("check_inventory: %w", err)
	}
	content := map[string]any{
		"product_id": p.ID,
		"stock":      p.Stock,
		"in_stock":   p.Stock > 0,
	}
	slog.InfoContext(ctx, "tool result", "tool", "check_inventory", "product_id", a.ProductID, "in_stock", p.Stock > 0, "duration_ms", time.Since(start).Milliseconds())
	return Result{
		Content: content,
		Display: map[string]any{"kind": "inventory", "product_id": p.ID, "stock": p.Stock, "in_stock": p.Stock > 0},
	}, nil
}
```

- [ ] **Step 5: Add truncate helper**

Add at the bottom of `catalog.go`:

```go
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

- [ ] **Step 6: Run tests**

Run: `cd go/ai-service && go test ./internal/tools/ -v -count=1 -run TestCatalog`
Expected: All catalog tests pass.

- [ ] **Step 7: Commit**

```bash
git add go/ai-service/internal/tools/catalog.go
git commit -m "feat(ai-service): add structured logging to catalog tools"
```

---

### Task 11: Add logging to order, cart, and return tools

**Files:**
- Modify: `go/ai-service/internal/tools/orders.go`
- Modify: `go/ai-service/internal/tools/cart.go`
- Modify: `go/ai-service/internal/tools/returns.go`

- [ ] **Step 1: Add logging to orders.go**

Add `"log/slog"` and `"time"` to imports.

In `listOrdersTool.Call()`, add after the successful `api.ListOrders` call:

```go
	slog.InfoContext(ctx, "tool result", "tool", "list_orders", "order_count", len(orders), "duration_ms", time.Since(start).Milliseconds())
```

Add `start := time.Now()` at the top of the function (after the auth check).

For error path, add:
```go
	slog.WarnContext(ctx, "tool error", "tool", "list_orders", "error", err.Error())
```

Apply the same pattern to `getOrderTool.Call()` (log order_id on success) and `summarizeOrdersTool.Call()` (log order_count and sub_llm call).

For `summarizeOrdersTool`, the sub-LLM call is particularly important. After `t.llm.Chat()` returns, log:

```go
	slog.InfoContext(ctx, "tool sub-llm call", "tool", "summarize_orders", "order_count", len(orders), "duration_ms", time.Since(llmStart).Milliseconds())
```

- [ ] **Step 2: Add logging to cart.go**

Add `"log/slog"` and `"time"` to imports.

In `viewCartTool.Call()`, add after the successful `api.GetCart` call:

```go
	slog.InfoContext(ctx, "tool result", "tool", "view_cart", "item_count", len(cart.Items), "duration_ms", time.Since(start).Milliseconds())
```

In `addToCartTool.Call()`, add after the successful `api.AddToCart` call:

```go
	slog.InfoContext(ctx, "tool result", "tool", "add_to_cart", "product_id", a.ProductID, "quantity", a.Quantity, "duration_ms", time.Since(start).Milliseconds())
```

Both with `start := time.Now()` at top and WARN log on error paths.

- [ ] **Step 3: Add logging to returns.go**

Add `"log/slog"` and `"time"` to imports.

In `initiateReturnTool.Call()`:

```go
	slog.InfoContext(ctx, "tool result", "tool", "initiate_return", "order_id", a.OrderID, "item_count", len(a.ItemIDs), "duration_ms", time.Since(start).Milliseconds())
```

With WARN log on error path.

- [ ] **Step 4: Run tests**

Run: `cd go/ai-service && go test ./internal/tools/ -v -count=1`
Expected: All tool tests pass.

- [ ] **Step 5: Commit**

```bash
git add go/ai-service/internal/tools/orders.go go/ai-service/internal/tools/cart.go go/ai-service/internal/tools/returns.go
git commit -m "feat(ai-service): add structured logging to order, cart, and return tools"
```

---

### Task 12: Add logging to RAG tools

**Files:**
- Modify: `go/ai-service/internal/tools/rag.go`

- [ ] **Step 1: Add logging to RAG tools**

Add `"log/slog"` and `"time"` to imports.

In `searchDocumentsTool.Call()`:

```go
	slog.InfoContext(ctx, "tool result", "tool", "search_documents", "query", truncate(a.Query, 200), "collection", collection, "result_count", len(out), "duration_ms", time.Since(start).Milliseconds())
```

In `askDocumentTool.Call()`:

```go
	slog.InfoContext(ctx, "tool result", "tool", "ask_document", "question", truncate(a.Question, 200), "collection", collection, "duration_ms", time.Since(start).Milliseconds())
```

In `listCollectionsTool.Call()`:

```go
	slog.InfoContext(ctx, "tool result", "tool", "list_collections", "collection_count", len(collections), "duration_ms", time.Since(start).Milliseconds())
```

All with `start := time.Now()` and WARN on error paths.

Note: `truncate()` was defined in `catalog.go`. Since it's in the same `tools` package, it's accessible here. No need to redefine.

- [ ] **Step 2: Run tests**

Run: `cd go/ai-service && go test ./internal/tools/ -v -count=1 -run TestRAG`
Expected: All RAG tool tests pass.

- [ ] **Step 3: Commit**

```bash
git add go/ai-service/internal/tools/rag.go
git commit -m "feat(ai-service): add structured logging to RAG tools"
```

---

### Task 13: Final verification

- [ ] **Step 1: Run full Go preflight**

Run: `make preflight-go`
Expected: golangci-lint clean, all tests pass with `-race`.

- [ ] **Step 2: Verify no sensitive data in log statements**

Grep for potential leaks:

```bash
grep -rn 'apiKey\|api_key\|password\|secret\|token\|jwt' go/ai-service/internal/ | grep -i slog
```

Expected: No matches. API keys, JWTs, and secrets must never appear in slog calls.

- [ ] **Step 3: Verify log consistency**

Check that all tool logs use the same field names:

```bash
grep -rn 'slog\.' go/ai-service/internal/tools/*.go | grep -oP '"[a-z_]+"' | sort | uniq -c | sort -rn
```

Expected: Consistent field names across tools (tool, duration_ms, error, etc.).

- [ ] **Step 4: Commit if any lint fixes needed**

If golangci-lint required changes, commit:

```bash
git add -A go/ai-service/
git commit -m "fix(ai-service): lint fixes for observability logging"
```
