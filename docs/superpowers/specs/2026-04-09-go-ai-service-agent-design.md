# `go/ai-service` — Go Agent Harness over Ollama Tool Calling

**Date:** 2026-04-09
**Status:** Design — ready for implementation plan
**Supersedes:** `2026-04-07-ai-enhancements-roadmap.md` (Tracks A, B-Go, C; Java Track B dropped)
**Related:** `docs/adr/rag-reevaluation-2026-04.md` (to be written as part of this work)

## 1. Goal & Positioning

Build `go/ai-service`, a new Go microservice that runs an LLM agent loop over Ollama's native tool-calling API. Its tools wrap real ecommerce and order operations, giving a unified shopping + concierge agent on `/go/ecommerce`. The tool registry is transport-agnostic so an MCP adapter can be layered on later without touching tool code.

**Portfolio story:**
> "Go microservice that runs an LLM agent loop, dispatches typed tool calls against a real ecommerce backend, and is operated like a production system (evals, caching, metrics, guardrails)."

**Why this replaces the prior roadmap:** RAG is now commodity in the 2026 hiring market, and long-context models have eroded naive RAG's differentiation. The portfolio's existing Doc Q&A and Debug Assistant already check the RAG box. The scarcer, higher-signal skill is **agents with tool use in a real backend service**, and the job search is Go-focused — so the AI story must live in Go, not Python or Java.

**Non-goals:**
- No new RAG work. Semantic product search is one tool among many, not the centerpiece.
- No MCP implementation in v1 — only a registry shape that admits one later.
- No agent memory or long-term personalization. Each session is stateless beyond the current conversation.
- No multi-agent orchestration.
- No `place_order` / checkout tool. The agent cannot move money.
- No Java stack integration. The Java services stay functional but are out of scope for this project.

## 2. Architecture

### 2.1 Service placement

```
Browser (/go/ecommerce)
   │
   ▼
Next.js frontend ──► ai-service ──► Ollama (Windows, tool-calling chat)
                        │
                        ├──► ecommerce-service (REST) — catalog, cart, orders, returns
                        ├──► Qdrant (Go client)       — semantic product search tool
                        └──► Redis                    — embedding cache, response cache
```

- `ai-service` is a sibling to `auth-service` and `ecommerce-service` under `go/`, deployed to the `go-ecommerce` namespace.
- `ai-service` **never talks to Postgres directly**. Every data access goes through `ecommerce-service` over HTTP, which forwards the user's JWT. This guarantees the agent can only do what the user could have done through the normal API.
- Ollama and Qdrant are reached cross-namespace via the existing DNS names (`ollama.ai-services.svc.cluster.local`, `qdrant.ai-services.svc.cluster.local`).
- Redis is the existing `go-ecommerce` Redis, namespaced under `ai:*` keys.

### 2.2 Internal layout

```
go/ai-service/
├── cmd/server/main.go
├── internal/
│   ├── agent/          # Loop. Owns conversation state, dispatch, step cap, error handling.
│   ├── llm/            # Ollama client. One interface, one impl. Tool-calling request/response shape.
│   ├── tools/
│   │   ├── registry.go # type Tool interface { Name, Description, Schema, Call }
│   │   ├── catalog.go  # search_products, get_product, check_inventory
│   │   ├── orders.go   # list_orders, get_order, summarize_orders, initiate_return
│   │   ├── cart.go     # add_to_cart, view_cart
│   │   └── clients/    # Typed HTTP clients for ecommerce-service, Qdrant, Redis
│   ├── http/           # Handlers: POST /chat (SSE), GET /health, GET /ready, GET /metrics
│   ├── auth/           # JWT validation against auth-service signing key
│   ├── cache/          # Redis-backed embedding + response cache
│   ├── evals/          # Eval harness (build-tagged)
│   └── observability/  # Prometheus metrics, structured logging
└── migrations/         # Empty in v1 — no relational schema owned by this service
```

**Boundary rationale:**
- `agent` knows nothing about HTTP, tools, or Ollama specifics. It operates on an `llm.Client` interface and a `tools.Registry`. This is what makes the harness testable and what lets MCP slot in later.
- `tools.Tool` is the only interface MCP will ever need to satisfy. An MCP adapter becomes a single file that pulls tool schemas from an MCP server and registers them as `Tool` implementations.
- `llm` is swappable — a hosted-model client (Anthropic/OpenAI) is one file if wanted for an interview demo.
- `clients/` is separate from `tools/` so tools stay thin: typed arg struct, client call, result shape.

### 2.3 Auth model

- Frontend sends the user's existing JWT (from auth-service) on `POST /chat`.
- `ai-service` validates the JWT, extracts `user_id`, and passes it explicitly to tools that need user scoping.
- When a tool calls `ecommerce-service`, it forwards the original JWT. `ecommerce-service` enforces authorization — `ai-service` never makes its own authz decisions.
- Catalog-only tools (`search_products`, `get_product`, `check_inventory`) are callable unauthenticated.

## 3. The Agent Loop

### 3.1 Shape

```go
// internal/agent/agent.go
type Agent struct {
    llm      llm.Client
    registry tools.Registry
    maxSteps int           // hard cap, default 8
    timeout  time.Duration // per-turn wall clock, default 30s
}

type Turn struct {
    UserID   string
    Messages []llm.Message
}

func (a *Agent) Run(ctx context.Context, turn Turn, emit func(Event)) error
```

`emit` is how the handler streams events to the browser (SSE). The agent doesn't know it's SSE — it just emits typed events. Tests pass a closure that appends to a slice.

### 3.2 Loop (pseudocode)

```
ctx, cancel := context.WithTimeout(ctx, a.timeout)
defer cancel()

messages := turn.Messages
for step := 0; step < a.maxSteps; step++ {
    resp, err := a.llm.Chat(ctx, messages, a.registry.Schemas())
    if err != nil { emit(ErrorEvent); return err }

    if len(resp.ToolCalls) == 0 {
        emit(FinalEvent{Text: resp.Content})
        return nil
    }

    messages = append(messages, resp.AssistantMessage())

    for _, call := range resp.ToolCalls {
        emit(ToolCallEvent{Name: call.Name, Args: call.Args})

        tool, ok := a.registry.Get(call.Name)
        if !ok {
            messages = append(messages, llm.ToolResult(call.ID, toolError("unknown tool: "+call.Name)))
            continue
        }

        result, err := tool.Call(ctx, call.Args, turn.UserID)
        if err != nil {
            emit(ToolErrorEvent{Name: call.Name, Err: err})
            messages = append(messages, llm.ToolResult(call.ID, toolError(err.Error())))
            continue
        }

        emit(ToolResultEvent{Name: call.Name, Result: result})
        messages = append(messages, llm.ToolResult(call.ID, result.Content))
    }
}

emit(ErrorEvent{Reason: "max steps exceeded"})
return ErrMaxSteps
```

### 3.3 Design decisions baked into the loop

1. **Tool errors become tool results, not loop errors.** Only infrastructure failures (LLM unreachable, context cancelled) bubble up. This is how real agents stay useful when tools flake.
2. **Hard step cap + wall-clock timeout.** Two independent bounds. Defaults: 8 steps, 30s.
3. **Parallel tool calls in one step are executed sequentially in v1.** Simpler, easier to reason about. Concurrency added only if evals show it matters.
4. **The registry is passed in, not global.** Tests build an `Agent` with a fake registry and a fake `llm.Client` and run the loop with no network.
5. **`emit` is a function, not a channel.** One less piece of concurrency.
6. **User ID is an explicit parameter on `Tool.Call`, not a context value.** Reviewers instantly see which tools are user-scoped.

### 3.4 Explicitly handled failure modes

- Malformed tool args → treated as tool error, fed back to the model, it usually self-corrects.
- Hallucinated tool name → synthetic error result, model recovers.
- Tool panic → `defer recover()` in the tool-call wrapper, becomes a tool error.
- Context cancelled mid-step → loop exits, partial events already emitted, handler closes SSE cleanly.

### 3.5 Deliberately *not* in the loop

- No reflection / "think before you act" prompting. Qwen 2.5 with tool calling doesn't need it and it doubles latency.
- No conversation summarization/compaction. Add if evals show context bloat.
- No tool-call deduplication. Redis response cache makes repeats cheap.
- No streaming of intermediate LLM text — only structured events and final-answer streaming.

## 4. Tool Registry & Catalog

### 4.1 Interface

```go
// internal/tools/registry.go
type Tool interface {
    Name() string
    Description() string
    Schema() json.RawMessage
    Call(ctx context.Context, args json.RawMessage, userID string) (Result, error)
}

type Result struct {
    Content any // JSON-serializable; what the LLM sees
    Display any // optional richer payload for the frontend (e.g. product cards)
}

type Registry interface {
    Register(Tool)
    Get(name string) (Tool, bool)
    Schemas() []ToolSchema // sent to Ollama on every turn
}
```

`Result.Content` is what the agent loop feeds back into the conversation (compact, token-efficient). `Result.Display` is what the handler emits on the SSE stream for rich frontend rendering.

**MCP path (future, not v1):** an `mcp.ClientTool` struct implements `Tool` by proxying `Call` to an MCP server over stdio/HTTP. `Schema()` comes from the MCP `tools/list` response. No other code changes.

### 4.2 v1 tool catalog

**`catalog.go` — unauthenticated, read-only:**

| Tool | Args | Calls |
|---|---|---|
| `search_products` | `query: string, limit?: int, max_price?: number` | Qdrant (semantic) → ecommerce-service for hydration |
| `get_product` | `product_id: string` | ecommerce-service `GET /products/{id}` |
| `check_inventory` | `product_id: string` | ecommerce-service `GET /products/{id}/inventory` |

**`orders.go` — authenticated, scoped to `userID`:**

| Tool | Args | Calls |
|---|---|---|
| `list_orders` | `limit?: int, status?: string` | ecommerce-service `GET /orders?user_id=...` |
| `get_order` | `order_id: string` | ecommerce-service `GET /orders/{id}` |
| `summarize_orders` | `period?: "week" \| "month" \| "all"` | `list_orders` + LLM summarization sub-call |
| `initiate_return` | `order_id: string, item_ids: string[], reason: string` | ecommerce-service `POST /orders/{id}/returns` |

**`cart.go` — authenticated:**

| Tool | Args | Calls |
|---|---|---|
| `add_to_cart` | `product_id: string, qty: int` | ecommerce-service `POST /cart/items` |
| `view_cart` | — | ecommerce-service `GET /cart` |

**`summarize_orders` note:** this tool does a sub-LLM call (agent → tool → LLM). It's the one tool that can blow latency and cost. Kept in v1 because the demo value is high; bounded by reusing the same wall-clock timeout and by capping input to the last 20 orders.

### 4.3 New ecommerce-service endpoints required

These are in scope for the implementation plan:
- `GET /products/{id}/inventory` (may exist; confirm during implementation)
- `POST /orders/{id}/returns`
- Any shape changes to `GET /orders` needed to support `status` filtering and `limit`.

### 4.4 Tool design rules

1. Tools are thin. >40-line bodies should be split or pushed into a helper.
2. Tools never bypass authorization. Every user-scoped tool forwards the JWT.
3. Tool args are validated twice: JSON Schema via the tool-calling contract, and Go struct unmarshaling with explicit required-field checks.
4. Tool results are bounded. `search_products` ≤ 10 results, `list_orders` ≤ 20, `summarize_orders` returns a short string.
5. Every tool has at least one unit test and one eval case.

## 5. Operations

### 5.1 Evals

Located at `go/ai-service/internal/evals/`. Run via `go test -tags=eval ./internal/evals/...`. Build tag keeps regular `go test ./...` fast and offline.

```go
type EvalCase struct {
    Name         string
    Prompt       string
    MustCallTool string
    MustNotCall  []string
    AssertResult func(t, finalText, toolTrace)
}
```

Starter case set (~15, grows with the service):
- **Tool selection:** "find me a waterproof jacket under $150" → must call `search_products` with `max_price=150`.
- **User scoping:** "where's my last order" → must call `list_orders` or `get_order` with current `userID`.
- **Negative:** "summarize everyone's orders" → must refuse or scope to current user.
- **Recovery:** tool wired to return a synthetic 500 on first call → agent must recover gracefully; final text must acknowledge the failure.
- **Bounded output:** seeded user with 50 orders → `list_orders` must be called with `limit ≤ 20`.

**Two eval tiers:**
- **Mocked-LLM tier** runs on every PR via a fake `llm.Client` that returns canned tool-call sequences. Fast, deterministic. Asserts wiring: the loop dispatches, recovers from errors, respects caps, emits correct events.
- **Real-LLM tier** runs nightly against Ollama on the Windows PC via the existing SSH path. Asserts structural properties (which tool, with what arg shape) rather than exact wording. A soft suite also checks final-answer embedding similarity to golden answers with a loose threshold; soft failures warn, don't block.

### 5.2 Caching (Redis, existing `go-ecommerce` Redis)

- **Embedding cache:** `ai:embed:<sha256(text)>` → vector bytes. TTL 30 days.
- **Tool response cache:** `ai:tool:<name>:<sha256(canonical_args+userID)>` → JSON result. TTL 60s for catalog tools, 10s for order tools, disabled for writes (`add_to_cart`, `initiate_return`).

Cache is opt-in per tool via an option on the `Tool` definition. No blanket wrapper.

### 5.3 Observability

New `/metrics` exposes:
- `ai_agent_turns_total{outcome="final"|"max_steps"|"error"|"timeout"|"refused"}`
- `ai_agent_steps_per_turn_bucket`
- `ai_agent_turn_duration_seconds_bucket`
- `ai_tool_calls_total{name, outcome}`
- `ai_tool_duration_seconds_bucket{name}`
- `ai_llm_tokens_total{direction="prompt"|"completion"}`
- `ai_cache_events_total{cache="embed"|"tool", event="hit"|"miss"}`

**Grafana:** one new row on the existing `system-overview` dashboard titled "AI Service" — turn rate, p95 turn latency, tool call rate by tool, cache hit rate, token throughput. Deliberately on the existing dashboard, not a separate one, to match the "AI as a system citizen" framing.

**Structured logs:** one JSON line per turn with `turn_id`, `user_id`, `steps`, `tools_called`, `duration_ms`, `outcome`. One line per turn, not per step.

### 5.4 Guardrails

- Input message > 4000 chars → HTTP 400.
- Conversation history truncated to last 20 messages (keep system prompt, oldest-first drop on history).
- Per-IP rate limit: token bucket, ~20 turns/minute, Redis-backed.
- PII scrub on logs: email/phone regex before write.
- Refusal detection: if final text matches known refusal prefixes AND no tools were called, outcome tagged `refused` in metrics.
- Hallucinated tool results (model emits a tool result without having called one) are ignored.

**Out of scope:** jailbreak detection, prompt-injection scanning, content moderation, dollar-cost tracking, prompt A/B testing. These are full projects on their own.

## 6. HTTP Surface, Frontend, Deployment, CI, Docs

### 6.1 HTTP surface

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/chat` | JWT (optional for catalog-only) | Start/continue a turn. SSE response. |
| `GET` | `/health` | none | Liveness. |
| `GET` | `/ready` | none | Readiness — checks Ollama, Qdrant, Redis reachable. |
| `GET` | `/metrics` | cluster-internal | Prometheus. |

**`POST /chat` request:**
```json
{
  "messages": [{"role": "user", "content": "..."}],
  "session_id": "optional-client-generated-uuid"
}
```

**SSE event types:**
- `tool_call` — `{name, args}`
- `tool_result` — `{name, display}`
- `tool_error` — `{name, error}`
- `final` — `{text}`, streamed token-by-token
- `error` — `{reason}`

Session state is client-held. The frontend sends the full message array on every turn. No server-side sessions in v1.

### 6.2 Frontend integration

New component: `frontend/src/components/go/AiAssistantDrawer.tsx`. Floating button on `/go/ecommerce/*` opens a right-side shadcn `Sheet`: message list, tool-call traces as collapsible cards, streaming final answer, input box.

**Key UX choices:**
- **Tool-call traces are visible by default**, not hidden behind a debug toggle. `🔧 search_products({query: "waterproof jacket", max_price: 150})` followed by rendered product cards is the single highest-leverage "this is an agent, not a chatbot" signal in the portfolio.
- Product-card results are interactive. Clicks navigate to the product page; add-to-cart goes through the normal cart flow.
- Drawer reads the existing JWT from the `/go/ecommerce` auth context.
- New env var `NEXT_PUBLIC_AI_SERVICE_URL` added to Vercel production **before merge** (per the CLAUDE.md localhost-fallback warning).

**Playwright E2E (mocked):** opens drawer, sends "find me a waterproof jacket under $150", asserts a `tool_call` event for `search_products` with `max_price=150`, asserts a product card renders, asserts a final answer streams. Runs in the existing staging mocked E2E suite.

### 6.3 Deployment

- `go/ai-service/Dockerfile` — multi-stage, matching `auth-service`/`ecommerce-service`.
- K8s manifests under `go/k8s/`: deployment, service, configmap. Deployed to `go-ecommerce` namespace.
- No migration Job (no schema).
- Ingress: new path `/ai-api/*` rewritten to `ai-service:8080` on the Minikube ingress.
- Cloudflare Tunnel: `https://api.kylebradshaw.dev/ai-api/*` → Minikube ingress. Existing wildcard rule covers it.
- Ollama: `ollama.ai-services.svc.cluster.local`.
- Qdrant: `qdrant.ai-services.svc.cluster.local`.
- Redis: existing `go-ecommerce` Redis, `ai:*` key prefix.

### 6.4 CI

Additions to `.github/workflows/go-ci.yml`:
- Lint + unit tests for `go/ai-service` (mocked, every push).
- Mocked-LLM eval suite (`go test -tags=eval ./go/ai-service/internal/evals/...`) on every push.
- Real-LLM eval suite nightly via scheduled workflow over SSH to the Windows PC.
- Image build + push to GHCR on main; deployment via the existing SSH/compose pipeline extended with `ai-service`.

New Make target: `preflight-ai-service` — lint + unit + mocked evals locally. Added to `make preflight-go` and `make preflight`.

### 6.5 Documentation

- `docs/adr/go-ai-service/01-agent-harness-in-go.ipynb` — ADR notebook covering why Go, why Ollama tool calling, why the registry shape, and Debug Assistant (Python) vs ai-service (Go) comparison. This notebook is the interview artifact.
- `docs/adr/rag-reevaluation-2026-04.md` — standalone ADR recording the "RAG is commodity, agents are the frontier, pivot to Go" decision. Worth its own file because a reviewer who sees the author change their mind for stated reasons trusts them more than one who sees a straight-line plan.
- Update `CLAUDE.md` project structure to list `go/ai-service/`.

## 7. Open Questions (resolved during implementation)

- Exact Ollama model for the agent: Qwen 2.5 14B is the default (already in the stack). Confirm tool-calling reliability during the implementation spike; fall back to a smaller tool-call-specialized model if needed.
- Whether `check_inventory` needs a new ecommerce-service endpoint or can reuse existing product fields.
- Whether `summarize_orders` sub-LLM call should share the parent turn's wall-clock budget or have its own.
- Exact auth mechanism for JWT verification in `ai-service`: shared secret vs. JWKS endpoint on auth-service.

## 8. Success Criteria

- A reviewer landing on `/go/ecommerce` can open the assistant drawer and, within 30 seconds, see a visible tool call, a visible tool result rendered as a product card, and a streamed final answer.
- `go test ./go/ai-service/...` passes offline.
- Mocked-LLM eval tier passes on every PR.
- Real-LLM eval tier runs nightly and its pass rate is tracked.
- Grafana `system-overview` dashboard has an "AI Service" row showing live traffic when the drawer is used.
- The ADR notebook explains the agent loop well enough that a Go engineer with no prior agent experience understands it on first read.
- The `rag-reevaluation-2026-04.md` ADR is committed and honest about why the roadmap pivoted.
