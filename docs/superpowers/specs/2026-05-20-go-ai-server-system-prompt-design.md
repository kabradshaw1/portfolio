# Go AI Server System Prompt Design

## TL;DR

Issue 267 adds a server-owned system prompt to the Go AI assistant HTTP chat path. The server will sanitize client chat history, remove client-supplied system messages, prepend the authoritative assistant prompt, and then pass the resulting messages into the existing agent loop. The agent's current history truncation behavior remains unchanged and continues to preserve the first system message.

## Context

The frontend drawer sends conversation history to `POST /chat` without a server-owned system prompt. `go/ai-service/internal/http/chat.go` currently validates and authenticates the request, then forwards `req.Messages` directly to `agent.Run`. `agent.Run` calls `guardrails.TruncateHistory`, which preserves a first `role: "system"` message, but no server code guarantees that such a message exists or that it is server controlled.

Core assistant behavior should not depend on frontend phrasing or client-controlled chat history.

## Goals

- Insert an authoritative server-owned system prompt for the Go AI assistant HTTP path.
- Strip client-supplied system messages so clients cannot override assistant policy.
- Preserve non-system conversation history order and existing SSE behavior.
- Preserve the existing `guardrails.TruncateHistory` contract, including first-system-message preservation.
- Add focused Go tests for prompt insertion, client-system stripping, and HTTP handoff behavior.

## Non-Goals

- No frontend behavior change is required.
- No LLM provider API change is required.
- No change to tool schemas or tool authorization checks is required.
- No global agent behavior change is required for evals or future non-HTTP callers.

## Design

Add a focused prompt helper for the Go AI service. The helper should expose:

- `ServerSystemPrompt`, a single constant containing the assistant policy.
- A function that accepts client messages and returns a new message slice with:
  - `llm.Message{Role: llm.RoleSystem, Content: ServerSystemPrompt}` at index 0.
  - Every non-system client message appended in its original order.
  - Every client-supplied `RoleSystem` message omitted.

`go/ai-service/internal/http/chat.go` will call this helper after request validation and before constructing `agent.Turn`.

```go
messages := guardrails.WithServerSystemPrompt(req.Messages)
turn := agent.Turn{UserID: userID, Messages: messages}
```

The helper should copy messages into a new slice so the handler does not mutate request data shared with tests or future middleware.

## Prompt Policy

The system prompt should describe the assistant as a shopping assistant for this portfolio ecommerce system. It should instruct the model to use registered tools when product, cart, order, return, recommendation, order-investigation, or document facts are needed.

The prompt must set these boundaries:

- Authenticated tools expose only the current user's data.
- The assistant must not claim access to another user's cart, orders, returns, or account state.
- The assistant should ask clarifying questions when required identifiers, product constraints, quantity, collection name, return details, or user intent are missing.
- Tool errors mean the requested action or lookup did not complete; the assistant should say that clearly, avoid pretending success, and suggest a retry or practical next step.
- Product details, prices, stock, order status, shipping state, return status, and document facts must come from tool results or be described as unavailable.

## Data Flow

1. Frontend sends `messages` to `POST /chat`.
2. Handler validates JSON, message count, message size, rate limits, and authentication as it does today.
3. Handler sanitizes the client message list with the server prompt helper.
4. Handler builds `agent.Turn` with the authenticated `UserID` and sanitized messages.
5. Agent truncates history with `guardrails.TruncateHistory`.
6. Because the server prompt is first, truncation preserves it and keeps the most recent remaining conversation messages.
7. Agent calls the LLM and streams the existing SSE events.

## Error Handling

System-prompt insertion should not introduce new user-facing errors. Client system messages are silently ignored rather than rejected. Existing validation errors, auth errors, admission-control errors, LLM errors, tool errors, and SSE behavior remain unchanged.

## Testing

Add focused Go tests:

- Helper inserts `ServerSystemPrompt` as the first message when the client sends only user and assistant messages.
- Helper strips one or more client-supplied system messages and preserves the non-system messages in order.
- Helper output still preserves the server prompt after `guardrails.TruncateHistory` on a long history.
- HTTP chat handler passes the server-owned prompt to the runner and does not pass a client-supplied system prompt through as the effective system policy.

Relevant test targets are expected to be under `go/ai-service/internal/guardrails` and `go/ai-service/internal/http`.

## Verification

Before committing the implementation, run the relevant Go verification:

```bash
make preflight-go
```

If fast targeted checks are useful during implementation, run package-level tests first:

```bash
cd go/ai-service && go test ./internal/guardrails ./internal/http ./internal/agent
```
