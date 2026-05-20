# AI Internal MCP Hub Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `/ai` into a navigation-assisted hub and replace the current internal MCP block with a stronger Observability MCP and Eval MCP showcase.

**Architecture:** Add a small reusable `AISectionNav` component for page anchors. Keep the internal MCP showcase in `MCPSection.tsx` because the copy and tool chips are specific to the `/ai` MCP presentation. Use existing Next.js, React, Tailwind, and Playwright patterns without changing MCP runtime behavior.

**Tech Stack:** Next.js, React, TypeScript, Tailwind CSS, Mermaid, Playwright.

---

## File Structure

- Create `frontend/src/components/ai/AISectionNav.tsx`
  - Owns the compact responsive anchor navigation for `/ai`.
  - Keeps page-level navigation styling out of `frontend/src/app/ai/page.tsx`.
- Modify `frontend/src/app/ai/page.tsx`
  - Imports and renders `AISectionNav`.
  - Adds stable section IDs for RAG Evaluation and Debugging Assistant.
  - Renames "Debug Assistant" to "Debugging Assistant" to match the nav label.
- Modify `frontend/src/components/ai/MCPSection.tsx`
  - Adds `id="mcp-server"` to the top MCP section.
  - Replaces the existing internal MCP visual block with featured Observability MCP and Eval MCP panels plus a compact QA MCP panel.
  - Adds `id="connect-client"` to the client configuration section.
- Modify `frontend/e2e/mocked/ai-mcp-section.spec.ts`
  - Extends mocked `/ai` tests for nav anchors, Observability MCP proof points, Eval MCP proof points, and deep links.

## Task 1: Add failing e2e coverage for hub navigation and featured MCP panels

**Files:**
- Modify: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Confirm worktree context**

Run from the feature worktree root:

```bash
pwd
git branch --show-current
git rev-parse --show-toplevel
git status --short
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/internal-mcp-visual-highlight
feature/internal-mcp-visual-highlight
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/internal-mcp-visual-highlight
```

`git status --short` may show only committed-ahead state before edits. If the
working directory is not inside `.codex/worktrees/internal-mcp-visual-highlight`,
stop and switch to that worktree before editing.

- [ ] **Step 2: Replace the internal MCP test and add a nav test**

In `frontend/e2e/mocked/ai-mcp-section.spec.ts`, replace the existing test
named `"MCP section highlights the internal engineering MCP workflow"` with the
two tests below:

```ts
  test("AI page exposes anchor navigation for the major systems", async ({
    page,
  }) => {
    await page.goto("/ai");

    const nav = page.getByRole("navigation", { name: "AI section navigation" });
    await expect(nav).toBeVisible();

    await expect(
      nav.getByRole("link", { name: "MCP Server" }),
    ).toHaveAttribute("href", "#mcp-server");
    await expect(
      nav.getByRole("link", { name: "Internal MCPs" }),
    ).toHaveAttribute("href", "#internal-mcps");
    await expect(
      nav.getByRole("link", { name: "RAG Evaluation" }),
    ).toHaveAttribute("href", "#rag-evaluation");
    await expect(
      nav.getByRole("link", { name: "Debugging Assistant" }),
    ).toHaveAttribute("href", "#debugging-assistant");
    await expect(
      nav.getByRole("link", { name: "Connect a Client" }),
    ).toHaveAttribute("href", "#connect-client");
  });

  test("MCP section highlights internal MCP ecosystems and deeper routes", async ({
    page,
  }) => {
    await page.goto("/ai");

    const section = page.locator("#internal-mcps");
    await expect(section).toBeVisible();
    await expect(
      section.getByRole("heading", {
        name: "Internal MCPs for Engineering Operations",
      }),
    ).toBeVisible();

    await expect(
      section.getByRole("heading", { name: "Observability MCP" }),
    ).toBeVisible();
    await expect(section.getByText("Prometheus")).toBeVisible();
    await expect(section.getByText("Loki")).toBeVisible();
    await expect(section.getByText("Jaeger")).toBeVisible();
    await expect(section.getByText("Grafana")).toBeVisible();
    await expect(section.getByText("5 dashboards")).toBeVisible();
    await expect(section.getByText("16 alert rules")).toBeVisible();
    await expect(section.getByText("get_system_health")).toBeVisible();
    await expect(section.getByText("investigate_checkout")).toBeVisible();
    await expect(section.getByText("get_trace")).toBeVisible();
    await expect(
      section.getByRole("link", { name: "See the full observability platform" }),
    ).toHaveAttribute("href", "/observability");

    await expect(
      section.getByRole("heading", { name: "Eval MCP service" }),
    ).toBeVisible();
    await expect(section.getByText("Eval API")).toBeVisible();
    await expect(section.getByText("RAG collections")).toBeVisible();
    await expect(section.getByText("dataset fixtures")).toBeVisible();
    await expect(section.getByText("evaluation runs")).toBeVisible();
    await expect(section.getByText("experiments")).toBeVisible();
    await expect(section.getByText("rerank")).toBeVisible();
    await expect(section.getByText("top_k")).toBeVisible();
    await expect(section.getByText("start_eval_run")).toBeVisible();
    await expect(section.getByText("compare_eval_runs")).toBeVisible();
    await expect(section.getByText("get_worst_eval_cases")).toBeVisible();
    await expect(
      section.getByRole("link", { name: "Open the RAG evaluation workflow" }),
    ).toHaveAttribute("href", "/ai/eval");

    await expect(
      section.getByRole("heading", { name: "QA MCP" }),
    ).toBeVisible();
    await expect(section.getByText("weak-topic tracking")).toBeVisible();
  });
```

- [ ] **Step 3: Run the targeted e2e test and verify it fails**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts
```

Expected: the new nav test fails because `AISectionNav` does not exist yet.
The internal MCP ecosystem test should also fail because the detailed
Observability MCP and Eval MCP panels do not exist yet.

## Task 2: Add the AI section anchor navigation

**Files:**
- Create: `frontend/src/components/ai/AISectionNav.tsx`
- Modify: `frontend/src/app/ai/page.tsx`
- Test: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Create `AISectionNav.tsx`**

Create `frontend/src/components/ai/AISectionNav.tsx` with this content:

```tsx
const aiSectionLinks = [
  { href: "#mcp-server", label: "MCP Server" },
  { href: "#internal-mcps", label: "Internal MCPs" },
  { href: "#rag-evaluation", label: "RAG Evaluation" },
  { href: "#debugging-assistant", label: "Debugging Assistant" },
  { href: "#connect-client", label: "Connect a Client" },
];

export function AISectionNav() {
  return (
    <nav
      aria-label="AI section navigation"
      className="mt-8 rounded-lg border border-foreground/10 bg-card p-3"
    >
      <div className="flex flex-wrap gap-2">
        {aiSectionLinks.map((link) => (
          <a
            key={link.href}
            href={link.href}
            className="rounded-md border border-foreground/10 px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-primary/60 hover:text-foreground"
          >
            {link.label}
          </a>
        ))}
      </div>
    </nav>
  );
}
```

- [ ] **Step 2: Render the nav and add page-level anchors**

In `frontend/src/app/ai/page.tsx`, add this import:

```tsx
import { AISectionNav } from "@/components/ai/AISectionNav";
```

Render the nav after the bio section and before `<MCPSection />`:

```tsx
        <AISectionNav />

        {/* MCP Server (top section) */}
        <MCPSection />
```

Change the RAG Evaluation section opening tag from:

```tsx
        <section className="mt-16">
```

to:

```tsx
        <section id="rag-evaluation" className="mt-16 scroll-mt-20">
```

Change the Debug Assistant section opening tag from:

```tsx
        <section className="mt-16">
          <h2 className="text-2xl font-semibold">Debug Assistant</h2>
```

to:

```tsx
        <section id="debugging-assistant" className="mt-16 scroll-mt-20">
          <h2 className="text-2xl font-semibold">Debugging Assistant</h2>
```

- [ ] **Step 3: Run the targeted e2e test**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts
```

Expected: the nav assertions pass. The internal MCP ecosystem assertions still
fail until Task 3 is implemented.

## Task 3: Replace the internal MCP block with featured ecosystem panels

**Files:**
- Modify: `frontend/src/components/ai/MCPSection.tsx`
- Test: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Replace the internal MCP chart constant with tool/proof arrays**

In `frontend/src/components/ai/MCPSection.tsx`, delete the
`internalMcpChart` constant and add these constants below `githubMcpUrl`:

```tsx
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
```

- [ ] **Step 2: Add the top MCP and connect-client anchors**

Change the top-level return section in `MCPSection` from:

```tsx
    <section className="mt-12">
```

to:

```tsx
    <section id="mcp-server" className="mt-12 scroll-mt-20">
```

Replace the "Connect your own client" heading and its following paragraph with
a nested anchored section that keeps the existing content:

```tsx
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
```

Keep the existing Claude Desktop, Codex CLI, MCP Inspector, and GitHub link
blocks immediately after the new `connect-client` section.

- [ ] **Step 3: Replace the current `#internal-mcps` contents**

In `frontend/src/components/ai/MCPSection.tsx`, replace the full current
`<section id="internal-mcps" ...>...</section>` block with this block:

```tsx
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
              A local MCP endpoint over Prometheus, Loki, Jaeger, Grafana
              gateway mode, and embedded runbooks. It turns operational
              questions into bounded evidence bundles for system health,
              checkout incidents, AI pipeline failures, eval runs, streaming
              analytics, service logs, and trace lookup without mutating the
              cluster.
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
              Eval API datasets, runs, experiments, conclusions, RAG collection
              metadata from ingestion, retrieval settings like top_k, rerank
              comparisons, worst-case review, and the eval-run evidence handoff
              into Observability MCP.
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
```

- [ ] **Step 4: Remove the now-unused Mermaid import**

If `MCPSection.tsx` no longer uses `MermaidDiagram`, remove this import:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";
```

- [ ] **Step 5: Run the targeted e2e test**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts
```

Expected: all tests in `ai-mcp-section.spec.ts` pass.

## Task 4: Run frontend preflights and commit

**Files:**
- Commit: `frontend/src/components/ai/AISectionNav.tsx`
- Commit: `frontend/src/app/ai/page.tsx`
- Commit: `frontend/src/components/ai/MCPSection.tsx`
- Commit: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Run required frontend preflight**

Run from the repo root:

```bash
make preflight-frontend
```

Expected: command exits 0. Existing unrelated lint warnings may appear if they
remain warnings and the command exits 0.

- [ ] **Step 2: Run required e2e preflight**

Run from the repo root:

```bash
make preflight-e2e
```

Expected: command exits 0.

- [ ] **Step 3: Review the final diff**

Run from the repo root:

```bash
git diff -- frontend/src/components/ai/AISectionNav.tsx frontend/src/app/ai/page.tsx frontend/src/components/ai/MCPSection.tsx frontend/e2e/mocked/ai-mcp-section.spec.ts
git status --short
```

Expected: only the four frontend files above are modified or added for the
implementation. The spec and this plan may also be present if they were not
already committed.

- [ ] **Step 4: Commit implementation**

Run from the repo root:

```bash
git add frontend/src/components/ai/AISectionNav.tsx frontend/src/app/ai/page.tsx frontend/src/components/ai/MCPSection.tsx frontend/e2e/mocked/ai-mcp-section.spec.ts
git commit -m "feat: expand ai internal mcp hub"
```

Expected: commit succeeds with only the frontend implementation files staged.

- [ ] **Step 5: Push the feature branch and update the PR**

Run from the repo root:

```bash
git push
```

Expected: the existing `feature/internal-mcp-visual-highlight` branch pushes
successfully and updates the open PR to `qa`.
