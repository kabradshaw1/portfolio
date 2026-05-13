# RAG Evaluation Workflow Guide Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a production-focused RAG evaluation workflow runbook and expose the same workflow as a discoverable `Guide` tab in `/ai/eval`.

**Architecture:** The runbook is the durable source of truth under `docs/runbooks/`. The frontend gets a focused `EvalGuideTab` component that renders a compact operator checklist and switches the existing `/ai/eval` tab state through callbacks. Existing eval API behavior remains unchanged.

**Tech Stack:** Markdown docs, Next.js 16 App Router, React 19, TypeScript, Tailwind CSS, Playwright mocked e2e tests.

---

## File Structure

- Create `docs/runbooks/rag-evaluation-workflow.md`
  - Production-facing usage workflow for the existing eval service.
  - Covers dataset creation, baseline runs, candidate runs, comparison, per-query inspection, and decisions.

- Create `frontend/src/components/eval/EvalGuideTab.tsx`
  - Pure client component for the `/ai/eval` guide content.
  - Receives `onSelectTab(tab: EvalTabId)` and uses it for action buttons.
  - Does not call APIs or own eval state.

- Modify `frontend/src/app/ai/eval/page.tsx`
  - Export or define shared `EvalTabId` including `"guide"`.
  - Add `Guide` as the first tab and default active tab.
  - Render `EvalGuideTab` when active.
  - Pass a tab-switch callback to the guide.

- Modify `frontend/e2e/mocked/eval-dashboard.spec.ts`
  - Add mocked coverage that `/ai/eval` opens on `Guide`.
  - Assert the guide contains the workflow content.
  - Assert guide action buttons switch to `Datasets`, `Evaluate`, `Results`, and `Dashboard`.

---

### Task 1: Add The Production Runbook

**Files:**
- Create: `docs/runbooks/rag-evaluation-workflow.md`

- [ ] **Step 1: Create the runbook**

Use this exact content:

```markdown
# RAG Evaluation Workflow

Use this workflow in production when measuring whether a RAG change improved
answer quality. The goal is not to produce one perfect score. The goal is to
build a repeatable habit: stable dataset, baseline run, candidate run, score
comparison, per-query review, and a written decision.

## Prerequisites

- You can log in to the production frontend.
- The eval service health gate on `/ai/eval` passes.
- The chat and ingestion services are serving the collection you want to test.
- Use the `documents` collection unless you are deliberately evaluating another
  collection.

## 1. Create Or Select A Golden Dataset

Open `/ai/eval`, then open `Datasets`.

Create a dataset with 8-15 high-signal questions for the first pass. Each item
must include:

- `query`: a realistic user question.
- `expected_answer`: the answer the system should be able to produce from the
  indexed documents.
- `expected_sources`: source names that should support the answer.

Good first datasets include a mix of:

- Easy factual questions that should always pass.
- Multi-source questions that test retrieval coverage.
- Questions that previously failed during manual testing.
- Edge cases where a plausible answer could hallucinate unsupported details.

Once a dataset becomes a baseline set, avoid changing its meaning. If the test
coverage needs to change materially, create a new dataset version with a clear
name such as `product-docs-rag-v2`.

## 2. Run A Baseline

Open `Evaluate`.

Use these settings:

- Dataset: the golden dataset you want to track.
- Collection: `documents`, unless evaluating another collection intentionally.
- Baseline run: leave empty.
- Notes: describe the baseline context, for example
  `Baseline before rerank comparison`.

Start the evaluation and wait for it to complete. The run may take several
minutes because every dataset item calls retrieval, generation, and judge logic.

## 3. Inspect Baseline Results

Open `Results`.

Review aggregate scores first:

- `faithfulness`: whether the answer is supported by retrieved context.
- `answer_relevancy`: whether the answer addresses the question.
- `context_precision`: whether the top retrieved chunks are useful.
- `context_recall`: whether retrieval found enough supporting information.

Then expand low-scoring questions and classify each failure:

- Retrieval miss: the needed context was not retrieved.
- Weak answer: context was present but the generated answer was incomplete.
- Unsupported answer: the model made claims not supported by context.
- Dataset issue: the expected answer or source is wrong or ambiguous.
- Expected-source mismatch: the answer is right, but the source expectation is
  too narrow or stale.

Fix dataset issues before treating the score as a system failure.

## 4. Run A Candidate

Make one deliberate RAG change at a time. Examples:

- Enable or tune reranking.
- Change chunk size or overlap.
- Change prompt version.
- Change retrieval `top_k`.
- Re-index a collection after improving parsing or chunking.

Open `Evaluate` and run the same dataset against the same collection.

Use these settings:

- Baseline run: select the completed baseline run for the same dataset and
  collection.
- Notes: state the exact change, for example
  `Candidate: enabled cross-encoder reranking`.

If the baseline selector rejects a run, use a completed run from the same
dataset and collection.

## 5. Compare Runs

Open `Dashboard`.

Select the same dataset and collection. Compare the baseline run to the
candidate run.

Read metric deltas as directional evidence:

- Positive faithfulness delta suggests fewer unsupported claims.
- Positive answer relevancy delta suggests answers better address the query.
- Positive context precision delta suggests better ranking.
- Positive context recall delta suggests retrieval found more required support.

Do not accept a change from aggregate deltas alone. Inspect per-query results,
especially when one metric improves and another regresses.

## 6. Decide

Keep the change when:

- Aggregate scores improve or remain stable in the metrics the change targeted.
- Important per-query results do not regress.
- The result makes sense after reading retrieved contexts and answers.

Revert or adjust the change when:

- Improvements are tiny and likely noisy.
- A high-value query regresses.
- Retrieval metrics improve but answers become less faithful.
- The dataset was too weak to support a clear decision.

When the next improvement requires code or UI work, create a focused follow-up
issue with the baseline run, candidate run, metric deltas, and failure examples.

## 7. Repeat

Build history gradually. For now, evaluation notes and dashboard history are the
lightweight experiment log. After several real measurement cycles, use that
experience to decide what the experiment ledger should store.
```

- [ ] **Step 2: Review the runbook for production wording**

Run:

```bash
sed -n '1,260p' docs/runbooks/rag-evaluation-workflow.md
```

Expected: The document is production-focused, refers to `/ai/eval`, and does not require unbuilt endpoints or issue #240/#241.

- [ ] **Step 3: Commit the runbook**

```bash
git add docs/runbooks/rag-evaluation-workflow.md
git commit -m "docs: add rag evaluation workflow runbook"
```

---

### Task 2: Add The Eval Guide Component

**Files:**
- Create: `frontend/src/components/eval/EvalGuideTab.tsx`

- [ ] **Step 1: Write the component**

Create `frontend/src/components/eval/EvalGuideTab.tsx` with:

```tsx
"use client";

export type EvalGuideTarget = "datasets" | "evaluate" | "results" | "dashboard";

type GuideStep = {
  title: string;
  body: string;
  actionLabel: string;
  target: EvalGuideTarget;
};

const GUIDE_STEPS: GuideStep[] = [
  {
    title: "Create or select a golden dataset",
    body:
      "Start with 8-15 stable, high-signal questions. Each item should include a realistic query, expected answer, and expected sources.",
    actionLabel: "Create dataset",
    target: "datasets",
  },
  {
    title: "Run a baseline",
    body:
      "Use the same collection you plan to improve, usually documents. Leave Baseline run empty and use notes such as Baseline before rerank comparison.",
    actionLabel: "Run baseline",
    target: "evaluate",
  },
  {
    title: "Inspect low-scoring queries",
    body:
      "Review aggregate scores first, then expand weak queries and classify failures as retrieval misses, weak answers, unsupported claims, or dataset issues.",
    actionLabel: "Review results",
    target: "results",
  },
  {
    title: "Run one candidate change",
    body:
      "Change one RAG variable at a time, then run the same dataset and collection. Select the completed baseline and write notes that name the exact change.",
    actionLabel: "Run candidate",
    target: "evaluate",
  },
  {
    title: "Compare and decide",
    body:
      "Use dashboard deltas as directional evidence, then inspect per-query results before deciding whether to keep, adjust, or revert the change.",
    actionLabel: "Compare runs",
    target: "dashboard",
  },
];

interface EvalGuideTabProps {
  onSelectTab: (tab: EvalGuideTarget) => void;
}

export function EvalGuideTab({ onSelectTab }: EvalGuideTabProps) {
  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h2 className="text-xl font-semibold text-gray-900">
          RAG Evaluation Workflow
        </h2>
        <p className="mt-2 max-w-3xl text-sm text-gray-600">
          Use this page to build a measurement history: stable golden dataset,
          baseline run, candidate run, score comparison, per-query review, and
          a written decision. Treat aggregate scores as signals, not proof, and
          always inspect the queries that moved.
        </p>
      </section>

      <section
        aria-label="Evaluation workflow steps"
        className="grid gap-4 md:grid-cols-2"
      >
        {GUIDE_STEPS.map((step, index) => (
          <article
            key={step.title}
            className="rounded-lg border border-gray-200 bg-white p-5 shadow-sm"
          >
            <div className="flex items-start gap-3">
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-indigo-50 text-sm font-semibold text-indigo-700">
                {index + 1}
              </span>
              <div className="min-w-0">
                <h3 className="text-base font-semibold text-gray-900">
                  {step.title}
                </h3>
                <p className="mt-2 text-sm leading-6 text-gray-600">
                  {step.body}
                </p>
                <button
                  type="button"
                  onClick={() => onSelectTab(step.target)}
                  className="mt-4 rounded-md border border-indigo-200 px-3 py-2 text-sm font-medium text-indigo-700 transition-colors hover:border-indigo-300 hover:bg-indigo-50 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2"
                >
                  {step.actionLabel}
                </button>
              </div>
            </div>
          </article>
        ))}
      </section>

      <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
        <h3 className="text-lg font-semibold text-gray-900">
          Decision checklist
        </h3>
        <div className="mt-4 grid gap-3 text-sm text-gray-700 md:grid-cols-3">
          <p className="rounded-md border border-gray-100 p-3">
            Keep changes when targeted metrics improve and important queries do
            not regress.
          </p>
          <p className="rounded-md border border-gray-100 p-3">
            Adjust changes when aggregate gains are narrow, noisy, or conflict
            with per-query evidence.
          </p>
          <p className="rounded-md border border-gray-100 p-3">
            Fix dataset issues before treating a bad score as a RAG pipeline
            failure.
          </p>
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 2: Run TypeScript check for the new component**

Run:

```bash
cd frontend && npm run lint -- src/components/eval/EvalGuideTab.tsx
```

Expected: PASS with no ESLint errors for the new component.

- [ ] **Step 3: Commit the guide component**

```bash
git add frontend/src/components/eval/EvalGuideTab.tsx
git commit -m "feat: add rag eval guide component"
```

---

### Task 3: Wire The Guide Into `/ai/eval`

**Files:**
- Modify: `frontend/src/app/ai/eval/page.tsx`

- [ ] **Step 1: Update imports and tab types**

Modify the imports and tab type near the top of `frontend/src/app/ai/eval/page.tsx` to include the guide:

```tsx
import { EvalGuideTab } from "@/components/eval/EvalGuideTab";
import { DashboardTab } from "@/components/eval/DashboardTab";
import { DatasetTab } from "@/components/eval/DatasetTab";
import EvaluateTab from "@/components/eval/EvaluateTab";
import ResultsTab from "@/components/eval/ResultsTab";
import { EvaluationDetail } from "@/lib/eval-api";

type TabId = "guide" | "datasets" | "evaluate" | "results" | "dashboard";

const TABS: { id: TabId; label: string }[] = [
  { id: "guide", label: "Guide" },
  { id: "datasets", label: "Datasets" },
  { id: "evaluate", label: "Evaluate" },
  { id: "results", label: "Results" },
  { id: "dashboard", label: "Dashboard" },
];
```

- [ ] **Step 2: Make Guide the default active tab**

Change the active tab state initialization in `EvalPageInner`:

```tsx
const [activeTab, setActiveTab] = useState<TabId>("guide");
```

- [ ] **Step 3: Render the guide tab**

Add guide rendering before the existing tab content:

```tsx
{/* Tab content */}
{activeTab === "guide" && <EvalGuideTab onSelectTab={setActiveTab} />}
{activeTab === "datasets" && <DatasetTab />}
{activeTab === "evaluate" && (
  <EvaluateTab onComplete={handleEvalComplete} />
)}
{activeTab === "results" && (
  <ResultsTab selectedEvaluation={completedEval} />
)}
{activeTab === "dashboard" && (
  <DashboardTab onSelectEvaluation={handleDashboardSelect} />
)}
```

- [ ] **Step 4: Run targeted lint**

Run:

```bash
cd frontend && npm run lint -- src/app/ai/eval/page.tsx src/components/eval/EvalGuideTab.tsx
```

Expected: PASS with no ESLint errors.

- [ ] **Step 5: Commit the page wiring**

```bash
git add frontend/src/app/ai/eval/page.tsx frontend/src/components/eval/EvalGuideTab.tsx
git commit -m "feat: show rag eval workflow guide"
```

---

### Task 4: Add Mocked E2E Coverage

**Files:**
- Modify: `frontend/e2e/mocked/eval-dashboard.spec.ts`

- [ ] **Step 1: Add guide tab assertions**

Inside `test.describe("/ai/eval dashboard", () => { ... })`, add this test before the existing dashboard test:

```ts
test("opens on guide tab and links to eval workflow tabs", async ({ page }) => {
  await page.goto("/ai/eval");

  await expect(page.getByRole("button", { name: "Guide" })).toHaveClass(
    /border-b-2/,
  );
  await expect(
    page.getByRole("heading", { name: "RAG Evaluation Workflow" }),
  ).toBeVisible();
  await expect(
    page.getByText("stable golden dataset, baseline run, candidate run"),
  ).toBeVisible();
  await expect(page.getByText("Decision checklist")).toBeVisible();

  await page.getByRole("button", { name: "Create dataset" }).click();
  await expect(page.getByRole("heading", { name: "Create a Golden Dataset" }))
    .toBeVisible();

  await page.getByRole("button", { name: "Guide" }).click();
  await page.getByRole("button", { name: "Run baseline" }).click();
  await expect(page.getByRole("heading", { name: "Run Evaluation" }))
    .toBeVisible();

  await page.getByRole("button", { name: "Guide" }).click();
  await page.getByRole("button", { name: "Review results" }).click();
  await expect(page.getByLabel("Evaluation")).toBeVisible();

  await page.getByRole("button", { name: "Guide" }).click();
  await page.getByRole("button", { name: "Compare runs" }).click();
  await expect(
    page.getByRole("heading", { name: "RAG Improvement Dashboard" }),
  ).toBeVisible();
});
```

- [ ] **Step 2: Run the targeted mocked e2e test file**

Run:

```bash
cd frontend && npm run e2e -- e2e/mocked/eval-dashboard.spec.ts
```

Expected: PASS for all tests in `eval-dashboard.spec.ts`.

- [ ] **Step 3: Commit the e2e coverage**

```bash
git add frontend/e2e/mocked/eval-dashboard.spec.ts
git commit -m "test: cover rag eval guide tab"
```

---

### Task 5: Run Required Preflights

**Files:**
- No file edits expected.

- [ ] **Step 1: Run frontend preflight**

Run from repo root:

```bash
make preflight-frontend
```

Expected: PASS. If it fails, fix only failures related to this change.

- [ ] **Step 2: Run e2e preflight**

Run from repo root:

```bash
make preflight-e2e
```

Expected: PASS. If it fails, fix only failures related to this change.

- [ ] **Step 3: Check git status**

Run:

```bash
git status --short
```

Expected: clean working tree after the previous commits, or only intentional changes that need one final commit.

---

### Task 6: Final Review And PR Prep

**Files:**
- Review only unless preflight fixes are needed.

- [ ] **Step 1: Review final diff**

Run:

```bash
git log --oneline -n 6
git diff origin/qa...HEAD -- docs/runbooks/rag-evaluation-workflow.md frontend/src/app/ai/eval/page.tsx frontend/src/components/eval/EvalGuideTab.tsx frontend/e2e/mocked/eval-dashboard.spec.ts
```

Expected: Diff includes the runbook, guide tab wiring, guide component, and e2e test only.

- [ ] **Step 2: Push feature branch**

Run:

```bash
git branch --show-current
git push -u origin HEAD
```

Expected: Branch pushes successfully. Do not push directly from `qa`; execute this plan from a feature worktree.

- [ ] **Step 3: Create PR to `qa`**

Run:

```bash
gh pr create --base qa --head "$(git branch --show-current)" --title "Add RAG evaluation workflow guide" --body "## Summary
- add a production runbook for using the RAG eval service
- add a Guide tab to /ai/eval with workflow actions
- cover guide tab navigation in mocked Playwright tests

## Verification
- make preflight-frontend
- make preflight-e2e"
```

Expected: PR is created targeting `qa`. Do not watch CI unless explicitly requested.
