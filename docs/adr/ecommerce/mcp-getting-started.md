# MCP Getting Started Guide

The ai-service now supports MCP (Model Context Protocol) in two modes: as a server (exposing your 9 ecommerce tools) and as a client (consuming tools from external MCP servers).

## Prerequisites

Build the binary:

```bash
cd go/ai-service && go build -o ai-service ./cmd/server/
```

You'll also need the ecommerce-service running so the tools have a backend to call.

---

## Experiment 1: Connect Claude Desktop to your tools

This lets Claude Desktop call your ecommerce tools directly — search products, check inventory, look up orders, etc.

1. Open your Claude Desktop config:

   ```bash
   # macOS
   code ~/Library/Application\ Support/Claude/claude_desktop_config.json
   ```

2. Add the shopping-assistant server:

   ```json
   {
     "mcpServers": {
       "shopping-assistant": {
         "command": "/Users/kylebradshaw/repos/gen_ai_engineer/go/ai-service/ai-service",
         "args": ["mcp"],
         "env": {
           "ECOMMERCE_URL": "http://localhost:8092",
           "JWT_SECRET": "your-jwt-secret"
         }
       }
     }
   }
   ```

3. Restart Claude Desktop. You should see the tools icon showing 8 available tools.

4. Try asking Claude: "Search for products under $50" or "What's in the product catalog?"

**Note:** User-scoped tools (orders, cart) require `AI_SERVICE_TOKEN` set to a valid JWT. Without it, only catalog tools (search, get product, check inventory) will work.

---

## Experiment 2: Connect Cursor to your tools

Same idea, different config location.

1. Open Cursor settings and find the MCP server configuration.

2. Add the same server config as Experiment 1.

3. Now when Cursor's AI assistant needs product data, it can call your tools.

---

## Experiment 3: Streamable HTTP endpoint

The `/mcp` endpoint is automatically available when running in `serve` mode.

1. Start the service normally:

   ```bash
   ./ai-service serve
   # or: go run ./cmd/server/ serve
   ```

2. The MCP endpoint is now live at `http://localhost:8093/mcp`.

3. Any MCP client that speaks streamable HTTP can connect. Test with curl:

   ```bash
   # Initialize a session
   curl -X POST http://localhost:8093/mcp \
     -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'
   ```

4. In production, this endpoint is reachable at `https://api.kylebradshaw.dev/ai-api/mcp`.

---

## Experiment 4: Self-calling demo (agent calls its own MCP server)

This proves the full round-trip: agent -> MCP client -> MCP server -> tool -> result -> agent.

1. Start the service with `MCP_SERVERS` pointing to itself:

   ```bash
   MCP_SERVERS='[{"name":"self","transport":"http","url":"http://localhost:8093/mcp"}]' \
     ./ai-service serve
   ```

2. The agent now has 18 tools — 9 local + 9 via MCP (prefixed `self.*`).

3. Open the Shopping Assistant drawer in the frontend and chat. You can ask it to use the MCP-prefixed tools specifically:

   > "Use the self.search_products tool to find jackets"

4. Watch the tool call events — you'll see `self.search_products` going through the MCP protocol.

---

## Experiment 5: Test stdio mode directly

Pipe JSON-RPC messages to the binary to see the protocol in action:

```bash
# List available tools
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}' | \
  ECOMMERCE_URL=http://localhost:8092 ./ai-service mcp
```

---

## What to explore next

- **Auth flow:** Generate a JWT from the auth-service, set it as `AI_SERVICE_TOKEN`, and try user-scoped tools (list orders, view cart) through Claude Desktop.
- **Multiple MCP servers:** Point `MCP_SERVERS` at a different MCP server (not just self) to see how tool discovery works across services.
- **Monitoring:** Watch Prometheus metrics at `/metrics` — MCP tool calls go through the same `ai_tool_calls_total` counter as local calls.
