# ai-service depth — Phase 1 design

**Date:** 2026-04-28
**Status:** Draft, pending user review
**Phase:** 1 of 2 (Phase 2 covers a separate ops/SRE MCP server)

## Context

`go/ai-service/` is already a real Model Context Protocol server. It uses the official `modelcontextprotocol/go-sdk`, registers 12 tools (9 ecommerce + 3 RAG) through `internal/mcp/server.go`, supports both stdio and HTTP transports, and ships a JWT-auth middleware for HTTP mode. It is the AI surface behind the chat experiences on `/ai/*` and the storefront on `/go/ecommerce`.

The concern raised in a prior session is that the current implementation is a **shallow MCP demonstration**: each tool is a thin wrapper over an HTTP endpoint, the LLM does all of the reasoning, and the protocol features that distinguish MCP from "an HTTP API the LLM can call" are absent — no Resources, no server-provided Prompts, no Sampling, no approval-gated writes, no composite tools that encode runbook-level domain knowledge.

This phase deepens ai-service along five axes so the live, visible portfolio surface demonstrates protocol fluency *and* operational depth, not just tool count.

A second MCP server focused on ops/SRE workflows (Loki/Prometheus/Jaeger/Postgres against the live cluster, demoed through Claude Code over Tailscale) is intentionally out of scope for this phase and tracked as Phase 2. Patterns developed here become templates for that server.

## Goals

- Make the *visible* AI surface non-shallow: every depth axis lands in something a portfolio visitor can touch within a minute on `kylebradshaw.dev`.
- Demonstrate the rare MCP features (Resources, server-provided Prompts, Sampling, approval gates) that almost no public demo implements.
- Encode at least three runbook-level investigations as composite tools so domain knowledge of *this* system is visible in the chat output, not just narrated by the LLM.
- Reuse existing patterns: `go/pkg/apperror`, `go/pkg/resilience`, `go/pkg/tracing`, the existing tool registry contract.

## Non-goals

- Ops/SRE workflows (Loki/Prom/Jaeger queries, k8s introspection, mTLS diagnosis, Kafka lag triage). Phase 2.
- A new top-level service. All work lives inside `go/ai-service/`.
- A parallel "demo user" abstraction with nightly reseeding. The existing `/go/ecommerce` flow already provides a real registration path; users who want personalized depth use a real account.
- Public exposure of write operations without approval gates.

## Identity and audience

- **Logged-in visitors** (registered via the existing `/go/ecommerce` flow) carry a real JWT. User-scoped tools (`investigate_my_order`, `recommend_with_rationale`) and user-scoped Resources (`user://orders`, `user://cart`) are registered for these sessions and operate on real data.
- **Anonymous visitors** get the chat without auth. User-scoped tools and Resources are not registered for that session; the chat retains the catalog tools, the portfolio Resources, the RAG depth improvements, and the `tell-me-about-this-portfolio` prompt.
- **Stdio MCP clients** (Claude Code, Claude Desktop) connecting locally use the existing `AI_SERVICE_TOKEN` JWT defaults already wired in `internal/mcp/server.go`. Per the user's earlier decision, stdio access is documented in the AI section of the portfolio for visitors who want to connect Claude Code themselves; it is not the primary demo surface.

## Composite tools (the depth)

Three composite tools, registered alongside the existing primitives. Each encodes a multi-source investigation that no single endpoint provides.

### `investigate_my_order(order_id)`

Walks the entire checkout saga for a given order:

1. Order row from `orderdb` (status, totals, line items, saga step).
2. Saga state machine history (events emitted, current state, retry count).
3. Payment outbox row from `paymentdb` (Stripe charge id, webhook receipt status).
4. Cart reservation history from `cartdb` (was reservation released? when?).
5. RabbitMQ message events for the order's correlation id (consumed, dead-lettered, requeued).
6. Stitched Jaeger trace by `trace_id` if present, summarized to span names + durations.
7. Loki log entries from `order-service`, `payment-service`, `cart-service` windowed to the saga timestamps.

Returns a structured verdict:

```json
{
  "stage": "payment_captured | warehouse_pending | failed | completed",
  "status": "ok | retrying | stalled | failed",
  "customer_message": "Your payment cleared but the warehouse hasn't acknowledged yet — we're retrying.",
  "technical_detail": "...",
  "next_action": "wait | contact_support | none",
  "evidence": { "trace_id": "...", "saga_step": "...", "last_log_entries": [...] }
}
```

The same investigation engine will be reused in Phase 2 with operator-framing for the ops MCP. Phase 1 returns customer-facing prose; Phase 2 returns operator-facing verdicts from the same underlying data pull.

### `compare_products(product_ids[])`

Pairwise structured comparison:

1. Fetch each product (name, category, price, attributes, image).
2. Fetch product embeddings from Qdrant (or compute on the fly if not present).
3. Compute pairwise cosine similarity.
4. Diff structured attributes (price, category, stock, key spec fields).
5. If RAG has indexed reviews or product copy, pull a 3-sentence summary per product via the chat service.

Returns:

```json
{
  "products": [{ "id": "...", "summary": "..." }, ...],
  "shared_attributes": { "category": "..." },
  "differing_attributes": [{ "field": "price", "values": {...} }, ...],
  "semantic_similarity": [{ "pair": ["a","b"], "score": 0.82 }, ...],
  "recommendation": "If you care about X, choose A; if Y, choose B."
}
```

### `recommend_with_rationale(user_id, category?)`

Personalized recommendation with explicit reasoning:

1. Pull order history, current cart, and recently viewed products for the user.
2. Build a query embedding from the average of past purchase embeddings (optionally filtered by `category`).
3. Search productdb embeddings for nearest neighbors, excluding products already owned or in cart.
4. For each result, compute the *signal* — which past purchase or cart item drove the match.
5. Rank by similarity score with a small recency boost.

Returns:

```json
{
  "products": [
    {
      "id": "...",
      "name": "...",
      "rationale": "Similar to the running shoes you bought in March; matches your interest in trail gear.",
      "surfaced_signals": ["order:abc:item:xyz", "cart:current:item:def"]
    }
  ],
  "query_embedding_source": "average_of_3_past_purchases"
}
```

The `rationale` and `surfaced_signals` are the demo: visitors see *why* the agent recommended what it did, not just *what*. This is the reasoning trace a normal recommendation API does not expose.

## Resources

MCP Resources are read-only URIs the client can list and fetch without invoking tools. The server registers handlers for `resources/list` and `resources/read`.

| URI | Description | Scope |
|---|---|---|
| `catalog://categories` | List of product categories with counts | Public |
| `catalog://featured` | Curated featured product set | Public |
| `catalog://product/{id}` | Single product detail | Public |
| `user://orders` | Logged-in user's order history | JWT-scoped |
| `user://cart` | Logged-in user's current cart | JWT-scoped |
| `runbook://how-this-portfolio-works` | Markdown architectural narrative | Public |
| `schema://ecommerce` | Sanitized ER summary of ecommerce databases | Public |

`runbook://how-this-portfolio-works` is loaded from `go/ai-service/resources/runbook.md` at startup and exposed as a single text resource. The chat reads it on session start to ground "tell me about this project" style questions without a tool call.

User-scoped Resources resolve the JWT `sub` claim from the request context and 404 for anonymous sessions rather than leaking other users' data.

## Server-provided Prompts

MCP Prompts are parameterized templates the server publishes via `prompts/list` and `prompts/get`. The web chat surfaces them as suggestion chips by calling `prompts/list` on session start.

| Name | Parameters | Behavior |
|---|---|---|
| `explain-my-order` | `order_id` | Wraps `investigate_my_order` with a customer-friendly framing prompt |
| `compare-and-recommend` | `category?` | Chains `recommend_with_rationale` then `compare_products` on the top results |
| `tell-me-about-this-portfolio` | none | Pre-loads `runbook://how-this-portfolio-works` and frames a guided tour |

Each Prompt is a Go struct in `internal/mcp/prompts/` with a render function that produces the MCP `GetPromptResult` shape (a sequence of messages with embedded resource references).

## Sampling

MCP Sampling lets the server request the *client's* LLM to perform a completion on the server's behalf. Two applications.

### RAG chunk summarization

When `rag_search` returns more than 5 chunks, the server uses `ctx.RequestSampling` to ask the client's LLM to produce a structured brief of the chunks before returning. The tool result includes both the brief and a chunk-id index, so the client can request raw chunks if needed.

Eval target: at least 60% reduction in tokens returned from `rag_search` compared with the raw-chunks baseline, with no more than a 10% regression on the existing RAGAS eval set.

### Error humanization

When a downstream tool errors with a structured `apperror` whose `internal` flag is true, the server uses Sampling to translate the raw error into a one-line customer message. The tool result includes both the raw error code (for debugging) and the humanized message (for chat display).

Both Sampling applications live behind a feature flag `MCP_SAMPLING_ENABLED`. If the connecting client does not advertise sampling capability during the MCP handshake, the server falls back to the unmodified behavior rather than failing.

## Approval-gated writes

Two write tools, both following a dry-run / confirm pattern.

### `cancel_order(order_id)`

Returns a dry-run manifest:

```json
{
  "action": "cancel_order",
  "order_id": "...",
  "refund_amount_cents": 4299,
  "items_to_return": [...],
  "estimated_refund_eta": "3-5 business days",
  "confirm_token": "...",
  "expires_at": "..."
}
```

### `update_cart_quantity(item_id, qty)`

Returns the projected cart totals plus a `confirm_token`.

### `confirm_action(confirm_token)`

Looks up the token, validates expiry and ownership, executes the underlying mutation, returns the result.

Tokens are stored in Redis with a 5-minute TTL, keyed by token id, with the action payload as the value. Every confirmation emits a structured audit log entry to Loki with `event=mcp.approval.confirmed`, `user_id`, `action`, `confirm_token_prefix`. Auth-related writes (password change, email change) stay out of this surface — they belong to auth-service.

## Architectural placement

All work lives in `go/ai-service/`. New packages:

```
internal/
├── mcp/
│   ├── server.go              (existing, extended with resources/prompts handlers)
│   ├── resources/
│   │   ├── catalog.go
│   │   ├── user.go
│   │   ├── runbook.go
│   │   └── schema.go
│   ├── prompts/
│   │   ├── explain_order.go
│   │   ├── compare_recommend.go
│   │   └── portfolio_tour.go
│   ├── sampling/
│   │   ├── client.go          (wraps ctx.RequestSampling, capability check)
│   │   ├── rag_summary.go
│   │   └── error_humanize.go
│   └── approval/
│       ├── store.go           (Redis-backed token store)
│       ├── handler.go         (confirm_action tool)
│       └── audit.go
├── tools/
│   ├── (existing primitives unchanged)
│   └── composite/
│       ├── investigate_order.go
│       ├── compare_products.go
│       └── recommend_rationale.go
resources/
└── runbook.md                 (text content for runbook:// resource)
```

Composite tools live in `internal/tools/composite/` to make the depth-tools visually separate from the primitives. They register through the same `tools.Registry` so the existing MCP server adapter picks them up automatically.

## Frontend changes

- The chat surface lives as a slide-in panel on `/go/ecommerce` and its child routes (`/go/ecommerce/cart`, `/go/ecommerce/orders`, `/go/ecommerce/[productId]`). The panel is route-aware: it pre-fetches `catalog://product/{id}` when on a product page, `user://cart` when on cart, etc., so the chat is contextual.
- `/ai/rag` stays as-is — it remains the document-QA demo for the existing notebooks/PDFs, not the ecommerce surface.
- Suggestion chips render server-provided Prompts dynamically by calling `prompts/list` on session start.
- Tool-call cards extend to render approval prompts: when a tool returns a `confirm_token`, the card shows the dry-run manifest with a "Confirm" button that calls `confirm_action`.
- A small "Resources used" expandable section shows which Resources the chat read during the session, so visitors can see the protocol surface in action.

## Auth surface

- HTTP transport keeps the existing JWT middleware. The middleware now also extracts the `sub` claim into request context for user-scoped Resources.
- Stdio transport keeps the existing `AI_SERVICE_TOKEN` defaults so Claude Code can connect locally.
- New environment variables:
  - `MCP_SAMPLING_ENABLED` (default `true` if client capability present)
  - `MCP_APPROVAL_TOKEN_TTL` (default `300s`)
  - `MCP_RESOURCES_RUNBOOK_PATH` (default `/app/resources/runbook.md`)
- All new env vars get added to ConfigMaps; no secrets are introduced.

## Observability

Every new package follows the existing `go/pkg/tracing` pattern: parent spans for tool/resource/prompt invocations, child spans for downstream calls. Prometheus counters:

- `mcp_resources_read_total{uri,result}`
- `mcp_prompts_get_total{name}`
- `mcp_sampling_requests_total{purpose,result}`
- `mcp_approval_tokens_total{action,event}` where event is `issued|confirmed|expired|invalid`
- `mcp_composite_tool_duration_seconds{tool}` histogram

Existing Loki logs gain structured fields for the new events. Existing Jaeger trace propagation is preserved end-to-end.

## Testing and evals

- Unit tests for each new package using the existing patterns (`apperror.ErrorHandler()`, `resilience.NewBreaker(...)`, `tracetest.NewInMemoryExporter()`).
- Composite tools each get a golden-path integration test against seeded fixtures.
- One eval case per composite tool added to `internal/evals/cases_test.go`, asserting the structured verdict shape and a substring of the customer message.
- Sampling RAG-summary eval: compare token-count and RAGAS scores against the unmodified baseline.
- Approval flow test: dry-run returns token; confirm without token fails; confirm with expired token fails; confirm with valid token executes and emits audit log.

## Effort estimate

Calendar time at part-time pace, with a phased ship:

- **v1.0 (Bar 2.5)** — Composite tools, Resources, Prompts, frontend integration. ~2.5 weeks.
- **v1.1 (Bar 3 complete)** — Sampling, approval gates, audit log, eval comparisons. ~1.5 weeks.

Total ~4 weeks. v1.0 ships visibly to the live site as soon as it's ready; v1.1 builds on top without a re-deploy break.

## Risks

- **Sampling client support.** Sampling is a relatively new MCP capability. The web chat client must be updated to advertise and handle sampling, or sampling has to be skipped client-side. The feature flag and capability check protect against shipping a regression.
- **Composite tool latency.** `investigate_my_order` touches 5+ datasources. Target: P95 under 2s, achieved through parallel fan-out (errgroup) and circuit breakers. If a downstream is unavailable, the verdict is returned with a `partial_evidence` flag rather than failing.
- **Resource staleness.** `runbook://how-this-portfolio-works` is loaded from disk. Document the requirement to redeploy ai-service when the runbook is updated, or watch the file with fsnotify (out of scope for v1.0).
- **JWT scope leak.** User-scoped Resources must validate the `sub` claim. Add an integration test that confirms anonymous and cross-user access return 404, not the wrong user's data.

## Open follow-ups for Phase 2

- Operator-facing framing of `investigate_my_order` (returns operator verdict instead of customer message).
- `diagnose_grpc_mtls(client, server)` encoding the runbook in CLAUDE.md.
- `diagnose_consumer_lag(group)` for Kafka analytics-service.
- Phase 2 server reuses the Sampling, approval, and Resource patterns established here.
