import { test, expect } from "./fixtures";

const projectHealth = {
  stats: {
    taskCountByStatus: { todo: 5, inProgress: 3, done: 12 },
    taskCountByPriority: { low: 4, medium: 8, high: 8 },
    overdueCount: 2,
    avgCompletionTimeHours: 48.5,
    memberWorkload: [
      { userId: "u1", name: "Kyle", assignedCount: 4, completedCount: 7 },
    ],
  },
  velocity: {
    weeklyThroughput: [
      { week: "2026-W14", completed: 5, created: 8 },
      { week: "2026-W13", completed: 3, created: 4 },
    ],
    avgLeadTimeHours: 36.2,
    leadTimePercentiles: { p50: 24, p75: 48, p95: 120 },
  },
  activity: {
    totalEvents: 142,
    eventCountByType: [
      { eventType: "task.created", count: 20 },
      { eventType: "task.status_changed", count: 85 },
    ],
    commentCount: 24,
    activeContributors: 5,
    weeklyActivity: [
      { week: "2026-W14", events: 32, comments: 6 },
      { week: "2026-W13", events: 28, comments: 4 },
    ],
  },
};

const myProjects = [
  { id: "p1", name: "Alpha" },
  { id: "p2", name: "Beta" },
];

test.describe("Java dashboard", () => {
  test.beforeEach(async ({ page }) => {
    // Seed auth token before any app code runs.
    await page.addInitScript(() => {
      localStorage.setItem("java_access_token", "test-access-token");
      localStorage.setItem("java_refresh_token", "test-refresh-token");
    });

    await page.route("**/graphql", async (route) => {
      const request = route.request();
      const postData = request.postDataJSON() as {
        operationName?: string;
        query?: string;
      } | null;
      const opName =
        postData?.operationName ||
        (postData?.query?.match(/query\s+(\w+)/)?.[1] ?? "");

      if (opName === "MyProjects") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: { myProjects } }),
        });
      }
      if (opName === "ProjectHealth") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ data: { projectHealth } }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ data: {} }),
      });
    });
  });

  test("renders analytics sections", async ({ page }) => {
    await page.goto("/java/dashboard");

    await expect(
      page.getByRole("heading", { name: "Project Analytics" })
    ).toBeVisible();
    await expect(page.getByText("By Status")).toBeVisible();
    await expect(page.getByText("By Priority")).toBeVisible();
    await expect(page.getByText("Weekly Throughput")).toBeVisible();
    await expect(page.getByText("Member Workload")).toBeVisible();
    await expect(page.getByText("Events by Type")).toBeVisible();
    await expect(page.getByText("Weekly Activity")).toBeVisible();
  });

  test("switching project updates the URL", async ({ page }) => {
    await page.goto("/java/dashboard?projectId=p1");

    await expect(
      page.getByRole("heading", { name: "Project Analytics" })
    ).toBeVisible();

    const select = page.getByRole("combobox", { name: /select project/i });
    await select.selectOption("p2");

    await expect(page).toHaveURL(/projectId=p2/);
  });
});
