# AI Agent Systems Rehearsal

Use this for agent, tool-calling, RAG, streaming, and AI gateway questions. Keep
answers backend-focused: contracts, reliability, safety, and observability.

## Repo Anchors

- `go/ai-service/internal/agent/agent.go`: bounded ReAct-style loop over LLM
  responses, tool calls, tool results, max steps, timeouts, trace spans, and
  metrics.
- `go/ai-service/internal/agent/events.go`: stream event contract:
  `tool_call`, `tool_result`, `tool_error`, `final`, and `error`.
- `go/ai-service/internal/tools/registry.go`: tool interface, JSON schemas,
  in-memory registry, and tool result shape.
- `go/ai-service/internal/tools/*`: ecommerce, cart, order, return, catalog,
  cached, RAG, and composite tools.
- `go/ai-service/internal/tools/clients/rag.go`: typed RAG client with HTTP
  timeouts, OpenTelemetry transport, retry classification, and circuit breaker.
- `go/ai-service/internal/http/chat.go`: POST `/chat`, request validation,
  auth from bearer/cookie, guardrail middleware, and SSE streaming.
- `go/ai-service/internal/guardrails/*`: history truncation, refusal detection,
  and Redis-backed rate limiting.
- `go/ai-service/internal/evals/*`: scripted LLM eval harness for tool calls,
  tool error recovery, max steps, and MCP-style round trips.
- `go/ai-service/internal/metrics/metrics.go`: turn, tool, Ollama, and cache
  metrics.

## High-Frequency Questions

### 1. How does a tool-calling agent work?

Fast answer:

> The loop sends conversation history and tool schemas to the model. If the
> model returns tool calls, the backend validates the tool name, executes the
> tool with context and user identity, appends the tool result back into the
> conversation, and asks the model again. If there are no tool calls, the answer
> is final. In this repo, `agent.Run` implements that bounded loop and emits
> streaming events for every call, result, error, and final answer.

Follow-ups:

- What is the difference between model state and backend state?
- Why do tool schemas matter?
- How do you handle multiple tool calls in one step?
- What gets returned to the client versus fed back to the model?

### 2. How do you prevent runaway agent loops?

Fast answer:

> I put hard bounds around the loop: request context, overall timeout, max
> steps, input size limits, and history truncation. Tool failures are converted
> into tool-result messages so the model can recover, but infrastructure
> failures return an error. In this repo, the agent has `maxSteps`, a per-turn
> timeout, `ErrMaxSteps`, and guardrail history truncation.

Follow-ups:

- What happens when max steps is reached?
- How do you choose the max step count?
- How do you handle client disconnects?
- How do you distinguish tool failure from LLM failure?

### 3. What is a tool registry?

Fast answer:

> A tool registry is the backend catalog of tools the model is allowed to call.
> Each tool exposes a stable name, description, JSON schema, and `Call` method.
> The registry gives schemas to the LLM and resolves tool names at runtime. In
> this repo, `tools.Registry` isolates tool implementations from the agent loop,
> which makes it easier to add ecommerce, RAG, or composite tools without
> rewriting the loop.

Follow-ups:

- How do you version a tool contract?
- How do you test a new tool?
- What if the model calls an unknown tool?
- How do you keep tool output compact?

### 4. How do you integrate RAG into an agent?

Fast answer:

> I keep RAG behind typed backend tools instead of letting the model call random
> services. The tool receives a query, calls retrieval or ask endpoints with a
> context deadline, returns compact sources and answer content to the model, and
> can expose richer display data to the UI. In this repo, the RAG client wraps
> Python chat and ingestion services with a 30-second HTTP client, OTel
> transport, retries, and a circuit breaker.

Follow-ups:

- What should be in the model context?
- How do you cite sources?
- How do you avoid retrying bad RAG requests?
- What happens when retrieval is down?

### 5. How do you stream agent responses?

Fast answer:

> Streaming is useful because agent work has visible intermediate steps. For
> browser clients, SSE is simple for one-way progress. The handler sets
> streaming headers, flushes each event, and after headers are sent it reports
> failures as stream events instead of normal JSON errors. In this repo,
> `/chat` emits `tool_call`, `tool_result`, `tool_error`, `final`, and `error`
> events.

Follow-ups:

- SSE versus WebSocket?
- What if a tool errors after streaming starts?
- How do you detect disconnected clients?
- What write timeout should a streaming server use?

### 6. How do you handle tool errors?

Fast answer:

> Tool errors should not always kill the whole turn. Unknown tools, tool
> failures, and unserializable results are emitted as tool errors and also fed
> back to the model as tool-result messages containing an error. That gives the
> model a chance to explain, retry another path, or produce a fallback. The repo
> also wraps tool calls with `safeCall` so panics become errors instead of
> crashing the process.

Follow-ups:

- Which errors should stop the turn?
- How much error detail should the model see?
- How do you prevent secret leakage in errors?
- How would you test panic recovery?

### 7. What guardrails belong in an AI gateway?

Fast answer:

> I start with ordinary backend controls: auth, input size limits, rate limits,
> context deadlines, allowlisted tools, safe error handling, and telemetry. Then
> I add AI-specific controls like history truncation, refusal detection, source
> grounding for RAG, and evals for common failure cases. In this repo, guardrails
> include message length limits, Redis-backed rate limiting, history truncation,
> and refusal outcome tracking.

Follow-ups:

- What should be enforced before the LLM call?
- What should be enforced after the LLM response?
- How do you handle anonymous users?
- What do you log without storing sensitive prompts?

### 8. How do you evaluate an agent?

Fast answer:

> I evaluate the behavior at the loop boundary, not just individual tools. Test
> cases should cover expected tool selection, tool error recovery, max-step
> enforcement, streaming event order, and final answer behavior. In this repo,
> `internal/evals` uses a scripted LLM and fake tools so the agent can be tested
> deterministically without a live model.

Follow-ups:

- Unit tests versus offline evals?
- What metrics define agent quality?
- How do you catch regressions in tool choice?
- How do you test without Ollama running?

### 9. How do you observe an agent in production?

Fast answer:

> I want traces for each turn, LLM call, and tool call; metrics for turn outcome,
> step count, tool latency, tool error rate, model token counts, and model
> latency; and structured logs with turn ID, user ID, step, tool, and outcome.
> The repo records agent turn metrics, tool metrics, Ollama call metrics, trace
> spans, and structured logs around each turn.

Follow-ups:

- What would you alert on?
- How do you debug high p99 agent latency?
- How do you know if a tool is flaky?
- How do you correlate a frontend report to backend traces?

### 10. Why use a provider abstraction?

Fast answer:

> The agent should depend on an LLM client interface, not a concrete provider.
> That lets tests use scripted responses, production use Ollama or another
> provider, and future model changes avoid touching the loop. The repo's agent
> depends on `llm.Client`, and the eval harness swaps in `ScriptedLLM`.

Follow-ups:

- What belongs in the provider interface?
- How do you handle provider-specific token metrics?
- What changes when switching models?
- How would you test timeout handling?

## Scenario Drills

### Scenario: The model keeps calling the same tool forever.

Answer outline:

> First, the max-step cap stops the runaway turn. Then I inspect streamed events
> and traces to see whether tool output is ambiguous, missing fields, or
> repeatedly erroring. I would tighten the tool schema or result format, add an
> eval that reproduces the loop, and alert on rising `max_steps` outcomes.

### Scenario: RAG is slow and chat p99 jumps.

Answer outline:

> Split the latency by spans: LLM call, RAG HTTP call, and tool execution. Check
> RAG error rate, breaker state, retry count, and whether retries are adding
> tail latency. Short-term, reduce retry budget or return a graceful tool error.
> Longer-term, tune retrieval, cache safe results, or separate RAG timeout from
> the whole turn budget.

### Scenario: A tool panics during a live chat.

Answer outline:

> The tool call should be isolated so the process survives. In this repo,
> `safeCall` recovers from panics, logs the tool name, emits a tool error, and
> feeds an error result back to the model. I would add a regression eval and fix
> the tool input validation or nil handling that caused the panic.

## Coding Exercises

### Exercise 1: Simple tool registry

Prompt:

> Implement a registry that registers tools by name, returns schemas, rejects
> duplicate names, and safely handles unknown tools.

What to say while coding:

- Use an interface for `Name`, `Schema`, and `Call`.
- Store tools in a `map[string]Tool`.
- Validate duplicate names at registration time.
- Unit test unknown tool and schema output.

### Exercise 2: ReAct loop with max steps

Prompt:

> Given a fake LLM that returns tool calls and a fake tool, implement a loop that
> stops on final answer, propagates context cancellation, and returns
> `ErrMaxSteps` after N iterations.

What to say while coding:

- Keep the loop deterministic and bounded.
- Append tool results back into messages.
- Treat tool errors as model-visible results.
- Test final answer, tool error recovery, and max-step behavior.

### Exercise 3: SSE event writer

Prompt:

> Write an HTTP handler helper that encodes named events as SSE and flushes
> after every event.

What to say while coding:

- Set `Content-Type: text/event-stream`.
- Marshal each payload to JSON.
- Write `event:` and `data:` lines.
- Flush if the writer supports `http.Flusher`.
