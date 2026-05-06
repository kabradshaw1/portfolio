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

#### Follow-up: What is the difference between model state and backend state?

Fast answer:

> Model state is the conversation we send back to the LLM: user messages,
> assistant tool calls, and tool-result messages. Backend state is authoritative
> state like the user ID, registry, carts, orders, caches, and deadlines. In this
> repo the agent copies and truncates message history, but tool calls execute
> against backend clients with `turn.UserID`, so the model can request work
> without owning the source of truth.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: Why do tool schemas matter?

Fast answer:

> Schemas are the contract between probabilistic model output and deterministic
> backend code. They give the model names, descriptions, and JSON parameters,
> and they give the backend something to validate and route. In this repo the
> registry exposes schemas on every LLM call, so adding a tool means defining a
> stable name, compact description, and parameter shape instead of adding custom
> logic to the agent loop.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you handle multiple tool calls in one step?

Fast answer:

> I treat them as one assistant step with several ordered backend operations.
> The agent appends the assistant tool-call message, executes each requested
> tool, appends each tool-result message, and then calls the model again. That
> keeps the transcript coherent and makes partial failure explicit: one tool can
> return a `tool_error` while the next tool still runs.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What gets returned to the client versus fed back to the model?

Fast answer:

> The client gets stream events meant for UI progress: tool call names and args,
> display payloads, tool errors, final text, or terminal errors. The model gets
> the message history it needs to continue reasoning: assistant tool calls and
> compact JSON tool-result messages. In this repo `Result.Display` can be richer
> for the frontend, while `Result.Content` is the compact, serializable payload
> fed back to the LLM.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 2. How do you prevent runaway agent loops?

Fast answer:

> I put hard bounds around the loop: request context, overall timeout, max
> steps, input size limits, and history truncation. Tool failures are converted
> into tool-result messages so the model can recover, but infrastructure
> failures return an error. In this repo, the agent has `maxSteps`, a per-turn
> timeout, `ErrMaxSteps`, and guardrail history truncation.

Follow-ups:

#### Follow-up: What happens when max steps is reached?

Fast answer:

> The agent emits an `error` stream event, records the turn outcome as
> `max_steps`, logs the turn with the configured step count, and returns
> `ErrMaxSteps`. That is intentionally a hard failure, not a best-effort final
> answer, because a loop that cannot converge needs a visible signal for evals,
> alerts, and schema fixes.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you choose the max step count?

Fast answer:

> I choose it from the expected workflow depth, then validate it with evals and
> traces. A simple ecommerce answer should need one or two tool rounds; multi-tool
> flows may need a few more. I would keep the default small enough to bound cost
> and latency, then raise it only when real traces show legitimate conversations
> hitting the cap.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you handle client disconnects?

Fast answer:

> The request context has to flow all the way through the LLM call and tool
> clients. In this repo the HTTP handler passes `c.Request.Context()` into
> `runner.Run`, the agent wraps it with a turn timeout, and tools receive that
> context. If the browser disconnects, downstream work should cancel instead of
> continuing to burn model or RAG capacity.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you distinguish tool failure from LLM failure?

Fast answer:

> An LLM failure means the next reasoning step could not be produced, so the turn
> returns an error. A tool failure is usually domain-level or upstream-specific,
> so the agent emits `tool_error`, appends an error-shaped tool result, and lets
> the model recover. That distinction is visible in this repo through separate
> turn outcomes and per-tool metrics.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 3. What is a tool registry?

Fast answer:

> A tool registry is the backend catalog of tools the model is allowed to call.
> Each tool exposes a stable name, description, JSON schema, and `Call` method.
> The registry gives schemas to the LLM and resolves tool names at runtime. In
> this repo, `tools.Registry` isolates tool implementations from the agent loop,
> which makes it easier to add ecommerce, RAG, or composite tools without
> rewriting the loop.

Follow-ups:

#### Follow-up: How do you version a tool contract?

Fast answer:

> I prefer additive versioning first: add optional fields, keep names stable, and
> preserve result shapes the model already learned. If behavior or required
> inputs change materially, introduce a new tool name such as a v2 contract and
> retire the old one after evals pass. The registry makes that explicit because
> the tool name is the runtime routing key.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you test a new tool?

Fast answer:

> I test the tool in isolation for schema, argument parsing, auth/user handling,
> success results, and upstream errors. Then I add an agent-loop or eval test
> proving the model-facing contract works: schema is advertised, the tool is
> resolved from the registry, the result becomes a valid tool message, and errors
> are recoverable.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What if the model calls an unknown tool?

Fast answer:

> The backend must not execute it. In this repo an unknown tool emits a
> `tool_error`, records a tool metric with outcome `unknown`, logs the requested
> name, and appends a tool-result message containing the error. That lets the
> model correct itself while preserving the allowlist boundary.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you keep tool output compact?

Fast answer:

> Split model content from display content. The model should see the fields
> needed for reasoning, like IDs, statuses, source snippets, and short summaries;
> the UI can receive richer cards through `Display`. That avoids wasting prompt
> tokens on frontend details and lowers the chance that a large payload causes
> truncation or bad follow-up tool choices.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 4. How do you integrate RAG into an agent?

Fast answer:

> I keep RAG behind typed backend tools instead of letting the model call random
> services. The tool receives a query, calls retrieval or ask endpoints with a
> context deadline, returns compact sources and answer content to the model, and
> can expose richer display data to the UI. In this repo, the RAG client wraps
> Python chat and ingestion services with a 30-second HTTP client, OTel
> transport, retries, and a circuit breaker.

Follow-ups:

#### Follow-up: What should be in the model context?

Fast answer:

> The model context should include the user question, the compact retrieved
> evidence, source identifiers, and any constraints like collection or limit. It
> should not include whole documents or raw vector-store metadata. In this repo
> the RAG tools return search results and ask answers with filenames, pages,
> scores, and concise text so the agent can reason without bloating the turn.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you cite sources?

Fast answer:

> Citations should come from backend retrieval metadata, not from the model's
> memory. The tool should return source file and page fields alongside the
> answer or snippets, and the final response should cite only those sources. The
> repo's RAG client models `SearchResult` and `AskSource`, which gives the agent
> concrete filename and page values to carry through.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you avoid retrying bad RAG requests?

Fast answer:

> Classify retryability before the retry loop. Validation errors, bad
> collections, malformed requests, and other 4xx responses should fail fast
> because repeating them only adds latency. The RAG client does that by using a
> retry policy that skips errors containing 4xx status text while still allowing
> transient network or server failures to be retried behind the breaker.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What happens when retrieval is down?

Fast answer:

> Retrieval downtime should degrade the turn, not hide as a hallucinated answer.
> The RAG tool returns an error, the agent streams `tool_error`, and the model can
> explain that document lookup is unavailable or answer from non-RAG context if
> that is acceptable. The circuit breaker prevents repeated failing calls from
> dominating p99 latency while the dependency is unhealthy.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 5. How do you stream agent responses?

Fast answer:

> Streaming is useful because agent work has visible intermediate steps. For
> browser clients, SSE is simple for one-way progress. The handler sets
> streaming headers, flushes each event, and after headers are sent it reports
> failures as stream events instead of normal JSON errors. In this repo,
> `/chat` emits `tool_call`, `tool_result`, `tool_error`, `final`, and `error`
> events.

Follow-ups:

#### Follow-up: SSE versus WebSocket?

Fast answer:

> I use SSE when the browser only needs server-to-client progress for one chat
> turn. It is simpler operationally: plain HTTP, easy flushing, and natural
> compatibility with proxies. I use WebSocket when the client needs bidirectional
> low-latency interaction, cancellation messages, or multiplexed sessions beyond
> a single request stream.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What if a tool errors after streaming starts?

Fast answer:

> Once streaming headers are sent, the server cannot switch to a normal JSON
> error response. The right behavior is to emit a `tool_error` or `error` event
> and flush it. This repo does that: after the handler writes the SSE headers,
> agent failures are reported through emitted events, and tool failures remain
> part of the model transcript.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you detect disconnected clients?

Fast answer:

> The primary signal is the request context being canceled when the client goes
> away. The handler passes that context into the agent, and the agent passes it
> into LLM and tool calls. I would also treat failed writes or flushes as a
> signal to stop emitting, but the important contract is context propagation to
> avoid orphaned work.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What write timeout should a streaming server use?

Fast answer:

> It should be longer than a normal JSON request timeout but still bounded by the
> product's latency budget and proxy limits. For this repo I would align the
> stream write timeout with the agent turn timeout plus a small flush margin, and
> keep individual dependency timeouts shorter so a slow RAG or LLM call cannot
> hold the connection indefinitely.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 6. How do you handle tool errors?

Fast answer:

> Tool errors should not always kill the whole turn. Unknown tools, tool
> failures, and unserializable results are emitted as tool errors and also fed
> back to the model as tool-result messages containing an error. That gives the
> model a chance to explain, retry another path, or produce a fallback. The repo
> also wraps tool calls with `safeCall` so panics become errors instead of
> crashing the process.

Follow-ups:

#### Follow-up: Which errors should stop the turn?

Fast answer:

> Stop the turn for infrastructure failures that make continued reasoning
> unreliable: canceled context, LLM provider failure, timeout, or max-step
> exhaustion. Tool errors, unknown tools, and unserializable tool results can be
> converted into tool-result messages because the model may still produce a
> useful fallback. This is exactly the split in `agent.Run`.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How much error detail should the model see?

Fast answer:

> The model should see enough structured detail to recover, not raw internals.
> Good detail is a stable error category, the tool name, and a short reason like
> "order service unavailable" or "invalid collection." Stack traces, hostnames,
> headers, SQL, tokens, and full upstream bodies belong in protected logs or
> traces, not in tool-result content.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you prevent secret leakage in errors?

Fast answer:

> Redact before errors cross trust boundaries. Tool implementations should wrap
> upstream failures with safe messages, logging should avoid full prompts and
> credentials, and streamed errors should be short. In this repo I would keep
> `args_preview` bounded, avoid logging Authorization values, and ensure tool
> errors never include raw headers, connection strings, or secret-backed config.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How would you test panic recovery?

Fast answer:

> I would register a fake tool whose `Call` panics, run the agent with a scripted
> LLM response that invokes it, and assert the process survives. The expected
> stream is a `tool_call`, then `tool_error`, then either a recovery final answer
> or another scripted step. I would also assert the returned error is not a panic
> and the tool metric records an error outcome.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 7. What guardrails belong in an AI gateway?

Fast answer:

> I start with ordinary backend controls: auth, input size limits, rate limits,
> context deadlines, allowlisted tools, safe error handling, and telemetry. Then
> I add AI-specific controls like history truncation, refusal detection, source
> grounding for RAG, and evals for common failure cases. In this repo, guardrails
> include message length limits, Redis-backed rate limiting, history truncation,
> and refusal outcome tracking.

Follow-ups:

#### Follow-up: What should be enforced before the LLM call?

Fast answer:

> Before the LLM call, enforce auth parsing, request size limits, message
> validation, rate limits, history truncation, allowed tool schemas, and context
> deadlines. In this repo those controls show up in the chat handler,
> guardrails middleware, `TruncateHistory`, and the registry-provided schema list
> passed to `llm.Chat`.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What should be enforced after the LLM response?

Fast answer:

> After the LLM response, enforce that tool names are allowlisted, arguments are
> parsed by the tool, tool results are JSON-serializable, final answers can be
> classified for refusal, and all outcomes are recorded. In this repo unknown
> tools do not execute, bad results become `tool_error`, and refusal text changes
> the recorded turn outcome.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you handle anonymous users?

Fast answer:

> Anonymous users can use low-risk read-only paths, but privileged tools must
> require identity. The handler leaves `userID` empty when no bearer token or
> access cookie is present, and tools receive that value. Tool code should treat
> an empty user ID as either anonymous context or a hard authorization failure,
> depending on the operation.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What do you log without storing sensitive prompts?

Fast answer:

> I log operational metadata: turn ID, user ID or anonymous marker, message
> count, model, step, tool name, duration, token counts, outcome, and bounded
> argument previews. I avoid storing full prompts, full tool results, raw
> Authorization headers, cookies, and secrets. If deeper debugging needs prompt
> capture, it should be explicit, sampled, redacted, and access-controlled.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 8. How do you evaluate an agent?

Fast answer:

> I evaluate the behavior at the loop boundary, not just individual tools. Test
> cases should cover expected tool selection, tool error recovery, max-step
> enforcement, streaming event order, and final answer behavior. In this repo,
> `internal/evals` uses a scripted LLM and fake tools so the agent can be tested
> deterministically without a live model.

Follow-ups:

#### Follow-up: Unit tests versus offline evals?

Fast answer:

> Unit tests verify deterministic contracts: registry lookup, schema emission,
> result serialization, panic recovery, and handler streaming. Offline evals
> verify behavior across a turn: whether the agent chooses the right tool,
> recovers from tool errors, and stops at max steps. This repo's build-tagged
> eval package uses `ScriptedLLM` so those cases do not depend on a live model.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What metrics define agent quality?

Fast answer:

> I track task success, final answer correctness, groundedness for RAG, tool
> choice accuracy, recovery rate after tool errors, refusal accuracy, step count,
> latency, token usage, and user-visible error rate. The repo already has useful
> primitives: turn outcomes, steps per turn, tool latency/error metrics, and LLM
> token and request-duration metrics.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you catch regressions in tool choice?

Fast answer:

> Keep a stable suite of scripted scenarios and assert the event sequence and
> tool names. If a prompt, schema description, or provider changes, those evals
> should catch "search products" suddenly calling a cart tool or a RAG question
> skipping retrieval. In this repo the eval harness can compare tool calls and
> max-step behavior without requiring Ollama.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you test without Ollama running?

Fast answer:

> Swap the provider with a fake or scripted implementation of `llm.Client`.
> Because the agent depends only on `Chat(ctx, messages, tools)`, tests can
> return canned final answers, tool calls, or errors. That is how the agent unit
> tests and build-tagged evals exercise the loop without starting Ollama or
> making network calls.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 9. How do you observe an agent in production?

Fast answer:

> I want traces for each turn, LLM call, and tool call; metrics for turn outcome,
> step count, tool latency, tool error rate, model token counts, and model
> latency; and structured logs with turn ID, user ID, step, tool, and outcome.
> The repo records agent turn metrics, tool metrics, Ollama call metrics, trace
> spans, and structured logs around each turn.

Follow-ups:

#### Follow-up: What would you alert on?

Fast answer:

> I would alert on elevated turn error rate, max-step rate, refusal spikes when
> unexpected, p99 turn duration, LLM provider error or latency spikes, tool error
> rate by name, and circuit breaker open rates for dependencies like RAG. Those
> alerts map directly to the repo's turn, tool, and Ollama metrics.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you debug high p99 agent latency?

Fast answer:

> Break the trace into agent turn, LLM calls, and tool calls first. Then compare
> step count, model request duration, token counts, RAG or ecommerce tool spans,
> retry behavior, and breaker state. In this repo the spans and metrics let me
> tell whether p99 is from more loop iterations, a slow provider, a slow tool, or
> streaming clients staying open.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you know if a tool is flaky?

Fast answer:

> Look at tool metrics by name: error rate, latency distribution, and outcome
> mix over time. A flaky tool shows intermittent errors or timeout spikes while
> the rest of the agent remains healthy. The agent also logs turn ID, step, tool
> name, duration, and success, so individual bad calls can be tied back to traces
> and upstream dependency logs.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you correlate a frontend report to backend traces?

Fast answer:

> Give the frontend a correlation ID from the request or response stream and put
> the same ID into backend logs and trace attributes. This repo already creates
> an agent turn ID inside the trace and logs it around LLM and tool work; the
> next production hardening step would be returning that ID in the stream or a
> header so support can jump from a user report to the exact trace.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 10. Why use a provider abstraction?

Fast answer:

> The agent should depend on an LLM client interface, not a concrete provider.
> That lets tests use scripted responses, production use Ollama or another
> provider, and future model changes avoid touching the loop. The repo's agent
> depends on `llm.Client`, and the eval harness swaps in `ScriptedLLM`.

Follow-ups:

#### Follow-up: What belongs in the provider interface?

Fast answer:

> The provider interface should contain the stable capability the agent needs:
> send messages plus tool schemas, receive final text or tool calls, and return
> normalized usage and timing metadata. It should not expose provider-specific
> request bodies to the agent. In this repo that boundary is `llm.Client.Chat`
> returning `llm.ChatResponse`.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How do you handle provider-specific token metrics?

Fast answer:

> Normalize them at the provider boundary, then record them through common
> metric fields. Ollama calls them prompt and eval counts, OpenAI and Anthropic
> expose different names, but the agent should only see prompt tokens,
> completion tokens, request duration, and optional eval duration. This repo's
> `ChatResponse` carries those normalized counts.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: What changes when switching models?

Fast answer:

> The loop should not change, but the adapter, model name, token accounting,
> tool-call JSON mapping, timeout budget, and eval baselines probably will. I
> would run the scripted tests first for contract safety, then run offline evals
> against real prompts to check tool choice, latency, refusal behavior, and RAG
> groundedness before switching production traffic.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

#### Follow-up: How would you test timeout handling?

Fast answer:

> Use a fake `llm.Client` or tool that blocks until the context is canceled, run
> the agent with a very short timeout, and assert the turn returns an error
> promptly. I would also test that downstream tools receive the context and stop
> work. That proves the timeout is enforced by the agent loop, not just by one
> provider implementation.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

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
