# Internal MCP Narrative Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a concise narrative subsection to the `/ai` MCP showcase describing the Observability MCP, QA MCP, and Eval MCP service.

**Architecture:** Keep the existing public MCP server section intact. Add one internal-operations subsection in `MCPSection.tsx` after the public tool catalog and before the interactive demo CTA, then extend the existing mocked e2e coverage for `/ai`.

**Tech Stack:** Next.js, React, TypeScript, Playwright, shadcn/Tailwind styling.

---

### Task 1: Add Internal MCP Narrative

**Files:**
- Modify: `frontend/src/components/ai/MCPSection.tsx`
- Test: `frontend/e2e/mocked/ai-mcp-section.spec.ts`

- [ ] **Step 1: Create or enter a feature worktree**

Run from the repo root:

```bash
git branch --show-current
git worktree add .codex/worktrees/internal-mcp-narrative -b feature/internal-mcp-narrative
cd .codex/worktrees/internal-mcp-narrative
pwd
git branch --show-current
git rev-parse --show-toplevel
```

Expected:

```text
feature/internal-mcp-narrative
/Users/kylebradshaw/repos/gen_ai_engineer/.codex/worktrees/internal-mcp-narrative
```

- [ ] **Step 2: Write the failing e2e assertion**

In `frontend/e2e/mocked/ai-mcp-section.spec.ts`, add this test:

```ts
test("MCP section describes the internal engineering MCPs", async ({ page }) => {
  await page.goto("/ai");

  await expect(
    page.getByRole("heading", {
      name: "Internal MCPs for Engineering Operations",
    }),
  ).toBeVisible();
  await expect(page.getByText("Observability MCP", { exact: false })).toBeVisible();
  await expect(page.getByText("QA MCP", { exact: false })).toBeVisible();
  await expect(page.getByText("Eval MCP service", { exact: false })).toBeVisible();
});
```

- [ ] **Step 3: Run the targeted e2e test to verify it fails**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts --project=chromium
```

Expected: the new test fails because the heading does not exist yet.

- [ ] **Step 4: Add the narrative subsection**

In `frontend/src/components/ai/MCPSection.tsx`, insert this block immediately after `<MCPToolCatalog />`:

```tsx
      <h3 className="mt-10 text-xl font-semibold">
        Internal MCPs for Engineering Operations
      </h3>
      <p className="mt-4 text-muted-foreground leading-relaxed">
        The public MCP server is the user-facing showcase. Behind it, three
        internal MCPs give Codex bounded, purpose-built access to the systems
        that keep this portfolio operable: production evidence, structured
        practice feedback, and repeatable RAG quality measurement.
      </p>
      <ul className="mt-4 list-disc space-y-2 pl-6 text-muted-foreground">
        <li>
          <strong className="text-foreground">Observability MCP:</strong>{" "}
          gathers service health, logs, metrics, and traces into bounded
          evidence bundles for incident triage and recovery verification.
        </li>
        <li>
          <strong className="text-foreground">QA MCP:</strong> manages
          structured interview-practice sessions, weak-topic tracking, answer
          attempts, and feedback against expected answers.
        </li>
        <li>
          <strong className="text-foreground">Eval MCP service:</strong>{" "}
          manages RAG datasets and evaluation runs, compares candidate changes,
          surfaces worst cases, and helps decide whether retrieval work improved
          quality.
        </li>
      </ul>
```

- [ ] **Step 5: Run the targeted e2e test to verify it passes**

Run from `frontend/`:

```bash
npx playwright test e2e/mocked/ai-mcp-section.spec.ts --project=chromium
```

Expected: all tests in `ai-mcp-section.spec.ts` pass.

- [ ] **Step 6: Run required frontend preflights**

Run from repo root:

```bash
make preflight-frontend
make preflight-e2e
```

Expected: both commands pass. If either command is blocked by local environment issues, capture the failing command and error.

- [ ] **Step 7: Commit the implementation**

Run from the feature worktree:

```bash
git status --short
git add frontend/src/components/ai/MCPSection.tsx frontend/e2e/mocked/ai-mcp-section.spec.ts
git commit -m "feat: describe internal mcp services"
```

Expected: commit succeeds with only the frontend implementation files staged.
