# AI Agent Systems Coding Exercises

### 1. Simple tool registry

Prompt:

> Implement a registry that registers tools by name, returns schemas, rejects
> duplicate names, and safely handles unknown tools.

What to say while coding:

- Use an interface for `Name`, `Schema`, and `Call`.
- Store tools in a `map[string]Tool`.
- Validate duplicate names at registration time.
- Unit test unknown tool and schema output.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 2. ReAct loop with max steps

Prompt:

> Given a fake LLM that returns tool calls and a fake tool, implement a loop that
> stops on final answer, propagates context cancellation, and returns
> `ErrMaxSteps` after N iterations.

What to say while coding:

- Keep the loop deterministic and bounded.
- Append tool results back into messages.
- Treat tool errors as model-visible results.
- Test final answer, tool error recovery, and max-step behavior.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM

### 3. SSE event writer

Prompt:

> Write an HTTP handler helper that encodes named events as SSE and flushes
> after every event.

What to say while coding:

- Set `Content-Type: text/event-stream`.
- Marshal each payload to JSON.
- Write `event:` and `data:` lines.
- Flush if the writer supports `http.Flusher`.

Fast design:

> Describe the expected design, edge cases, tests, and tradeoffs.

Repo anchors:
- `go/ai-service/internal/agent/agent.go` - bounded ReAct-style loop over LLM
