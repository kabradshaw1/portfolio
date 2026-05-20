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

const observabilityProofPoints = [
  "Prometheus",
  "Loki",
  "Jaeger",
  "Grafana",
  "5 dashboards",
  "16 alert rules",
];

const observabilityTools = [
  "get_system_health",
  "investigate_checkout",
  "investigate_ai_pipeline",
  "investigate_eval_run",
  "investigate_streaming_analytics",
  "get_service_evidence",
  "search_logs",
  "get_trace",
];

const evalProofPoints = [
  "Eval API",
  "RAG collections",
  "dataset fixtures",
  "evaluation runs",
  "experiments",
  "rerank",
  "top_k",
];

const evalTools = [
  "start_eval_run",
  "wait_for_eval_run",
  "compare_eval_runs",
  "get_worst_eval_cases",
  "get_rag_collection_config",
  "record_eval_experiment_conclusion",
  "summarize_eval_experiment",
];

export function MCPSection() {
  return (
    <section id="mcp-server" className="mt-12 scroll-mt-20">
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

      <section id="internal-mcps" className="mt-10 scroll-mt-20">
        <h3 className="text-xl font-semibold">
          Internal MCPs for Engineering Operations
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          These MCPs are Codex-facing control surfaces over larger engineering
          systems. The route stays focused on what the MCP layer adds: bounded
          tool access, evidence gathering, repeatable workflows, and links into
          the deeper platforms behind each service.
        </p>

        <div className="mt-6 grid gap-4 lg:grid-cols-2">
          <article className="rounded-lg border border-foreground/10 bg-card p-4 sm:p-5">
            <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Read-only production evidence
            </div>
            <h4 className="mt-2 text-lg font-semibold">Observability MCP</h4>
            <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
              A local MCP endpoint over the metrics, logs, traces, dashboard,
              and runbook layers. It turns operational questions into bounded
              evidence bundles for system health, checkout incidents, AI
              pipeline failures, eval runs, streaming analytics, service logs,
              and trace lookup without mutating the cluster.
            </p>

            <div className="mt-4 flex flex-wrap gap-2">
              {observabilityProofPoints.map((point) => (
                <span
                  key={point}
                  className="rounded-md border border-foreground/10 bg-muted px-2 py-1 text-xs text-muted-foreground"
                >
                  {point}
                </span>
              ))}
            </div>

            <div className="mt-4">
              <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Representative tools
              </div>
              <div className="mt-2 flex flex-wrap gap-2">
                {observabilityTools.map((tool) => (
                  <code
                    key={tool}
                    className="rounded bg-muted px-2 py-1 text-xs text-muted-foreground"
                  >
                    {tool}
                  </code>
                ))}
              </div>
            </div>

            <Link
              href="/observability"
              className="mt-5 inline-flex text-sm font-medium underline hover:text-foreground"
            >
              See the full observability platform &rarr;
            </Link>
          </article>

          <article className="rounded-lg border border-foreground/10 bg-card p-4 sm:p-5">
            <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              RAG experiment control plane
            </div>
            <h4 className="mt-2 text-lg font-semibold">Eval MCP service</h4>
            <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
              An operator MCP for repeatable RAG evaluation work. It coordinates
              evaluator datasets, run history, study records, conclusions,
              collection metadata from ingestion, retrieval settings, ranking
              comparisons, worst-case review, and the eval-run evidence handoff
              into the observability control plane.
            </p>

            <div className="mt-4 flex flex-wrap gap-2">
              {evalProofPoints.map((point) => (
                <span
                  key={point}
                  className="rounded-md border border-foreground/10 bg-muted px-2 py-1 text-xs text-muted-foreground"
                >
                  {point}
                </span>
              ))}
            </div>

            <div className="mt-4">
              <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Representative tools
              </div>
              <div className="mt-2 flex flex-wrap gap-2">
                {evalTools.map((tool) => (
                  <code
                    key={tool}
                    className="rounded bg-muted px-2 py-1 text-xs text-muted-foreground"
                  >
                    {tool}
                  </code>
                ))}
              </div>
            </div>

            <Link
              href="/ai/eval"
              className="mt-5 inline-flex text-sm font-medium underline hover:text-foreground"
            >
              Open the RAG evaluation workflow &rarr;
            </Link>
          </article>
        </div>

        <article className="mt-4 rounded-lg border border-foreground/10 bg-card p-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Structured practice feedback
          </div>
          <h4 className="mt-2 text-lg font-semibold">QA MCP</h4>
          <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
            A compact internal MCP for structured practice sessions, expected
            answer feedback, weak-topic tracking, review attempts, and scoring.
            It keeps interview preparation in the same tool-call workflow
            without competing with the production observability and RAG eval
            control planes.
          </p>
        </article>
      </section>

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

      <section id="connect-client" className="mt-10 scroll-mt-20">
        <h3 className="text-xl font-semibold">Connect your own client</h3>
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
      </section>

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
