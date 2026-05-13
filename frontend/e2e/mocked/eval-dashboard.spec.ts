import { expect, test } from "./fixtures";
import type { Locator } from "@playwright/test";

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

async function expectReadableWhiteControl(locator: Locator) {
  const colors = await locator.evaluate((element) => ({
    control: getComputedStyle(element).color,
    page: getComputedStyle(document.body).color,
  }));

  expect(colors.control).not.toBe(colors.page);
}

test.describe("/ai/eval dashboard", () => {
  test.beforeEach(async ({ page }) => {
    await seedGoAuth(page);
    await mockEvalApi(page);
  });

  test("opens on guide tab and links to eval workflow tabs", async ({
    page,
  }) => {
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
    await expect(
      page.getByRole("heading", { name: "Create a Golden Dataset" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Guide" }).click();
    await page.getByRole("button", { name: "Run baseline" }).click();
    await expect(
      page.getByRole("heading", { name: "Run Evaluation" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Guide" }).click();
    await page.getByRole("button", { name: "Run candidate" }).click();
    await expect(
      page.getByRole("heading", { name: "Run Evaluation" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Guide" }).click();
    await page.getByRole("button", { name: "Review results" }).click();
    await expect(page.getByLabel("Evaluation")).toBeVisible();

    await page.getByRole("button", { name: "Guide" }).click();
    await page.getByRole("button", { name: "Compare runs" }).click();
    await expect(
      page.getByRole("heading", { name: "RAG Improvement Dashboard" }),
    ).toBeVisible();
  });

  test("renders dashboard tab with trends, comparison, and change log", async ({
    page,
  }) => {
    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Dashboard" }).click();

    await expect(
      page.getByRole("heading", { name: "RAG Improvement Dashboard" }),
    ).toBeVisible();
    await expect(page.getByLabel("Dataset")).toHaveValue("ds-1");
    await expect(page.getByLabel("Collection")).toHaveValue("documents");
    await expect(page.getByTestId("rag-score-trend-chart")).toBeVisible();
    await expect(page.getByText("Faithfulness")).toBeVisible();
    await expect(page.getByText("+0.09")).toBeVisible();
    await expect(page.getByText("-0.04")).toBeVisible();
    await expect(
      page.getByText("Increased chunk overlap from 200 to 300"),
    ).toBeVisible();
  });

  test("change log opens the existing Results view for a run", async ({
    page,
  }) => {
    await page.goto("/ai/eval");
    await page.getByRole("button", { name: "Dashboard" }).click();
    await page.getByRole("button", { name: /View eval-tuned-002 results/ }).click();

    await expect(page.getByRole("button", { name: "Results" })).toHaveClass(
      /border-b-2/,
    );
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

    await expect(page.getByRole("button", { name: "Results" })).toHaveClass(
      /border-b-2/,
    );
    await expect(page.getByText("Aggregate Scores")).toBeVisible();
  });

  test("keeps light form controls readable in the dark document theme", async ({
    page,
  }) => {
    await page.goto("/ai/eval");

    await page.getByRole("button", { name: "Evaluate" }).click();
    await expectReadableWhiteControl(page.getByLabel("Dataset"));
    await expectReadableWhiteControl(page.getByLabel("Collection"));
    await expectReadableWhiteControl(page.getByLabel("Notes"));
    await expectReadableWhiteControl(page.getByLabel("Baseline run"));

    await page.getByRole("button", { name: "Results" }).click();
    await expectReadableWhiteControl(page.getByLabel("Evaluation"));

    await page.getByRole("button", { name: "Dashboard" }).click();
    await expectReadableWhiteControl(page.getByLabel("Dataset"));
    await expectReadableWhiteControl(page.getByLabel("Collection"));
    await expectReadableWhiteControl(page.getByLabel("Baseline comparison run"));
    await expectReadableWhiteControl(page.getByLabel("Candidate comparison run"));
  });
});
