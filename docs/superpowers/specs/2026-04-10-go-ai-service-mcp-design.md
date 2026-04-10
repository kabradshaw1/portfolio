# Go AI-Service MCP Adapter — Design Spec

**Date:** 2026-04-10
**Status:** Design — ready for implementation plan
**Roadmap ref:** v2 item #1 from `docs/superpowers/specs/2026-04-09-go-ai-service-v2-roadmap.md`

---

## Context

The `go/ai-service` v1 shipped a working agent loop with 9 tools, a `tools.Tool` interface explicitly designed for future MCP adaptation, and an `llm.Client` abstraction that already supports Ollama, OpenAI, and Anthropic backends. The comment in `registry.go` — *"Tool is the only interface a future MCP adapter needs to satisfy"* — is now being fulfilled.

MCP (Model Context Protocol) is the emerging standard for tool interop between AI agents and tool providers. Adding MCP support turns the portfolio pitch from "Go agent with typed tool dispatch" into "Go agent with a transport-agnostic tool registry that speaks MCP — usable from Claude Desktop, Cursor, or any MCP host."

---

## Approach

**Embedded MCP server + thin client adapter.** The MCP server runs inside the existing ai-service binary as an additional transport. Same process, same tool registry — the 9 tools are registered once and served through both the current `/chat` SSE endpoint and MCP (stdio + streamable HTTP). The MCP client wraps tools discovered from external MCP servers and plugs them into the same registry.

**Why not a separate binary:** One registry, one deploy, one test surface. The "transport-agnostic" story is literal when it's the same binary.

**Why not MCP-as-middleware:** Adding a JSON-RPC round-trip to every local tool call is over-engineered for 9 tools in the same process.

---

## SDK

**`github.com/modelcontextprotocol/go-sdk`** (official MCP SDK, v1.5.0+)

- Semver stability guarantees (v1.x)
- Maintained by the MCP org + Google
- GitHub's own MCP Server migrated from `mark3labs/mcp-go` to this SDK
- Single `mcp` package, struct-tag-based tool schemas, idiomatic Go
- Supports stdio, SSE, and streamable HTTP transports for both client and server

---

## Architecture

```
                         ┌─────────────────────────────────┐
                         │         ai-service binary        │
                         │                                  │
  Claude Desktop ──stdio─┤  MCP Server                     │
  Cursor ─────────stdio─┤  (adapts tools.Registry → MCP)  │
  Remote host ──HTTP────┤  transports: stdio, streamable   │
                         │                                  │
                         │  Agent Loop                      │
  Frontend ───SSE /chat──┤  ├── tools.Registry (9 tools)   │
                         │  └── MCPClientTool (MCP proxy)   │
                         │                                  │
                         │  MCP Client                      │
                         │  (discovers & calls MCP servers) │
                         └─────────────────────────────────┘
```

**Two runtime modes:**

- `ai-service serve` (default) — HTTP server with `/chat`, `/health`, `/metrics`, plus streamable HTTP MCP endpoint at `/mcp`
- `ai-service mcp` — stdio MCP server mode for Claude Desktop / Cursor

The `tools.Registry` remains the single source of truth. The MCP server adapts it outward. The MCP client adapts external tools inward.

---

## MCP Server — Exposing the 9 Tools

**Package:** `internal/mcp/server.go`

Wraps `tools.Registry` as an MCP server using the official Go SDK.

**Tool adaptation:** Each `tools.Tool` in the registry becomes an MCP tool automatically:

| `tools.Tool` method | MCP mapping |
|---|---|
| `Name()` | MCP tool name |
| `Description()` | MCP tool description |
| `Schema()` | MCP tool input schema (already JSON Schema) |
| `Call(ctx, args, userID)` | MCP tool handler |

**Auth:**

- **Streamable HTTP:** JWT sent as Bearer token in HTTP headers. The server extracts `userID` the same way the `/chat` handler does today.
- **Stdio:** JWT passed as `AI_SERVICE_TOKEN` environment variable or CLI flag. Unauthenticated tools (search_products, get_product, check_inventory) work without it.

Full OAuth 2.1 is overkill — the project already has JWT auth via auth-service.

**Streamable HTTP endpoint:** Mounted at `/mcp` on the existing Gin server. Same port, same binary.

**Stdio mode:** `ai-service mcp` reads JSON-RPC from stdin, writes to stdout. Configured in Claude Desktop's `claude_desktop_config.json` or Cursor's MCP settings.

---

## MCP Client — Consuming MCP Servers

**Package:** `internal/mcp/client.go`

Discovers tools from MCP servers and wraps each as a `tools.Tool` for the existing registry.

**`MCPClientTool`** implements `tools.Tool`:

- On startup, calls `tools/list` on the target MCP server to discover available tools and schemas
- Creates one `MCPClientTool` per discovered tool
- `Call()` sends a `tools/call` JSON-RPC request and maps the response back to `tools.Result`

**Configuration:** `MCP_SERVERS` environment variable (JSON array):

```json
[{"name": "self", "transport": "http", "url": "http://localhost:8093/mcp"}]
```

When unset, the client side is not activated — no behavior change from today.

**Tool name collisions:** MCP-sourced tools are prefixed with the server name to avoid collisions: `self.search_products`, `self.get_product`, etc. The agent sees both local and MCP versions in its tool list.

**Demo:** In the self-calling demo, the agent has 18 tools — 9 local, 9 via MCP (`self.*`). Verifiable by asking the agent to use a prefixed tool, or by disabling local tools and running purely through MCP.

---

## Binary Changes

**CLI subcommands** via simple `os.Args` check (no cobra needed):

- `ai-service serve` (default) — existing HTTP server + `/mcp` endpoint
- `ai-service mcp` — stdio MCP server mode

**Dockerfile:** No changes. Same binary, both modes.

---

## Deployment

**K8s:** No changes. `/mcp` is another route on the same pod. Existing service and ingress (`/ai-api/*`) pick it up.

**Cloudflare Tunnel:** `/mcp` reachable at `https://api.kylebradshaw.dev/ai-api/mcp` — no tunnel config changes.

**Claude Desktop config example:**

```json
{
  "mcpServers": {
    "shopping-assistant": {
      "command": "./ai-service",
      "args": ["mcp"],
      "env": {
        "ECOMMERCE_URL": "http://localhost:8092",
        "JWT_SECRET": "..."
      }
    }
  }
}
```

---

## Testing & Evals

**Unit tests:**

- `internal/mcp/server_test.go` — MCP server correctly adapts each tool from registry (schema mapping, call dispatch, auth extraction). Uses `evals.EchoTool` pattern.
- `internal/mcp/client_test.go` — `MCPClientTool` correctly wraps discovered tools and maps calls/results. Spins up in-process MCP server with test tools, connects client, asserts discovery and round-trips.
- Name-collision prefixing logic tested separately.

**Eval harness:**

- One new eval case in `internal/evals/cases_test.go`: agent routes a tool call through `MCPClientTool`, verifying the full path (agent → MCP client → MCP server → tool → result → agent). Uses `ScriptedLLM` — no real LLM needed.

**Integration test (manual):**

- Start `ai-service serve`, then `ai-service mcp` in stdio mode, pipe `tools/list` and verify 9 tools.
- Configure Claude Desktop, manually call a tool.

**CI:** Tests run as part of existing `make preflight-go`. No new CI jobs — all MCP tests are in-process.

---

## ADR

Markdown ADR at `docs/adr/go-ai-service-mcp.md` documenting:

- **Why MCP:** transport-agnostic tool registry was the v1 design goal; MCP validates it
- **Why the official Go SDK:** semver stability, official backing, ecosystem direction
- **Why embedded:** same registry, less ops, stronger narrative
- **What was rejected:** separate binary, MCP-as-middleware, Python/TS, full OAuth 2.1
- **Why both transports:** stdio for local MCP hosts, streamable HTTP for networked access

---

## Files to Create or Modify

| File | Action | Purpose |
|---|---|---|
| `go/ai-service/internal/mcp/server.go` | Create | MCP server adapter over tools.Registry |
| `go/ai-service/internal/mcp/server_test.go` | Create | Server unit tests |
| `go/ai-service/internal/mcp/client.go` | Create | MCP client → tools.Tool adapter |
| `go/ai-service/internal/mcp/client_test.go` | Create | Client unit tests |
| `go/ai-service/cmd/server/main.go` | Modify | Add subcommand routing (serve/mcp), mount `/mcp` endpoint on Gin router |
| `go/ai-service/internal/evals/cases_test.go` | Modify | Add MCP round-trip eval case |
| `go/ai-service/go.mod` | Modify | Add `modelcontextprotocol/go-sdk` dependency |
| `docs/adr/go-ai-service-mcp.md` | Create | ADR documenting decisions |

---

## Out of Scope

- Full OAuth 2.1 implementation (JWT is sufficient)
- MCP resources or prompts (tools only)
- External MCP server integration (self-calling demo is sufficient)
- Frontend changes (MCP is backend-only; the `/chat` SSE path is unchanged)
- Rich product cards in the drawer (separate roadmap item #9)
