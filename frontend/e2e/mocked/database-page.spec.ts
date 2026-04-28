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

  test("PostgreSQL tab renders all four pillar headings", async ({ page }) => {
    await page.goto("/database");
    await expect(
      page.getByRole("heading", { name: "Query Optimization & Benchmarking", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Schema Design — Partitioning & Materialized Views", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Migration Safety — migration-lint", level: 2 }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Reliability & Recovery", level: 2 }),
    ).toBeVisible();
  });

  test("each pillar has a stable anchor id", async ({ page }) => {
    await page.goto("/database");
    for (const id of ["optimization", "schema", "migrations", "reliability"]) {
      await expect(page.locator(`#${id}`)).toBeVisible();
    }
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
});
