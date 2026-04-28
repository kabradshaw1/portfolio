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
});
