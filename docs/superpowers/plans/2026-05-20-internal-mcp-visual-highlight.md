# Internal MCP Visual Highlight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the plain internal MCP list on `/ai` with a visually stronger flow-and-transcript section that remains consistent with the shopping assistant presentation.

**Architecture:** Keep the visual highlight inside `MCPSection.tsx` because the content is specific to the `/ai` MCP section and does not need reuse yet. Use the existing `MermaidDiagram` component for the flow diagram and compact Tailwind panels for the transcript and “Why this matters” copy. Extend the existing mocked `/ai` MCP e2e spec to cover the anchor, transcript, and why-it-matters panel.

**Tech Stack:** Next.js, React, TypeScript, Tailwind CSS, Mermaid, Playwright.

---

### Task 1: Visual Highlight Section

**Files:**
- Modify: `frontend/src/components/ai/MCPSection.tsx`
- Test: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Create or enter a feature worktree**

Run from the repo root:

```bash
git fetch origin qa
git worktree add .codex/worktrees/internal-mcp-visual-highlight -b feature/internal-mcp-visual-highlight origin/qa
cd .codex/worktrees/internal-mcp-visual-highlight
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/internal-mcp-visual-highlight
feature/internal-mcp-visual-highlight
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/internal-mcp-visual-highlight
```

- [ ] **Step 2: Write the failing e2e assertions**

In `frontend/e2e/mocked/ai-mcp-section.spec.ts`, update the existing internal MCP test so it asserts the anchor, transcript, and why-it-matters panel:

```ts
  test("MCP section highlights the internal engineering MCP workflow", async ({
    page,
  }) => {
    await page.goto("/ai");

    const section = page.locator("#internal-mcps");
    await expect(section).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "Internal MCPs for Engineering Operations",
      }),
    ).toBeVisible();
    await expect(page.getByText("Observability MCP", { exact: false })).toBeVisible();
    await expect(page.getByText("QA MCP", { exact: false })).toBeVisible();
    await expect(page.getByText("Eval MCP service", { exact: false })).toBeVisible();
    await expect(page.getByText("Evidence-backed engineering action")).toBeVisible();
    await expect(page.getByText("Example tool-call trace")).toBeVisible();
    await expect(
      page.getByText("observability.investigate_checkout"),
    ).toBeVisible();
    await expect(page.getByText("Why this matters")).toBeVisible();
  });
```

- [ ] **Step 3: Run the targeted e2e test to verify it fails**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts
```

Expected: the updated internal MCP test fails because `#internal-mcps`, the transcript labels, and `Evidence-backed engineering action` do not exist yet.

- [ ] **Step 4: Add the Mermaid flow chart**

In `frontend/src/components/ai/MCPSection.tsx`, add this import near the existing imports:

```tsx
import { MermaidDiagram } from "@/components/MermaidDiagram";
```

Then add this chart constant below `githubMcpUrl`:

```tsx
const internalMcpChart = `flowchart LR
  CODEX[Codex]
  subgraph InternalMCPs ["Internal MCPs"]
    OBS[Observability MCP<br/>logs · traces · health]
    QA[QA MCP<br/>answers · critique · weak topics]
    EVAL[Eval MCP service<br/>datasets · runs · comparisons]
  end
  ACTION[Evidence-backed<br/>engineering action]
  CODEX --> InternalMCPs
  InternalMCPs --> ACTION`;
```

- [ ] **Step 5: Replace the plain internal MCP subsection with the visual highlight**

In `frontend/src/components/ai/MCPSection.tsx`, replace the existing `Internal MCPs for Engineering Operations` heading, paragraph, and three-item list with this block:

```tsx
      <section id="internal-mcps" className="mt-10 scroll-mt-20">
        <h3 className="text-xl font-semibold">
          Internal MCPs for Engineering Operations
        </h3>
        <p className="mt-4 text-muted-foreground leading-relaxed">
          The public MCP server is the user-facing showcase. Behind it, three
          internal MCPs give Codex bounded, purpose-built access to the systems
          that keep this portfolio operable: production evidence, structured
          practice feedback, and repeatable RAG quality measurement.
        </p>

        <div className="mt-6 rounded-lg border border-foreground/10 bg-card p-4 sm:p-6">
          <MermaidDiagram chart={internalMcpChart} />
        </div>

        <div className="mt-6 grid gap-4 md:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)]">
          <div className="rounded-lg border border-foreground/10 bg-card p-4">
            <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Example tool-call trace
            </div>
            <div className="mt-4 space-y-3">
              <div className="ml-auto w-fit max-w-[85%] rounded-lg bg-primary px-3 py-2 text-sm text-primary-foreground">
                Why did checkout stall in QA?
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="inline-block size-2 animate-pulse rounded-full bg-yellow-500" />
                observability.investigate_checkout
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="inline-block size-2 rounded-full bg-green-500" />
                observability.get_trace
              </div>
              <div className="mr-auto w-fit max-w-[92%] rounded-lg bg-muted px-3 py-2 text-sm">
                Payment was created, but the order saga never observed
                completion. The trace points to a RabbitMQ reply timeout.
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-foreground/10 bg-card p-4">
            <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Why this matters
            </div>
            <ul className="mt-4 list-disc space-y-2 pl-5 text-sm text-muted-foreground">
              <li>
                Turns production questions into bounded evidence requests.
              </li>
              <li>
                Keeps practice feedback and weak-topic tracking in the same
                workflow.
              </li>
              <li>
                Makes RAG changes measurable before they are treated as
                improvements.
              </li>
            </ul>
          </div>
        </div>
      </section>
```

- [ ] **Step 6: Run the targeted e2e test to verify it passes**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts
```

Expected: all tests in `ai-mcp-section.spec.ts` pass.

- [ ] **Step 7: Run required frontend preflights**

Run from repo root:

```bash
make preflight-frontend
make preflight-e2e
```

Expected: both commands pass. Existing lint warnings may appear if they are unrelated and the command exits 0.

- [ ] **Step 8: Commit the implementation**

Run from the feature worktree:

```bash
git status --short
git add frontend/src/components/ai/MCPSection.tsx frontend/e2e/mocked/ai-mcp-section.spec.ts
git commit -m "feat: highlight internal mcp workflows"
```

Expected: commit succeeds with only the frontend implementation files staged.
