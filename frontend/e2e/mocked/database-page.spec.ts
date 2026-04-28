import { test, expect } from "./fixtures";

test.describe("/database page", () => {
  test("renders the page heading", async ({ page }) => {
    await page.goto("/database");
    await expect(
      page.getByRole("heading", { name: "Database Engineering", level: 1 }),
    ).toBeVisible();
  });

  test("renders all three tab labels", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByRole("button", { name: "PostgreSQL", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "NoSQL", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Vector", exact: true })).toBeVisible();
  });

  test("PostgreSQL tab is active by default", async ({ page }) => {
    await page.goto("/database");
    await expect(page.getByTestId("postgres-tab")).toBeVisible();
    await expect(page.getByTestId("nosql-tab")).not.toBeVisible();
    await expect(page.getByTestId("vector-tab")).not.toBeVisible();
  });

  test("clicking NoSQL switches to the NoSQL tab", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "NoSQL", exact: true }).click();
    await expect(page.getByTestId("nosql-tab")).toBeVisible();
    await expect(page.getByTestId("postgres-tab")).not.toBeVisible();
  });

  test("clicking Vector switches to the Vector tab", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "Vector", exact: true }).click();
    await expect(page.getByTestId("vector-tab")).toBeVisible();
  });

  test("NoSQL stub points to /java", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "NoSQL", exact: true }).click();
    await expect(page.getByText("MongoDB powers the activity feed", { exact: false })).toBeVisible();
    const link = page.getByRole("link", { name: /View MongoDB usage in \/java/ });
    await expect(link).toHaveAttribute("href", "/java");
  });

  test("Vector stub points to /ai", async ({ page }) => {
    await page.goto("/database");
    await page.getByRole("button", { name: "Vector", exact: true }).click();
    await expect(page.getByText("Qdrant backs the retrieval layer", { exact: false })).toBeVisible();
    const link = page.getByRole("link", { name: /View vector DB usage in \/ai/ });
    await expect(link).toHaveAttribute("href", "/ai");
  });

  test("PostgreSQL tab renders all five pillar headings", async ({ page }) => {
    await page.goto("/database");
    await expect(
      page.getByRole("heading", { name: "Query Optimization & Benchmarking", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", {
        name: "Query Observability — pg_stat_statements + auto_explain",
        level: 2,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Connection Pooling — PgBouncer", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Read Replica & Reporting Pool", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Reliability & Backups", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Migration Safety — migration-lint", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Schema Design — Partitioning & Materialized Views", level: 2 }),
    ).toBeVisible();
  });

  test("each pillar has a stable anchor id", async ({ page }) => {
    await page.goto("/database");
    for (const id of [
      "optimization",
      "observability",
      "pooling",
      "replica",
      "reliability",
      "migrations",
      "schema",
    ]) {
      await expect(page.locator(`#${id}`)).toBeVisible();
    }
  });

  test("TOC items render in the new order", async ({ page }) => {
    await page.goto("/database");
    const sidebarLabels = await page
      .locator('[data-testid="sticky-toc-sidebar"] a')
      .allTextContents();
    expect(sidebarLabels.map((s) => s.trim())).toEqual([
      "Query Optimization",
      "Query Observability",
      "Connection Pooling",
      "Read Replica",
      "Reliability & Backups",
      "Migration Safety",
      "Schema Design",
    ]);
  });

  test("optimization pillar contains the moved benchmark table", async ({ page }) => {
    await page.goto("/database");
    const optimization = page.locator("#optimization");
    await expect(optimization.locator("table")).toBeVisible();
    await expect(optimization.locator("table td", { hasText: /3\.5(x|×)/ })).toBeVisible();
  });

  test("PostgreSQL tab includes recruiter keywords inline", async ({ page }) => {
    await page.goto("/database");
    // Several of these keywords appear in both narrative + bullet — first match is enough.
    await expect(page.getByText("PostgreSQL 16", { exact: false }).first()).toBeVisible();
    await expect(page.getByText("Range partitioning", { exact: false }).first()).toBeVisible();
    await expect(
      page.getByText("CREATE INDEX CONCURRENTLY", { exact: false }).first(),
    ).toBeVisible();
    await expect(page.getByText("postgres_exporter", { exact: false }).first()).toBeVisible();
  });

  test("clicking a TOC link scrolls to the corresponding pillar", async ({ page }) => {
    await page.goto("/database");
    // Default viewport is desktop-sized; the sidebar TOC is the visible one.
    await page
      .locator('[data-testid="sticky-toc-sidebar"] a[href="#migrations"]')
      .click();
    await expect(
      page.getByRole("heading", { name: "Migration Safety — migration-lint", level: 2 }),
    ).toBeInViewport();
  });

  test("/go Microservices tab links back to /database#optimization", async ({ page }) => {
    await page.goto("/go");
    const breadcrumb = page.locator('[data-testid="database-optimization-breadcrumb"]');
    await expect(breadcrumb).toBeVisible();
    const link = breadcrumb.getByRole("link", { name: "Database", exact: true });
    await expect(link).toHaveAttribute("href", "/database#optimization");
  });

  test("homepage links to /database", async ({ page }) => {
    await page.goto("/");
    const link = page.getByRole("link", { name: /Database Engineering/ });
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute("href", "/database");
  });
});
