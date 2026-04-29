# MCP Depth: Resources, Prompts, and Composite Tools

- **Date:** 2026-04-29
- **Status:** Accepted

## Context

The first `go/ai-service` MCP surface exposed the same low-level ecommerce tools used by the web chat. That was useful for tool-calling demos, but shallow for MCP clients: the model had to infer which tools to chain, had no stable read-only context URIs, and had no server-provided prompt templates for common workflows.

The portfolio goal is to demonstrate production-grade Go engineering around an AI service, not only that an LLM can call CRUD endpoints. The backend therefore needs protocol depth:

- Composite tools that perform multi-source fan-out and return verdict-shaped output.
- MCP Resources for stable, read-only context such as catalog summaries, authenticated user state, architecture runbooks, and sanitized schema notes.
- MCP Prompts for repeatable workflows clients can surface as suggestions or slash commands.

This has to fit the existing architecture: the agent loop still depends on `tools.Registry`, auth remains JWT-scoped, and the MCP SDK should not leak through business logic packages.

## Decision

`go/ai-service` now exposes three composite tools:

- `investigate_my_order`, which correlates order state, saga state, payment state, cart reservation state, traces, and logs into a customer-facing verdict.
- `compare_products`, which compares product structure and optional semantic similarity.
- `recommend_with_rationale`, which turns user history signals into recommendation rationales.

MCP Resources are implemented behind `internal/mcp/resources.Registry`. Resource implementations are SDK-independent and expose a small interface: URI, name, description, MIME type, and `Read(ctx)`. The server adapter translates those resources into the pinned `modelcontextprotocol/go-sdk` types at the boundary.

`user://` resources read the authenticated user from `jwtctx.UserID(ctx)`. Anonymous reads return `ErrResourceNotFound`, which avoids accidentally leaking whether another user's data exists. HTTP MCP requests populate both the MCP context user id and the shared JWT context. Stdio mode applies `AI_SERVICE_TOKEN` defaults to resource reads when configured.

`catalog://product/{id}` is represented as an MCP resource template rather than enumerating every product in `resources/list`. The registry holds an optional catalog client and dispatches matching reads dynamically.

Server-provided prompts are implemented behind `internal/mcp/prompts.Registry`, also independent from the SDK. The adapter maps prompt arguments and rendered messages into MCP `Prompt` and `GetPromptResult` values.

The Docker image now includes `go/ai-service/resources/` at `/app/resources/`, and K8s config supplies the resource file paths plus observability and product-service URLs.

## Consequences

MCP clients now see a richer protocol surface: tools for high-level workflows, resources for stable context, and prompts for repeatable user tasks.

The registries keep business logic independent from the MCP SDK, so SDK version churn is limited to adapter files under `internal/mcp`.

The user resource guard is intentionally conservative. Returning not-found for anonymous reads hides both data and existence. The trade-off is that clients must provide a valid token before user resources become useful.

Composite tools add operational dependencies on service databases and observability endpoints. Startup pings are warn-only so the broader AI service can still start, and per-call results can degrade with partial evidence rather than failing the whole MCP surface.

Prometheus counters and OpenTelemetry spans now cover resource reads, prompt renders, and composite tool latency, giving the same visibility expected from regular HTTP and agent workflows.
