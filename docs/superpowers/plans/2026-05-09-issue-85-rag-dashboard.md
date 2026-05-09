# Issue 85 RAG Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Dashboard` tab to `/ai/eval` that shows RAGAS score trends, run comparison deltas, and an annotated run change log, while letting new evaluation runs carry notes and baseline metadata.

**Architecture:** Keep the backend unchanged and bring the frontend into parity with existing eval-service endpoints. Add typed API helpers in `frontend/src/lib/eval-api.ts`, extend `EvaluateTab`, create a focused `DashboardTab`, and wire run selection through the existing `/ai/eval` page state so dashboard entries can open detailed results.

**Tech Stack:** Next.js 16, React 19, TypeScript strict mode, Tailwind CSS, Recharts through the existing shadcn `ChartContainer`, Playwright mocked frontend tests.

---

## File Structure

- Modify `frontend/src/lib/eval-api.ts`
  - Owns eval API types and fetch helpers.
  - Add backend parity fields: `notes`, `config`, `baseline_eval_id`.
  - Add `getHistory()` and `compareRuns()`.
  - Extend `startEvaluation()` with optional notes and baseline id.

- Modify `frontend/src/components/eval/EvaluateTab.tsx`
  - Owns evaluation creation workflow.
  - Add optional notes and baseline dropdown.
  - Load completed baseline options for the selected dataset and collection.

- Create `frontend/src/components/eval/DashboardTab.tsx`
  - Owns Dashboard tab filters, history loading, Recharts trend chart, comparison panel, and annotated change log.
  - Accepts `onSelectEvaluation(evaluation)` so entries can switch to existing Results detail.

- Modify `frontend/src/app/ai/eval/page.tsx`
  - Add `dashboard` tab id.
  - Keep selected evaluation state at page level.
  - Pass a handler from Dashboard to Results.

- Modify `frontend/src/components/eval/DatasetTab.tsx`
  - Render missing `item_count` defensively.

- Create `frontend/e2e/mocked/eval-dashboard.spec.ts`
  - Mock eval endpoints and assert user-visible behavior.

---

### Task 1: Add Mocked E2E Coverage For The Dashboard Workflow

**Files:**
- Create: `frontend/e2e/mocked/eval-dashboard.spec.ts`

- [ ] **Step 1: Create the failing Playwright spec**

Create `frontend/e2e/mocked/eval-dashboard.spec.ts` with this content:

```ts
import { test, expect } from "./fixtures";

const goUser = {
  userId: "user-1",
  email: "kyle@example.com",
  name: "Kyle",
};

const datasets = [
  { id: "ds-1", name: "rag-baseline", created_at: "2026-05-01T10:00:00Z" },
];

const historyRuns = [
  {
    id: "eval-base-001",
    dataset_id: "ds-1",
    status: "completed",
    collection: "documents",
    aggregate_scores: {
      faithfulness: 0.62,
      answer_relevancy: 0.68,
      context_precision: 0.55,
      context_recall: 0.58,
    },
    results: [
      {
        query: "What is chunking?",
        answer: "Chunking splits documents into retrievable pieces.",
        contexts: ["Chunking creates smaller segments."],
        scores: {
          faithfulness: 0.62,
          answer_relevancy: 0.68,
          context_precision: 0.55,
          context_recall: 0.58,
        },
      },
    ],
    error: null,
    created_at: "2026-05-01T10:00:00Z",
    completed_at: "2026-05-01T10:02:00Z",
    notes: "Baseline run before retrieval tuning",
    config: {
      chat: { top_k: 5, prompt_version: "v1-baseline" },
      collection: { chunk_size: 1000, chunk_overlap: 200 },
    },
    baseline_eval_id: null,
  },
  {
    id: "eval-tuned-002",
    dataset_id: "ds-1",
    status: "completed",
    collection: "documents",
    aggregate_scores: {
      faithfulness: 0.71,
      answer_relevancy: 0.72,
      context_precision: 0.51,
      context_recall: 0.6,
    },
    results: [
      {
        query: "What is chunking?",
        answer: "Chunking splits documents for embedding and retrieval.",
        contexts: ["Chunking creates smaller segments."],
        scores: {
          faithfulness: 0.71,
          answer_relevancy: 0.72,
          context_precision: 0.51,
          context_recall: 0.6,
        },
      },
    ],
    error: null,
    created_at: "2026-05-02T10:00:00Z",
    completed_at: "2026-05-02T10:02:00Z",
    notes: "Increased chunk overlap from 200 to 300",
    config: {
      chat: { top_k: 5, prompt_version: "v1-baseline" },
      collection: { chunk_size: 1000, chunk_overlap: 300 },
    },
    baseline_eval_id: "eval-base-001",
  },
];

async function seedGoAuth(page: import("@playwright/test").Page) {
  await page.addInitScript((user) => {
    localStorage.setItem("go_user", JSON.stringify(user));
  }, goUser);
}

async function mockEvalApi(page: import("@playwright/test").Page) {
  await page.route("**/eval/datasets", async (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ datasets }),
      });
    }
    return route.fulfill({
      status: 201,
      contentType: "application/json",
      body: JSON.stringify({ id: "ds-new" }),
    });
  });

  await page.route("**/eval/evaluations/history?**", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ runs: historyRuns }),
    }),
  );

  await page.route("**/eval/evaluations/compare?**", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        runs: historyRuns,
        deltas: {
          faithfulness: [0, 0.09],
          answer_relevancy: [0, 0.04],
          context_precision: [0, -0.04],
          context_recall: [0, 0.02],
        },
      }),
    }),
  );

  await page.route("**/eval/evaluations", async (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ evaluations: historyRuns }),
      });
    }

    const body = route.request().postDataJSON() as Record<string, unknown>;
    expect(body).toMatchObject({
      dataset_id: "ds-1",
      collection: "documents",
      notes: "Raised overlap for better recall",
      baseline_eval_id: "eval-base-001",
    });

    return route.fulfill({
      status: 202,
      contentType: "application/json",
      body: JSON.stringify({ id: "eval-new-003", status: "running" }),
    });
  });

  await page.route("**/eval/evaluations/eval-new-003", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        ...historyRuns[1],
        id: "eval-new-003",
        notes: "Raised overlap for better recall",
        baseline_eval_id: "eval-base-001",
      }),
    }),
  );

  await page.route("**/eval/evaluations/eval-tuned-002", async (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(historyRuns[1]),
    }),
  );
}

test.describe("/ai/eval dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await seedGoAuth(page);
    await mockEvalApi(page);
  });

  test("renders dashboard tab with trends, comparison, and change log", async ({
    page,
  }) => {
    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Dashboard" }).click();

    await expect(page.getByRole("heading", { name: "RAG Improvement Dashboard" })).toBeVisible();
    await expect(page.getByLabel("Dataset")).toHaveValue("ds-1");
    await expect(page.getByLabel("Collection")).toHaveValue("documents");
    await expect(page.getByTestId("rag-score-trend-chart")).toBeVisible();
    await expect(page.getByText("Faithfulness")).toBeVisible();
    await expect(page.getByText("+0.09")).toBeVisible();
    await expect(page.getByText("-0.04")).toBeVisible();
    await expect(page.getByText("Increased chunk overlap from 200 to 300")).toBeVisible();
  });

  test("change log opens the existing Results view for a run", async ({ page }) => {
    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Dashboard" }).click();
    await page.getByRole("button", { name: /View eval-tuned-002 results/ }).click();

    await expect(page.getByRole("button", { name: "Results" })).toHaveClass(/border-b-2/);
    await expect(page.getByText("Aggregate Scores")).toBeVisible();
    await expect(page.getByText("Per-Query Breakdown")).toBeVisible();
  });

  test("evaluate tab sends notes and baseline metadata", async ({ page }) => {
    await page.clock.install();
    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Evaluate" }).click();

    await page.getByLabel("Notes").fill("Raised overlap for better recall");
    await page.getByLabel("Baseline run").selectOption("eval-base-001");
    await page.getByRole("button", { name: "Run Evaluation" }).click();
    await page.clock.runFor(5000);

    await expect(page.getByRole("button", { name: "Results" })).toHaveClass(/border-b-2/);
    await expect(page.getByText("Aggregate Scores")).toBeVisible();
  });
});
```

- [ ] **Step 2: Run the new spec and verify it fails**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts
```

Expected result: FAIL because the `Dashboard` tab and new Evaluate inputs do not exist.

- [ ] **Step 3: Commit the failing test**

```bash
git add frontend/e2e/mocked/eval-dashboard.spec.ts
git commit -m "test: cover rag eval dashboard workflow"
```

---

### Task 2: Update Eval API Types And Fetch Helpers

**Files:**
- Modify: `frontend/src/lib/eval-api.ts`

- [ ] **Step 1: Extend the API types**

In `frontend/src/lib/eval-api.ts`, update the existing type section to include these additions:

```ts
export type EvalConfigSnapshot = Record<string, unknown>;

export type EvaluationSummary = {
  id: string;
  dataset_id: string;
  status: "running" | "completed" | "failed";
  collection: string | null;
  aggregate_scores: QueryScore | null;
  created_at: string;
  completed_at: string | null;
  notes: string | null;
  config: EvalConfigSnapshot | null;
  baseline_eval_id: string | null;
};

export type EvaluationDetail = EvaluationSummary & {
  results: QueryResult[] | null;
  error: string | null;
};

export type RunComparison = {
  runs: EvaluationDetail[];
  deltas: Record<keyof QueryScore, number[]>;
};

export type RunHistory = {
  runs: EvaluationDetail[];
};
```

Keep the existing `GoldenItem`, `DatasetSummary`, `QueryScore`, and `QueryResult` definitions. Do not rename the snake_case fields returned by the API.

- [ ] **Step 2: Extend `startEvaluation()`**

Replace the existing function signature and body with:

```ts
export async function startEvaluation(
  datasetId: string,
  collection?: string,
  notes?: string,
  baselineEvalId?: string,
): Promise<{ id: string; status: string }> {
  const body: Record<string, string> = { dataset_id: datasetId };
  if (collection !== undefined) {
    body.collection = collection;
  }
  if (notes !== undefined && notes.trim() !== "") {
    body.notes = notes.trim();
  }
  if (baselineEvalId !== undefined && baselineEvalId !== "") {
    body.baseline_eval_id = baselineEvalId;
  }
  const res = await evalFetch("/evaluations", {
    method: "POST",
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(
      `Failed to start evaluation: ${res.status} ${res.statusText}`,
    );
  }
  return res.json();
}
```

- [ ] **Step 3: Add history and compare helpers**

Add these functions after `listEvaluations()`:

```ts
export async function getHistory(
  datasetId: string,
  collection: string,
): Promise<RunHistory> {
  const params = new URLSearchParams({
    dataset_id: datasetId,
    collection,
  });
  const res = await evalFetch(`/evaluations/history?${params.toString()}`);
  if (!res.ok) {
    throw new Error(
      `Failed to load evaluation history: ${res.status} ${res.statusText}`,
    );
  }
  return res.json();
}

export async function compareRuns(ids: string[]): Promise<RunComparison> {
  const params = new URLSearchParams({ ids: ids.join(",") });
  const res = await evalFetch(`/evaluations/compare?${params.toString()}`);
  if (!res.ok) {
    throw new Error(
      `Failed to compare evaluations: ${res.status} ${res.statusText}`,
    );
  }
  return res.json();
}
```

- [ ] **Step 4: Run typecheck**

Run:

```bash
cd frontend
npx tsc --noEmit
```

Expected result: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/eval-api.ts
git commit -m "feat: add eval history and comparison client"
```

---

### Task 3: Extend EvaluateTab With Notes And Baseline Selection

**Files:**
- Modify: `frontend/src/components/eval/EvaluateTab.tsx`

- [ ] **Step 1: Import new API helpers**

Update the import from `@/lib/eval-api`:

```ts
import {
  DatasetSummary,
  EvaluationDetail,
  getEvaluation,
  getHistory,
  listDatasets,
  startEvaluation,
} from "@/lib/eval-api";
```

- [ ] **Step 2: Add state for notes, baseline, and baseline options**

Add these state values after the existing `collection` state:

```ts
const [notes, setNotes] = useState<string>("");
const [baselineEvalId, setBaselineEvalId] = useState<string>("");
const [baselineRuns, setBaselineRuns] = useState<EvaluationDetail[]>([]);
const [baselineLoading, setBaselineLoading] = useState<boolean>(false);
```

- [ ] **Step 3: Load baseline runs when dataset or collection changes**

Add this effect below the existing dataset-loading `useEffect`:

```ts
useEffect(() => {
  const trimmedCollection = collection.trim();
  if (!selectedDatasetId || !trimmedCollection) {
    setBaselineRuns([]);
    setBaselineEvalId("");
    return;
  }

  let cancelled = false;
  setBaselineLoading(true);
  getHistory(selectedDatasetId, trimmedCollection)
    .then((history) => {
      if (cancelled) return;
      setBaselineRuns(history.runs);
      setBaselineEvalId((current) =>
        history.runs.some((run) => run.id === current) ? current : "",
      );
    })
    .catch(() => {
      if (cancelled) return;
      setBaselineRuns([]);
      setBaselineEvalId("");
    })
    .finally(() => {
      if (!cancelled) setBaselineLoading(false);
    });

  return () => {
    cancelled = true;
  };
}, [selectedDatasetId, collection]);
```

- [ ] **Step 4: Pass notes and baseline to `startEvaluation()`**

Replace the existing `startEvaluation()` call in `handleRun()` with:

```ts
const result = await startEvaluation(
  selectedDatasetId,
  collection.trim() || undefined,
  notes,
  baselineEvalId,
);
```

After `onComplete(detail);`, add:

```ts
setNotes("");
```

- [ ] **Step 5: Add the new form controls**

Insert this JSX between the collection input block and the error block:

```tsx
{/* Notes input */}
<div>
  <label
    htmlFor="eval-notes"
    className="mb-1 block text-sm font-medium text-gray-700"
  >
    Notes <span className="font-normal text-gray-400">(optional)</span>
  </label>
  <textarea
    id="eval-notes"
    value={notes}
    onChange={(e) => setNotes(e.target.value.slice(0, 500))}
    placeholder="What changed since the last run?"
    rows={3}
    disabled={running}
    className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm placeholder:text-gray-400 focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-400"
  />
  <p className="mt-1 text-xs text-gray-500">{notes.length}/500</p>
</div>

{/* Baseline selector */}
<div>
  <label
    htmlFor="baseline-select"
    className="mb-1 block text-sm font-medium text-gray-700"
  >
    Baseline run <span className="font-normal text-gray-400">(optional)</span>
  </label>
  <select
    id="baseline-select"
    value={baselineEvalId}
    onChange={(e) => setBaselineEvalId(e.target.value)}
    disabled={running || baselineLoading || baselineRuns.length === 0}
    className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500 disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-400"
  >
    <option value="">
      {baselineLoading ? "Loading baselines..." : "(none)"}
    </option>
    {baselineRuns.map((run) => (
      <option key={run.id} value={run.id}>
        {new Date(run.created_at).toLocaleString()} - {run.id.slice(0, 8)}
      </option>
    ))}
  </select>
</div>
```

- [ ] **Step 6: Run focused test**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "evaluate tab sends notes"
```

Expected result: PASS after this task and Task 2 are complete.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/eval/EvaluateTab.tsx
git commit -m "feat: annotate eval runs from evaluate tab"
```

---

### Task 4: Build DashboardTab

**Files:**
- Create: `frontend/src/components/eval/DashboardTab.tsx`

- [ ] **Step 1: Create the component skeleton and utilities**

Create `frontend/src/components/eval/DashboardTab.tsx` with this top section:

```tsx
"use client";

import { useEffect, useMemo, useState } from "react";
import {
  CartesianGrid,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from "recharts";
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import {
  DatasetSummary,
  EvaluationDetail,
  QueryScore,
  compareRuns,
  getHistory,
  listDatasets,
} from "@/lib/eval-api";

const METRICS = [
  ["faithfulness", "Faithfulness"],
  ["answer_relevancy", "Relevancy"],
  ["context_precision", "Precision"],
  ["context_recall", "Recall"],
] as const;

type MetricKey = (typeof METRICS)[number][0];

const chartConfig = {
  faithfulness: { label: "Faithfulness", color: "#2563eb" },
  answer_relevancy: { label: "Relevancy", color: "#16a34a" },
  context_precision: { label: "Precision", color: "#ca8a04" },
  context_recall: { label: "Recall", color: "#dc2626" },
} satisfies ChartConfig;

interface DashboardTabProps {
  onSelectEvaluation: (evaluation: EvaluationDetail) => void;
}

function shortId(id: string): string {
  return id.slice(0, 12);
}

function formatScore(value: number | null | undefined): string {
  return typeof value === "number" ? value.toFixed(2) : "N/A";
}

function formatDelta(value: number | null | undefined): string {
  if (typeof value !== "number") return "0.00";
  const rounded = value.toFixed(2);
  return value > 0 ? `+${rounded}` : rounded;
}

function deltaClass(value: number | null | undefined): string {
  if (typeof value !== "number" || Math.abs(value) < 0.005) {
    return "text-gray-500";
  }
  return value > 0 ? "text-green-600" : "text-red-600";
}

function averageScore(scores: QueryScore | null): number | null {
  if (!scores) return null;
  const values = METRICS.map(([key]) => scores[key]).filter(
    (value): value is number => typeof value === "number",
  );
  if (values.length === 0) return null;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}
```

- [ ] **Step 2: Add state and data loading**

Inside `export function DashboardTab({ onSelectEvaluation }: DashboardTabProps)`, add:

```tsx
export function DashboardTab({ onSelectEvaluation }: DashboardTabProps) {
const [datasets, setDatasets] = useState<DatasetSummary[]>([]);
const [selectedDatasetId, setSelectedDatasetId] = useState("");
const [collection, setCollection] = useState("documents");
const [runs, setRuns] = useState<EvaluationDetail[]>([]);
const [baselineId, setBaselineId] = useState("");
const [candidateId, setCandidateId] = useState("");
const [deltas, setDeltas] = useState<Record<MetricKey, number[]> | null>(null);
const [loading, setLoading] = useState(false);
const [error, setError] = useState("");
const [compareError, setCompareError] = useState("");

useEffect(() => {
  listDatasets()
    .then((data) => {
      setDatasets(data);
      if (data.length > 0) setSelectedDatasetId(data[0].id);
    })
    .catch(() => setError("Failed to load datasets."));
}, []);

useEffect(() => {
  const trimmedCollection = collection.trim();
  if (!selectedDatasetId || !trimmedCollection) {
    setRuns([]);
    setBaselineId("");
    setCandidateId("");
    setDeltas(null);
    return;
  }

  let cancelled = false;
  setLoading(true);
  setError("");
  setCompareError("");
  getHistory(selectedDatasetId, trimmedCollection)
    .then((history) => {
      if (cancelled) return;
      setRuns(history.runs);
      setBaselineId(history.runs[0]?.id ?? "");
      setCandidateId(history.runs[history.runs.length - 1]?.id ?? "");
      setDeltas(null);
    })
    .catch(() => {
      if (cancelled) return;
      setRuns([]);
      setError("Failed to load evaluation history.");
    })
    .finally(() => {
      if (!cancelled) setLoading(false);
    });

  return () => {
    cancelled = true;
  };
}, [selectedDatasetId, collection]);

useEffect(() => {
  if (!baselineId || !candidateId || baselineId === candidateId) {
    setDeltas(null);
    return;
  }

  let cancelled = false;
  setCompareError("");
  compareRuns([baselineId, candidateId])
    .then((comparison) => {
      if (cancelled) return;
      setDeltas(comparison.deltas as Record<MetricKey, number[]>);
    })
    .catch(() => {
      if (cancelled) return;
      setDeltas(null);
      setCompareError("Failed to compare selected runs.");
    });

  return () => {
    cancelled = true;
  };
}, [baselineId, candidateId]);
```

- [ ] **Step 3: Add derived chart data**

Still inside the component, before `return`, add:

```tsx
const chartData = useMemo(
  () =>
    runs.map((run) => ({
      id: run.id,
      createdAt: new Date(run.created_at).toLocaleDateString(),
      faithfulness: run.aggregate_scores?.faithfulness ?? null,
      answer_relevancy: run.aggregate_scores?.answer_relevancy ?? null,
      context_precision: run.aggregate_scores?.context_precision ?? null,
      context_recall: run.aggregate_scores?.context_recall ?? null,
    })),
  [runs],
);

const baselineRun = runs.find((run) => run.id === baselineId) ?? null;
const candidateRun = runs.find((run) => run.id === candidateId) ?? null;
```

- [ ] **Step 4: Add the JSX**

Use this return body:

```tsx
return (
  <div className="space-y-6">
    <div>
      <h2 className="text-xl font-semibold text-gray-900">
        RAG Improvement Dashboard
      </h2>
      <p className="mt-1 text-sm text-gray-600">
        Track RAGAS scores over time, compare runs, and connect changes to
        measured quality impact.
      </p>
    </div>

    <div className="grid gap-4 md:grid-cols-2">
      <div>
        <label
          htmlFor="dashboard-dataset"
          className="mb-1 block text-sm font-medium text-gray-700"
        >
          Dataset
        </label>
        <select
          id="dashboard-dataset"
          value={selectedDatasetId}
          onChange={(event) => setSelectedDatasetId(event.target.value)}
          className="block w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
        >
          {datasets.length === 0 && <option value="">No datasets available</option>}
          {datasets.map((dataset) => (
            <option key={dataset.id} value={dataset.id}>
              {dataset.name}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label
          htmlFor="dashboard-collection"
          className="mb-1 block text-sm font-medium text-gray-700"
        >
          Collection
        </label>
        <input
          id="dashboard-collection"
          value={collection}
          onChange={(event) => setCollection(event.target.value)}
          className="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm shadow-sm focus:border-indigo-500 focus:outline-none focus:ring-1 focus:ring-indigo-500"
        />
      </div>
    </div>

    {error && <p className="text-sm text-red-600">{error}</p>}
    {loading && <p className="text-sm text-gray-600">Loading evaluation history...</p>}
    {!loading && !error && selectedDatasetId && !collection.trim() && (
      <p className="text-sm text-gray-600">Enter a collection to load history.</p>
    )}
    {!loading && !error && collection.trim() && runs.length === 0 && (
      <p className="text-sm text-gray-600">
        No completed runs exist for this dataset and collection.
      </p>
    )}

    {runs.length > 0 && (
      <>
        <section
          className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm"
          data-testid="rag-score-trend-chart"
        >
          <h3 className="mb-4 text-lg font-semibold text-gray-900">
            Score Trends
          </h3>
          <ChartContainer config={chartConfig} className="min-h-[280px] w-full">
            <LineChart data={chartData} margin={{ left: 12, right: 12 }}>
              <CartesianGrid vertical={false} />
              <XAxis dataKey="createdAt" tickLine={false} axisLine={false} />
              <YAxis domain={[0, 1]} tickLine={false} axisLine={false} />
              <ChartTooltip content={<ChartTooltipContent />} />
              {METRICS.map(([key]) => (
                <Line
                  key={key}
                  type="monotone"
                  dataKey={key}
                  stroke={`var(--color-${key})`}
                  strokeWidth={2}
                  dot
                  connectNulls={false}
                />
              ))}
            </LineChart>
          </ChartContainer>
        </section>

        <div className="grid gap-6 lg:grid-cols-2">
          <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
            <h3 className="mb-4 text-lg font-semibold text-gray-900">
              Run Comparison
            </h3>
            {runs.length < 2 ? (
              <p className="text-sm text-gray-600">
                At least two completed runs are needed for comparison.
              </p>
            ) : (
              <div className="space-y-4">
                <div className="grid gap-3 sm:grid-cols-2">
                  <select
                    aria-label="Baseline comparison run"
                    value={baselineId}
                    onChange={(event) => setBaselineId(event.target.value)}
                    className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm"
                  >
                    {runs.map((run) => (
                      <option key={run.id} value={run.id}>
                        {shortId(run.id)}
                      </option>
                    ))}
                  </select>
                  <select
                    aria-label="Candidate comparison run"
                    value={candidateId}
                    onChange={(event) => setCandidateId(event.target.value)}
                    className="rounded-md border border-gray-300 bg-white px-3 py-2 text-sm"
                  >
                    {runs.map((run) => (
                      <option key={run.id} value={run.id}>
                        {shortId(run.id)}
                      </option>
                    ))}
                  </select>
                </div>
                {compareError && <p className="text-sm text-red-600">{compareError}</p>}
                <div className="space-y-2">
                  {METRICS.map(([key, label]) => {
                    const base = baselineRun?.aggregate_scores?.[key] ?? null;
                    const candidate = candidateRun?.aggregate_scores?.[key] ?? null;
                    const delta = deltas?.[key]?.[1] ?? (
                      typeof base === "number" && typeof candidate === "number"
                        ? candidate - base
                        : 0
                    );
                    return (
                      <div
                        key={key}
                        className="grid grid-cols-4 items-center gap-2 rounded-md border border-gray-100 px-3 py-2 text-sm"
                      >
                        <span className="font-medium text-gray-700">{label}</span>
                        <span>{formatScore(base)}</span>
                        <span>{formatScore(candidate)}</span>
                        <span className={`font-semibold ${deltaClass(delta)}`}>
                          {formatDelta(delta)}
                        </span>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </section>

          <section className="rounded-lg border border-gray-200 bg-white p-6 shadow-sm">
            <h3 className="mb-4 text-lg font-semibold text-gray-900">
              Annotated Change Log
            </h3>
            <div className="space-y-3">
              {runs.map((run) => {
                const avg = averageScore(run.aggregate_scores);
                return (
                  <article
                    key={run.id}
                    className="rounded-md border border-gray-100 p-3"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div>
                        <p className="text-sm font-medium text-gray-900">
                          {shortId(run.id)}
                        </p>
                        <p className="text-xs text-gray-500">
                          {new Date(run.created_at).toLocaleString()} - {run.collection}
                        </p>
                      </div>
                      <span className="text-sm font-semibold text-gray-700">
                        {formatScore(avg)}
                      </span>
                    </div>
                    {run.notes && (
                      <p className="mt-2 text-sm text-gray-700">{run.notes}</p>
                    )}
                    {run.config && (
                      <p className="mt-2 text-xs text-gray-500">
                        Config snapshot captured
                      </p>
                    )}
                    <button
                      type="button"
                      onClick={() => onSelectEvaluation(run)}
                      className="mt-3 text-sm font-medium text-indigo-600 hover:text-indigo-700"
                    >
                      View {run.id} results
                    </button>
                  </article>
                );
              })}
            </div>
          </section>
        </div>
      </>
    )}
  </div>
);
}
```

- [ ] **Step 5: Run typecheck**

Run:

```bash
cd frontend
npx tsc --noEmit
```

Expected result: PASS. Fix any strict-mode errors in `DashboardTab.tsx` before committing.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/eval/DashboardTab.tsx
git commit -m "feat: add rag evaluation dashboard tab content"
```

---

### Task 5: Wire Dashboard Into The Eval Page And Results Flow

**Files:**
- Modify: `frontend/src/app/ai/eval/page.tsx`

- [ ] **Step 1: Import `DashboardTab`**

Add:

```ts
import { DashboardTab } from "@/components/eval/DashboardTab";
```

- [ ] **Step 2: Extend tab types and labels**

Replace:

```ts
type TabId = "datasets" | "evaluate" | "results";
```

with:

```ts
type TabId = "datasets" | "evaluate" | "results" | "dashboard";
```

Add this entry to `TABS` after `Results`:

```ts
{ id: "dashboard", label: "Dashboard" },
```

- [ ] **Step 3: Add a dashboard selection handler**

Add this function below `handleEvalComplete`:

```ts
function handleDashboardSelect(evaluation: EvaluationDetail) {
  setCompletedEval(evaluation);
  setActiveTab("results");
}
```

- [ ] **Step 4: Render the Dashboard tab**

Add this JSX after the Results tab render block:

```tsx
{activeTab === "dashboard" && (
  <DashboardTab onSelectEvaluation={handleDashboardSelect} />
)}
```

- [ ] **Step 5: Run the dashboard tests**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts -g "renders dashboard tab|change log opens"
```

Expected result: PASS after Task 4 is complete.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/app/ai/eval/page.tsx
git commit -m "feat: wire rag dashboard into eval page"
```

---

### Task 6: Render Dataset Item Counts Defensively

**Files:**
- Modify: `frontend/src/lib/eval-api.ts`
- Modify: `frontend/src/components/eval/DatasetTab.tsx`
- Modify: `frontend/src/components/eval/EvaluateTab.tsx`

- [ ] **Step 1: Make `item_count` optional**

In `frontend/src/lib/eval-api.ts`, change `DatasetSummary` to:

```ts
export type DatasetSummary = {
  id: string;
  name: string;
  item_count?: number;
  created_at: string;
};
```

- [ ] **Step 2: Update DatasetTab display**

In `frontend/src/components/eval/DatasetTab.tsx`, replace:

```tsx
<span className="ml-2 text-xs text-muted-foreground">
  {ds.item_count} item{ds.item_count !== 1 ? "s" : ""}
</span>
```

with:

```tsx
{typeof ds.item_count === "number" && (
  <span className="ml-2 text-xs text-muted-foreground">
    {ds.item_count} item{ds.item_count !== 1 ? "s" : ""}
  </span>
)}
```

- [ ] **Step 3: Update EvaluateTab option display**

In `frontend/src/components/eval/EvaluateTab.tsx`, replace:

```tsx
{ds.name} ({ds.item_count} items)
```

with:

```tsx
{ds.name}
{typeof ds.item_count === "number" ? ` (${ds.item_count} items)` : ""}
```

- [ ] **Step 4: Run typecheck**

Run:

```bash
cd frontend
npx tsc --noEmit
```

Expected result: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/eval-api.ts frontend/src/components/eval/DatasetTab.tsx frontend/src/components/eval/EvaluateTab.tsx
git commit -m "fix: tolerate eval datasets without item counts"
```

---

### Task 7: Final Verification And Cleanup

**Files:**
- Check: `frontend/e2e/mocked/eval-dashboard.spec.ts`
- Check: `frontend/src/components/eval/DashboardTab.tsx`
- Check: `frontend/src/components/eval/EvaluateTab.tsx`
- Check: `frontend/src/lib/eval-api.ts`

- [ ] **Step 1: Run the focused mocked spec**

Run:

```bash
cd frontend
npx playwright test e2e/mocked/eval-dashboard.spec.ts
```

Expected result: PASS.

- [ ] **Step 2: Run frontend preflight**

Run from repo root:

```bash
make preflight-frontend
```

Expected result: PASS.

- [ ] **Step 3: Run e2e preflight**

Run from repo root:

```bash
make preflight-e2e
```

Expected result: PASS.

- [ ] **Step 4: Check worktree**

Run:

```bash
git status --short
```

Expected result: only intentional files modified, or clean after final commit.

- [ ] **Step 5: Commit final adjustments if verification required fixes**

If Step 1, Step 2, or Step 3 required small fixes, commit them:

```bash
git add frontend/src/lib/eval-api.ts frontend/src/components/eval/DatasetTab.tsx frontend/src/components/eval/EvaluateTab.tsx frontend/src/components/eval/DashboardTab.tsx frontend/src/app/ai/eval/page.tsx frontend/e2e/mocked/eval-dashboard.spec.ts
git commit -m "fix: polish rag dashboard verification"
```

If verification passed without additional edits, do not create an empty commit.
