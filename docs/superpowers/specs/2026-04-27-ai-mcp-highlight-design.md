# Design: Highlight the MCP Server on the `/ai` Portfolio Page

- **Date:** 2026-04-27
- **Status:** Draft — pending implementation
- **Builds on:**
  - `go/ai-service/internal/mcp/` — existing MCP server using `modelcontextprotocol/go-sdk`
  - `frontend/src/components/go/tabs/AiAssistantTab.tsx` — existing tool catalog diagram
  - `frontend/src/app/ai/page.tsx` — current `/ai` portfolio landing page
- **Related GitHub issues:**
  - Close on completion: #79 (RAG eval harness — already shipped under `services/eval/`), #83 (eval UI — already shipped at `/ai/eval`)
  - Future RAG section work: #80 (hybrid search), #81 (cross-encoder re-ranking)
  - Future eval section work: #84 (compare/history endpoints), #85 (improvement-tracking dashboard)

## Context

The Go ai-service exposes 12 tools (8 ecommerce + 3 RAG + 1 returns) through two consumers of a single tool registry:

1. The **in-app agent loop** in `internal/agent/` — drives the shopping-assistant chat and `/ai/rag` chat. Calls Ollama with tool schemas, dispatches tools, streams SSE.
2. The **MCP server** in `internal/mcp/` — exposes the same registry to external MCP clients over HTTP Streamable transport at `/mcp`, with optional Bearer-JWT auth.

Today the portfolio describes only consumer (1). The MCP-server story shows up briefly inside the Go section's "AI Assistant" tab but isn't framed as MCP, and the dedicated `/ai` page makes no mention of MCP at all. That is the most interview-relevant artifact in the AI section — an official-SDK MCP server that any MCP client can connect to — and it is currently invisible to anyone scanning the AI page.

This spec adds an MCP-Server section at the top of `/ai`, reorders the existing sections, and gives visitors enough material to connect their own MCP client (Claude Desktop, Codex CLI, MCP Inspector) to the running server.

## Goals

- Lead the `/ai` page with the MCP server — the strongest artifact for a Gen AI Engineer audience.
- Give visitors a concrete path to actually use the server from their own AI client (Claude Desktop, Codex, Inspector).
- Reuse the existing tool-catalog diagram across `/ai` and `/go` rather than maintaining two copies.
- Keep the MCP framing honest: an external MCP client does NOT trigger the in-app Go agent loop — it runs its own loop. Diagrams must not conflate the two.
- Move RAG Evaluation up to the second slot, since it pairs naturally with the MCP framing.
- Close issues that are already done (#79, #83) so the open-issue list reflects reality.

## Non-goals

- No new interactive demo on `/ai`. The Shopping Assistant on `/go` already exercises the same toolchain end-to-end; we link to it rather than duplicate it.
- No backend changes to the MCP server. Auth, transport, and the tool set stay as they are.
- No work on issues #80, #81, #84, #85. They land in the RAG / Eval sections in future cycles, not in this scope.
- No reframing of the Document Q&A or Debug Assistant sections beyond reordering. Their copy stays as-is.

## Page structure

Final ordering of sections on `frontend/src/app/ai/page.tsx`:

| # | Section | Status |
|---|---|---|
| 1 | **MCP Server** | NEW (top) |
| 2 | RAG Evaluation | moved up from position 3 |
| 3 | Document Q&A Assistant | unchanged content; new position 3 |
| 4 | Debug Assistant | unchanged content; new position 4 |

The Bio paragraph at the top of the page is updated to mention the MCP-server framing in one sentence. The existing Grafana-dashboard link stays.

## Section 1: MCP Server — content breakdown

### 1a. What & why (1 short paragraph)

Describe the MCP server in plain terms: built on the official `modelcontextprotocol/go-sdk`, exposes 12 tools across ecommerce and RAG domains, runs over HTTP Streamable transport, optional Bearer-JWT auth (public tools work anonymously, scoped tools require a token). Mention OTel trace propagation and the Go→Python bridge (with circuit breaker) for the RAG tools, since those are the production-quality details that distinguish this from a tutorial-grade server.

### 1b. Architecture diagram (NEW)

A new Mermaid diagram showing the **external MCP client path** (distinct from the in-app agent path). High-level shape:

```
External MCP client (Claude Desktop / Codex / Inspector)
   │  HTTPS Streamable transport, optional Bearer JWT
   ▼
ai-service /mcp endpoint  (Go, modelcontextprotocol/go-sdk)
   │
   ▼
Tool registry (12 tools)
   │
   ├── Ecommerce backend  (REST/gRPC, OTel-propagated)
   │
   └── Python RAG bridge  (HTTP, circuit breaker, OTel)
           │
           ▼
       Qdrant + Ollama
```

The diagram must visibly differ from the in-app agent diagrams in `AiAssistantTab.tsx` so a reader doesn't confuse the two paths. No "Ollama" box on the MCP path — the external client owns its own LLM.

### 1c. Tool catalog (REUSED)

Extract the existing `flowchart LR` tool-catalog diagram from `AiAssistantTab.tsx` into a shared component:

- New file: `frontend/src/components/ai/MCPToolCatalog.tsx`
- Default export: a component that renders the existing 12-tool catalog Mermaid chart wrapped in `<MermaidDiagram />`.
- Replace the inline chart string in `AiAssistantTab.tsx` with `<MCPToolCatalog />`.
- Render the same component in the new MCP section on `/ai`.

The component takes no props. If we later want to vary the framing copy around the diagram between the two locations, we can add a prop, but YAGNI for now.

### 1d. "Try it interactively" link

A single CTA button that links to `/go` (Shopping Assistant tab). Copy: *"The same tools power the in-app shopping assistant — try it on the Go section."* This is the equivalent of the "Try the Demo →" CTAs on the other AI sections, but instead of a new demo it points to the existing one.

### 1e. Connect your own client

A new subsection with three copy-pasteable configuration snippets, each in a `<pre>` block with a code copy button (use whatever pattern already exists in the repo for code blocks; if none exists, plain `<pre>` is fine — no scope creep on copy-button infrastructure).

The public MCP endpoint is `https://api.kylebradshaw.dev/go-ai/mcp` (verify the exact ingress path during implementation — check `k8s/go-ecommerce/` ingress rules for ai-service before publishing the snippet).

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "kyle-portfolio": {
      "transport": "http",
      "url": "https://api.kylebradshaw.dev/go-ai/mcp"
    }
  }
}
```

**Codex CLI** — config block matching the format Codex expects for HTTP MCP servers (verify against current Codex docs at implementation time; if the format has shifted, update the snippet rather than ship something stale).

**MCP Inspector** — single command:

```bash
npx @modelcontextprotocol/inspector https://api.kylebradshaw.dev/go-ai/mcp
```

A short note below the snippets: *"Public tools (catalog search, RAG search, `list_collections`) work without auth. Scoped tools (cart, orders, returns) require a Bearer JWT — register and log in at /go/register, then copy the token from the auth header in DevTools."*

### 1f. GitHub link

A small footer link to `go/ai-service/internal/mcp/` on GitHub, the way other portfolio sections do.

## Component changes

| File | Change |
|---|---|
| `frontend/src/app/ai/page.tsx` | Add Section 1 (MCP Server), reorder existing sections, update bio paragraph |
| `frontend/src/components/ai/MCPToolCatalog.tsx` | NEW — extracted shared component for the 12-tool diagram |
| `frontend/src/components/go/tabs/AiAssistantTab.tsx` | Replace inline tool-catalog chart string with `<MCPToolCatalog />` import |
| (no backend changes) | — |

## Issue cleanup as part of this scope

- **Close #79** (Phase 4a: RAG evaluation harness) with a comment linking to `services/eval/` and `/ai/eval`.
- **Close #83** (Eval Service UI) with a comment linking to `frontend/src/app/ai/eval/page.tsx`.
- Leave #80, #81 (RAG retrieval quality) and #84, #85 (eval comparison/dashboard) open. They belong to the RAG and Eval sections respectively — out of scope for this MCP highlight.

## Out of scope (explicit non-goals worth re-stating)

- No interactive MCP-tool runner UI on `/ai`.
- No work on hybrid search, re-ranking, eval comparison endpoints, or eval dashboard.
- No changes to the MCP server's auth model, transport, or tool registry.
- No reframing of `/ai/rag`, `/ai/debug`, or `/ai/eval` pages.

## Verification

- `make preflight-frontend` (tsc + Next build + lint) passes.
- `/ai` renders all four sections in the new order, with the new MCP section at the top.
- The shared `<MCPToolCatalog />` component renders identically on both `/ai` and `/go` (Shopping Assistant tab).
- The Claude Desktop and Inspector snippets are syntactically valid (paste-test against a local Claude Desktop and a fresh Inspector run before merging).
- Manually verify the public ingress path resolves: `curl -s https://api.kylebradshaw.dev/go-ai/mcp` returns a non-error response (the MCP handshake will reject a plain GET, but the server should respond — a 404 or transport error means the ingress path is wrong).
- Issues #79 and #83 are closed with reference comments.

## Open questions

- **Exact public path for the MCP endpoint.** I've assumed `https://api.kylebradshaw.dev/go-ai/mcp` based on the documented ingress routing pattern (`/go-api/*`, `/go-auth/*`, `/go-products/*`). The actual ai-service ingress path needs to be confirmed before the connection snippets ship — the wrong path in a portfolio JSON config block is a credibility hit.
- **Should the connection-instructions section include a "minimal JWT for testing" path?** Right now visitors would need to register and copy a token from DevTools to exercise scoped tools. A more polished story would be a "guest token" link that mints a read-only JWT scoped to the demo user. Out of scope for this round; flagged for future iteration.
