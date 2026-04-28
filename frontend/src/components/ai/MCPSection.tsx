import Link from "next/link";
import { MCPArchitectureDiagram } from "./MCPArchitectureDiagram";
import { MCPToolCatalog } from "./MCPToolCatalog";

const claudeDesktopConfig = `{
  "mcpServers": {
    "kyle-portfolio": {
      "transport": "http",
      "url": "https://api.kylebradshaw.dev/ai-api/mcp"
    }
  }
}`;

const codexConfig = `[mcp_servers.kyle-portfolio]
transport = "http"
url = "https://api.kylebradshaw.dev/ai-api/mcp"`;

const inspectorCommand = `npx @modelcontextprotocol/inspector https://api.kylebradshaw.dev/ai-api/mcp`;

const githubMcpUrl =
  "https://github.com/kabradshaw1/portfolio/tree/main/go/ai-service/internal/mcp";

export function MCPSection() {
  return (
    <section className="mt-12">
      <h2 className="text-2xl font-semibold">MCP Server</h2>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The Go ai-service exposes twelve tools to any{" "}
        <a
          href="https://modelcontextprotocol.io"
          target="_blank"
          rel="noopener noreferrer"
          className="underline hover:text-foreground transition-colors"
        >
          Model Context Protocol
        </a>{" "}
        client over HTTPS. Built on the official{" "}
        <code>modelcontextprotocol/go-sdk</code>, it fronts both the ecommerce
        backend (REST + gRPC) and a Python RAG pipeline (HTTP, circuit breaker,
        OTel trace propagation). Authentication is optional: catalog and
        knowledge-base tools work anonymously; cart, order, and return tools
        require a Bearer JWT.
      </p>

      <h3 className="mt-10 text-xl font-semibold">Architecture</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        External MCP clients connect over the HTTPS Streamable transport. The
        Go server enforces optional JWT auth, then routes tool calls to either
        the ecommerce backend or the Python RAG bridge. The bridge uses a
        circuit breaker with a 30-second timeout and propagates OTel trace
        context across the language boundary.
      </p>
      <div className="mt-6 rounded-xl border border-foreground/10 bg-card p-6">
        <MCPArchitectureDiagram />
      </div>

      <MCPToolCatalog />

      <h3 className="mt-10 text-xl font-semibold">Try it interactively</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The same tool registry powers an in-browser agent loop on the Go
        section. The agent runs Qwen 2.5 14B locally and streams tool calls
        and results live.
      </p>
      <div className="mt-6">
        <Link
          href="/go"
          className="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors"
        >
          Try it on the Go section &rarr;
        </Link>
      </div>

      <h3 className="mt-10 text-xl font-semibold">Connect your own client</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The MCP server is publicly reachable at{" "}
        <code className="rounded bg-muted px-1.5 py-0.5 text-sm">
          https://api.kylebradshaw.dev/ai-api/mcp
        </code>
        . Public tools (catalog search, RAG search,{" "}
        <code>list_collections</code>) work without auth. Auth-scoped tools
        require a Bearer JWT &mdash; register at{" "}
        <Link href="/go/register" className="underline hover:text-foreground">
          /go/register
        </Link>
        , log in, and copy the access token from the{" "}
        <code>Authorization</code> header in DevTools.
      </p>

      <h4 className="mt-6 text-lg font-medium">Claude Desktop</h4>
      <p className="mt-2 text-sm text-muted-foreground">
        Add to{" "}
        <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
          ~/Library/Application Support/Claude/claude_desktop_config.json
        </code>
        :
      </p>
      <pre className="mt-3 overflow-x-auto rounded-lg border border-foreground/10 bg-card p-4 text-xs">
        <code>{claudeDesktopConfig}</code>
      </pre>

      <h4 className="mt-6 text-lg font-medium">Codex CLI</h4>
      <p className="mt-2 text-sm text-muted-foreground">
        Add to{" "}
        <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
          ~/.codex/config.toml
        </code>
        :
      </p>
      <pre className="mt-3 overflow-x-auto rounded-lg border border-foreground/10 bg-card p-4 text-xs">
        <code>{codexConfig}</code>
      </pre>

      <h4 className="mt-6 text-lg font-medium">MCP Inspector</h4>
      <p className="mt-2 text-sm text-muted-foreground">
        Browse and invoke tools directly:
      </p>
      <pre className="mt-3 overflow-x-auto rounded-lg border border-foreground/10 bg-card p-4 text-xs">
        <code>{inspectorCommand}</code>
      </pre>

      <p className="mt-8 text-sm">
        <a
          href={githubMcpUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="underline hover:text-foreground transition-colors"
        >
          View source on GitHub &rarr;
        </a>
      </p>
    </section>
  );
}
