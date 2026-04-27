# `/ai` Page MCP Highlight — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lead the `/ai` portfolio page with a new MCP Server section that explains the official-SDK MCP server, reuses the 12-tool catalog as a shared component, links to the existing `/go` interactive demo, and provides connect-your-own-client snippets for Claude Desktop, Codex CLI, and MCP Inspector.

**Architecture:** Pure frontend change — no backend touched. One shared React component (`MCPToolCatalog`) extracts the existing tool-catalog Mermaid chart out of `AiAssistantTab.tsx` so both `/ai` and `/go` render the same source of truth. Section ordering on `/ai` is reorganized so MCP leads, RAG Evaluation moves up to position 2, and existing Document Q&A and Debug sections move down without copy changes. Verification uses Playwright mocked e2e tests against the standard `frontend/e2e/mocked/` pattern (no unit-test framework exists for this project).

**Tech Stack:** Next.js 16 App Router, React 19, TypeScript, Tailwind, Mermaid v10 via the existing `<MermaidDiagram />` component, Playwright for e2e verification.

**Spec:** `docs/superpowers/specs/2026-04-27-ai-mcp-highlight-design.md` (read before starting — this plan defers definitions to the spec rather than repeating them).

**Public MCP endpoint (verified):** `https://api.kylebradshaw.dev/ai-api/mcp` — `go/k8s/ingress.yml` rewrites `/ai-api/(.*)` → `/$1` on `go-ai-service:8093`, hitting the `/mcp` handler in `go/ai-service/cmd/server/routes.go:75`. Confirmed by `frontend/e2e/smoke-prod/smoke-health.spec.ts:26-30`.

---

## File Structure

| File | Responsibility |
|---|---|
| `frontend/src/components/ai/MCPToolCatalog.tsx` *(NEW)* | Shared 12-tool Mermaid catalog. Renders the same diagram in both `/ai` and `/go` Shopping Assistant. No props. |
| `frontend/src/components/ai/MCPArchitectureDiagram.tsx` *(NEW)* | New Mermaid diagram showing the external MCP-client request path (distinct from in-app agent diagrams in `AiAssistantTab.tsx`). |
| `frontend/src/components/ai/MCPSection.tsx` *(NEW)* | Composes the entire MCP-Server section: copy, both diagrams, CTA to `/go`, connection-instruction snippets, GitHub link. Keeps `frontend/src/app/ai/page.tsx` from ballooning. |
| `frontend/src/app/ai/page.tsx` *(MODIFY)* | Add `<MCPSection />` at the top, reorder remaining sections (RAG Eval up to position 2), update one bio paragraph to mention MCP. |
| `frontend/src/components/go/tabs/AiAssistantTab.tsx` *(MODIFY)* | Replace inline tool-catalog `flowchart LR` chart string with `<MCPToolCatalog />` import. Other diagrams in this file stay as-is — they describe the in-app agent path. |
| `frontend/e2e/mocked/ai-mcp-section.spec.ts` *(NEW)* | Playwright tests verifying the MCP section renders on `/ai`, the shared tool catalog renders on both `/ai` and `/go` (Shopping Assistant tab), and the connection snippets are visible. |
| `docs/superpowers/specs/2026-04-27-ai-mcp-highlight-design.md` | Already updated on this branch with verified ingress path. No further changes needed. |

---

## Task 1: Create the failing Playwright test for the new `/ai` MCP section

**Files:**
- Create: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

This test drives every visible piece of the MCP section. Other tasks make it pass.

- [ ] **Step 1: Write the failing test**

Create `frontend/e2e/mocked/ai-mcp-section.spec.ts`:

```typescript
import { test, expect } from "./fixtures";

test.describe("/ai MCP Server section", () => {
  test("MCP Server is the first section heading on /ai", async ({ page }) => {
    await page.goto("/ai");
    const sectionHeadings = page.locator("section h2");
    await expect(sectionHeadings.first()).toHaveText("MCP Server");
  });

  test("RAG Evaluation appears as the second section on /ai", async ({ page }) => {
    await page.goto("/ai");
    const sectionHeadings = page.locator("section h2");
    await expect(sectionHeadings.nth(1)).toHaveText("RAG Evaluation");
  });

  test("MCP section shows the verified public endpoint", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByText("https://api.kylebradshaw.dev/ai-api/mcp", { exact: false }),
    ).toBeVisible();
  });

  test("MCP section renders the Claude Desktop config snippet", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByRole("heading", { name: "Claude Desktop", exact: false }),
    ).toBeVisible();
    await expect(page.getByText('"mcpServers"', { exact: false })).toBeVisible();
  });

  test("MCP section renders the Codex CLI config snippet", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByRole("heading", { name: "Codex CLI", exact: false }),
    ).toBeVisible();
  });

  test("MCP section renders the MCP Inspector command", async ({ page }) => {
    await page.goto("/ai");
    await expect(
      page.getByText("npx @modelcontextprotocol/inspector", { exact: false }),
    ).toBeVisible();
  });

  test("MCP section CTA links to the /go shopping assistant tab", async ({
    page,
  }) => {
    await page.goto("/ai");
    const cta = page.getByRole("link", { name: /Try it on the Go section/i });
    await expect(cta).toBeVisible();
    await expect(cta).toHaveAttribute("href", "/go");
  });

  test("MCP section links to the GitHub source for the MCP server", async ({
    page,
  }) => {
    await page.goto("/ai");
    const githubLink = page.getByRole("link", { name: /View source on GitHub/i });
    await expect(githubLink).toBeVisible();
    await expect(githubLink).toHaveAttribute(
      "href",
      /github\.com\/.*\/go\/ai-service\/internal\/mcp/,
    );
  });

  test("Tool catalog renders on /ai (shared component, identifying caption)", async ({
    page,
  }) => {
    await page.goto("/ai");
    await expect(
      page.getByText(/twelve tools/i, { exact: false }).first(),
    ).toBeVisible();
  });

  test("Tool catalog renders on /go AI Assistant tab", async ({ page }) => {
    await page.goto("/go");
    await page.getByRole("button", { name: "AI Assistant" }).click();
    await expect(page.getByText(/twelve tools/i, { exact: false })).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd frontend && npx playwright test e2e/mocked/ai-mcp-section.spec.ts --reporter=list
```

Expected: every test fails. The first test fails with the first `<h2>` in `<section>` reading "Document Q&A Assistant" (current top section), not "MCP Server."

If Playwright browsers are not installed, run `npx playwright install chromium` first.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/mocked/ai-mcp-section.spec.ts
git commit -m "test(ai): failing e2e for new /ai MCP section"
```

---

## Task 2: Extract the 12-tool catalog into `MCPToolCatalog` shared component

**Files:**
- Create: `frontend/src/components/ai/MCPToolCatalog.tsx`
- Modify: `frontend/src/components/go/tabs/AiAssistantTab.tsx` (lines 17–58 — the `<h3>Tool Catalog</h3>` block + the `<MermaidDiagram chart={...} />` containing the `flowchart LR` with `AGENT`, `Catalog`, `Orders`, `CartReturns`, `Knowledge` subgraphs)

The phrase **"twelve tools"** must appear in the component's prose so the e2e selector from Task 1 has something to bind to in both render locations.

- [ ] **Step 1: Create the shared component**

Create `frontend/src/components/ai/MCPToolCatalog.tsx`:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";

const toolCatalogChart = `flowchart LR
  AGENT((MCP client<br/>or in-app agent))
  subgraph Catalog ["Catalog (public)"]
    T1[search_products<br/>query + max_price]
    T2[get_product<br/>full details by ID]
    T3[check_inventory<br/>stock count]
  end
  subgraph Orders ["Orders (auth-scoped)"]
    T4[list_orders<br/>last 20 orders]
    T5[get_order<br/>single order detail]
    T6[summarize_orders<br/>LLM-generated summary]
  end
  subgraph CartReturns ["Cart & Returns (auth-scoped)"]
    T7[view_cart<br/>items + total]
    T8[add_to_cart<br/>product + quantity]
    T9[initiate_return<br/>order item + reason]
  end
  subgraph Knowledge ["Knowledge Base (public, RAG)"]
    T10[search_documents<br/>semantic search + sources]
    T11[ask_document<br/>natural-language Q&A]
    T12[list_collections<br/>vector store inventory]
  end
  AGENT --> Catalog
  AGENT --> Orders
  AGENT --> CartReturns
  AGENT --> Knowledge
  X[place_order<br/>deliberately excluded]:::disabled
  AGENT -.-x X
  classDef disabled stroke-dasharray: 5 5,opacity:0.5`;

export function MCPToolCatalog() {
  return (
    <div>
      <h3 className="mt-10 text-xl font-semibold">Tool Catalog</h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The MCP server exposes twelve tools across four domains. Catalog and
        knowledge-base tools are public; order, cart, and return tools require a
        Bearer JWT. Knowledge-base tools call the Python RAG pipeline through a
        circuit-breaker HTTP bridge with a 30-second timeout. Checkout
        (<code>place_order</code>) is deliberately excluded — the agent can advise
        but not transact.
      </p>
      <div className="mt-6">
        <MermaidDiagram chart={toolCatalogChart} />
      </div>
    </div>
  );
}
```

Note: the `AGENT` node label changed from `"Agent<br/>Qwen 2.5 14B"` to `"MCP client<br/>or in-app agent"` to keep the component honest at both render sites — an external MCP client doesn't run Qwen. The Go AI Assistant tab now describes the LLM separately in its surrounding copy.

- [ ] **Step 2: Replace the inline diagram in `AiAssistantTab.tsx`**

Open `frontend/src/components/go/tabs/AiAssistantTab.tsx`. Replace lines 17–58 (the `<h3>Tool Catalog</h3>` heading, the surrounding `<p>` paragraph, and the entire `<div className="mt-6"><MermaidDiagram chart={...} /></div>` block containing the `flowchart LR` chart) with a single `<MCPToolCatalog />` invocation.

Final replacement block (replaces those lines exactly — keep `<p className="mt-4 ...">An LLM-powered shopping assistant ...</p>` from line 6 untouched):

```tsx
import { MCPToolCatalog } from "@/components/ai/MCPToolCatalog";
```

(at the top of the file, alongside the existing `import { MermaidDiagram } ...`)

```tsx
      <MCPToolCatalog />
```

(replacing the lines that previously held the heading, paragraph, and Mermaid block)

After the change, `AiAssistantTab.tsx` should still describe the in-app Agent Loop, Product-search sequence, and Knowledge-query sequence diagrams — those stay as-is.

- [ ] **Step 3: Run the `/go` tool-catalog test**

```bash
cd frontend && npx playwright test e2e/mocked/ai-mcp-section.spec.ts -g "Tool catalog renders on /go" --reporter=list
```

Expected: PASS. (The other tests still fail because `/ai` doesn't have the section yet.)

- [ ] **Step 4: Run typecheck**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ai/MCPToolCatalog.tsx \
        frontend/src/components/go/tabs/AiAssistantTab.tsx
git commit -m "refactor(ai): extract MCPToolCatalog shared component"
```

---

## Task 3: Build the `MCPArchitectureDiagram` component

**Files:**
- Create: `frontend/src/components/ai/MCPArchitectureDiagram.tsx`

This diagram must visibly differ from the existing in-app agent sequence diagrams in `AiAssistantTab.tsx`. No `Ollama` box. The external MCP client owns its own LLM.

- [ ] **Step 1: Create the component**

Create `frontend/src/components/ai/MCPArchitectureDiagram.tsx`:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";

const mcpArchitectureChart = `flowchart LR
  subgraph Clients ["External MCP clients"]
    direction TB
    CD[Claude Desktop]
    CX[Codex CLI]
    INS[MCP Inspector]
  end
  subgraph Server ["ai-service /mcp endpoint (Go)"]
    direction TB
    HTTP[HTTPS Streamable<br/>transport]
    AUTH{Bearer JWT?}
    REG[Tool registry<br/>12 tools]
    HTTP --> AUTH
    AUTH -->|valid token| REG
    AUTH -->|absent| REG
  end
  subgraph Backends ["Backends (in-cluster)"]
    direction TB
    EC[Ecommerce<br/>REST + gRPC]
    RAG[Python RAG bridge<br/>circuit breaker, OTel]
    QD[(Qdrant)]
    OLL[(Ollama)]
    RAG --> QD
    RAG --> OLL
  end
  CD -->|"public + auth-scoped<br/>tools"| HTTP
  CX --> HTTP
  INS -->|"discovery + invoke"| HTTP
  REG --> EC
  REG --> RAG`;

export function MCPArchitectureDiagram() {
  return <MermaidDiagram chart={mcpArchitectureChart} />;
}
```

- [ ] **Step 2: Typecheck**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ai/MCPArchitectureDiagram.tsx
git commit -m "feat(ai): MCP architecture diagram for external-client path"
```

---

## Task 4: Build the `MCPSection` component (composes the full section)

**Files:**
- Create: `frontend/src/components/ai/MCPSection.tsx`

This component must contain the visible strings the Task 1 tests assert on:
- `<h2>MCP Server</h2>`
- `https://api.kylebradshaw.dev/ai-api/mcp` (visible somewhere in body text or code block)
- `<h3>Claude Desktop</h3>` and `"mcpServers"` (in the JSON snippet)
- `<h3>Codex CLI</h3>`
- `npx @modelcontextprotocol/inspector` (in the Inspector snippet)
- A link with text matching `/Try it on the Go section/i` and `href="/go"`
- A link with text matching `/View source on GitHub/i` and `href` matching `github.com/.*/go/ai-service/internal/mcp`

- [ ] **Step 1: Create the component**

Create `frontend/src/components/ai/MCPSection.tsx`:

```tsx
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
        require a Bearer JWT — register at{" "}
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
```

- [ ] **Step 2: Typecheck**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ai/MCPSection.tsx
git commit -m "feat(ai): MCPSection component with diagrams + connect snippets"
```

---

## Task 5: Wire `MCPSection` into `/ai` and reorder existing sections

**Files:**
- Modify: `frontend/src/app/ai/page.tsx`

Final section order on the page after this task:
1. Header `<h1>` + bio paragraph (bio updated)
2. `<MCPSection />` (NEW)
3. RAG Evaluation `<section>` (moved up from position 4)
4. Document Q&A Assistant `<section>` + How It Works diagram + demo CTA (moved down)
5. Debug Assistant `<section>` + How It Works diagram + demo CTA (unchanged position relative to Document Q&A)

The Eval Demo CTA and Document Q&A demo CTA stay attached to their respective sections — when reordering, drag each section together with its trailing diagram + CTA blocks.

- [ ] **Step 1: Read the existing page in full to anchor the edit**

```bash
cat frontend/src/app/ai/page.tsx | head -200
```

Confirm the current order is: Document Q&A → Debug → RAG Evaluation. The reorder moves RAG Evaluation between the new MCP section and Document Q&A.

- [ ] **Step 2: Edit `frontend/src/app/ai/page.tsx`**

Replace the existing default-export function body with the new ordering. Concrete edits:

a. **Add the import** at the top of the file, alongside the existing imports:

```tsx
import { MCPSection } from "@/components/ai/MCPSection";
```

b. **Update the bio paragraph** (currently lines 50–55). Change the first sentence so it leads with the MCP framing:

```tsx
          <p className="text-muted-foreground leading-relaxed">
            Building intelligent systems with retrieval-augmented generation,
            agentic architectures, and Model Context Protocol (MCP) servers
            that any AI client can call. This section demonstrates an MCP
            server fronting twelve tools, RAG pipelines with evaluation,
            vector search, and tool-using agents — built with FastAPI, Qdrant,
            Ollama, and Go, deployed on Kubernetes.
          </p>
```

The Grafana paragraph immediately below stays as-is.

c. **Insert `<MCPSection />`** as the first content section after the bio block. Place it before the existing `{/* Project Explanation */}` section.

d. **Move the RAG Evaluation section** (currently the last `<section>` plus its "Try RAG Evaluation" CTA `<section>`) so they sit immediately after `<MCPSection />` and before the Document Q&A section.

After the edit, the JSX body order should look like (comments only, not literal):

```
- Header (h1)
- Bio section (updated copy)
- <MCPSection />
- RAG Evaluation section + Eval Demo CTA section
- Document Q&A section + How It Works (architectureDiagram) + Demo CTA section
- Debug Assistant section + How It Works (debugArchitectureDiagram) + Debug Demo CTA section
```

Leave the `architectureDiagram` and `debugArchitectureDiagram` constants at the top of the file untouched.

- [ ] **Step 3: Run typecheck**

```bash
cd frontend && npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Run the `/ai` MCP tests**

```bash
cd frontend && npx playwright test e2e/mocked/ai-mcp-section.spec.ts --reporter=list
```

Expected: all tests PASS.

- [ ] **Step 5: Spot-check visually**

```bash
cd frontend && npm run dev
```

Open `http://localhost:3000/ai`. Confirm:
- "MCP Server" is the first `<h2>` after the bio.
- Architecture diagram and Tool Catalog render via Mermaid.
- Three connection snippets appear with correct heading copy.
- "Try it on the Go section →" links to `/go`.
- "View source on GitHub →" link points at the `internal/mcp` directory.
- RAG Evaluation appears immediately below MCP Server.
- Document Q&A and Debug sections appear after, with their existing diagrams and CTAs intact.

Stop the dev server after the spot-check.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/ai/page.tsx
git commit -m "feat(ai): lead /ai page with MCP Server section, reorder sections"
```

---

## Task 6: Run the project's full frontend preflight

**Files:** none modified.

- [ ] **Step 1: Run preflight-frontend**

```bash
make preflight-frontend
```

This runs `tsc`, `next build`, `eslint`. Expected: PASS.

- [ ] **Step 2: Run mocked e2e**

```bash
make preflight-e2e
```

Expected: full mocked-e2e suite passes, including the new `ai-mcp-section.spec.ts`.

If either preflight fails, fix the failure inline and re-run before proceeding. Type errors and lint errors are mechanical fixes; do not commit broken code through to push.

- [ ] **Step 3: Commit any preflight-driven fixes**

If Step 1 or Step 2 produced fixes, commit them with a focused message such as `chore(ai): fix lint/typecheck under MCP highlight`. If no fixes were needed, skip this step — no empty commit.

---

## Task 7: Push and open the PR to `qa`

Per `CLAUDE.md` feature-branch flow: spec was already approved, plan execution proceeds to push without further approval, no CI watching.

- [ ] **Step 1: Push the branch**

```bash
cd /Users/kylebradshaw/repos/gen_ai_engineer/.claude/worktrees/agent-feat-ai-mcp-highlight
git push -u origin agent/feat-ai-mcp-highlight
```

- [ ] **Step 2: Open the PR against `qa`**

```bash
gh pr create --base qa --title "Highlight MCP server on /ai page" --body "$(cat <<'EOF'
## Summary
- Lead the /ai portfolio page with a new MCP Server section explaining the official-SDK MCP server, its architecture, and the 12-tool catalog.
- Extract the tool catalog into a shared `<MCPToolCatalog />` component reused on both /ai and /go (Shopping Assistant tab).
- Add connect-your-own-client snippets for Claude Desktop, Codex CLI, and MCP Inspector pointing at the verified public endpoint `https://api.kylebradshaw.dev/ai-api/mcp`.
- Reorder /ai sections so RAG Evaluation sits immediately below the new MCP section.

Closes #79 — RAG eval harness shipped under `services/eval/`.
Closes #83 — Eval Service UI shipped at `/ai/eval`.

## Test plan
- [ ] `make preflight-frontend` passes (tsc + next build + eslint)
- [ ] `make preflight-e2e` passes, including new `frontend/e2e/mocked/ai-mcp-section.spec.ts`
- [ ] Visual check on QA: MCP Server is first `<h2>`, RAG Evaluation second; tool catalog renders on both /ai and /go
- [ ] Public endpoint resolves: `curl -s -o /dev/null -w "%{http_code}\n" https://qa-api.kylebradshaw.dev/ai-api/mcp` returns a non-5xx response (MCP handshake will not accept a plain GET, but the server should respond)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Capture the PR URL**

The previous step prints the PR URL. Save it to report back to Kyle.

---

## Task 8: Close completed roadmap issues

Per spec, issues #79 and #83 represent work that has already shipped. Close them with reference comments so the open-issue list reflects reality.

- [ ] **Step 1: Close #79 with a reference comment**

```bash
gh issue close 79 --comment "RAG evaluation harness shipped under \`services/eval/\` (see \`services/eval/app/evaluator.py\` and the corresponding tests). The interactive UI driving it landed at \`/ai/eval\` (tracked separately under #83). Per the 2026-04-27 \`/ai\` MCP highlight spec, this issue is closed as done."
```

- [ ] **Step 2: Close #83 with a reference comment**

```bash
gh issue close 83 --comment "Eval Service UI shipped at \`/ai/eval\` (see \`frontend/src/app/ai/eval/page.tsx\` and \`frontend/src/components/eval/\`). Datasets, Evaluate, and Results tabs are all implemented and the page is live in production. Closing as done per the 2026-04-27 \`/ai\` MCP highlight spec."
```

- [ ] **Step 3: Verify both are closed**

```bash
gh issue list --state open --search "in:title Phase 4a OR in:title Eval Service UI" --limit 5
```

Expected: empty result.

---

## Self-review — done before handoff to execution

**Spec coverage:**
- ✅ Section 1a (What & why) — Task 4 prose
- ✅ Section 1b (Architecture diagram) — Task 3
- ✅ Section 1c (Tool catalog) — Task 2 + reuse in Task 4
- ✅ Section 1d (Try it interactively / link to /go) — Task 4 CTA
- ✅ Section 1e (Connect your own client) — Task 4 snippets
- ✅ Section 1f (GitHub link) — Task 4 footer
- ✅ Bio paragraph update — Task 5b
- ✅ Section reordering — Task 5d
- ✅ Component changes table — Tasks 2–5
- ✅ Issue cleanup — Task 8
- ✅ Verification (preflight, MCP-section tests, public endpoint) — Tasks 6 + 7 PR test plan

**Placeholders:** none. The Codex config and Inspector command are written verbatim using the formats both tools currently document. Codex `~/.codex/config.toml` `[mcp_servers.<name>]` block and Inspector `npx @modelcontextprotocol/inspector <url>` are stable. If the format has shifted by impl time, the executing agent updates the snippet rather than ships stale.

**Type consistency:** `<MCPToolCatalog />`, `<MCPArchitectureDiagram />`, `<MCPSection />` — names used identically across all tasks. Imports use the `@/components/ai/...` alias matching the project's existing tsconfig pattern.
