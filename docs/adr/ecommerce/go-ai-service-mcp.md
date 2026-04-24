# ADR: MCP Support in go/ai-service

**Date:** 2026-04-10
**Status:** Accepted

## Context

The `go/ai-service` v1 shipped with a `tools.Tool` interface and `tools.Registry` designed to be transport-agnostic. The comment in `registry.go` — "Tool is the only interface a future MCP adapter needs to satisfy" — made the intent explicit. MCP (Model Context Protocol) has become the emerging standard for tool interop between AI agents and tool providers.

## Decision

Add both an MCP server (exposing the 9 tools) and an MCP client (consuming external MCP servers) to the existing ai-service binary, using the official `modelcontextprotocol/go-sdk`.

### Why the official Go SDK

- v1.x with semver stability guarantees
- Maintained by the MCP org and Google
- GitHub's own MCP Server migrated from `mark3labs/mcp-go` to this SDK
- Single `mcp` package, idiomatic Go API

### Why embedded (not a separate binary)

One registry, one deploy, one test surface. The "transport-agnostic" story is literal when it's the same binary serving tools through both SSE (`/chat`) and MCP (`/mcp`).

### Why both transports (stdio + streamable HTTP)

- **Stdio** — required for Claude Desktop, Cursor, and most MCP hosts
- **Streamable HTTP** — enables network-accessible MCP server, needed for the agent's own MCP client to connect (K8s pod-to-pod)

### Why not full OAuth 2.1

The project already has JWT auth via auth-service. Adding a full OAuth 2.1 authorization server would be significant scope for zero portfolio signal. The MCP server accepts Bearer JWTs using the same validation logic as `/chat`.

## Alternatives Rejected

- **Separate MCP server binary** — doubles the deployment surface for no demo value
- **MCP-as-middleware** (all tool calls go through MCP) — over-engineered; adds JSON-RPC overhead to every local tool call
- **Python or TypeScript** — breaks the "same registry, two transports" Go portfolio story
- **`mark3labs/mcp-go`** — community library, pre-v1 (v0.47), no stability guarantees

## Consequences

- The ai-service binary gains a `mcp` subcommand for stdio mode
- The `/mcp` endpoint is automatically available on the existing HTTP port
- MCP client support is opt-in via `MCP_SERVERS` environment variable
- `summarize_orders` tool is excluded from stdio mode (requires LLM client)
