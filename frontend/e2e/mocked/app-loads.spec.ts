import { test, expect } from "./fixtures";

test.describe("App loads", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );
  });

  test("renders the header", async ({ page }) => {
    await page.goto("/ai/rag");
    // Target the h1 in the header specifically (ChatWindow also has an h2 with same text)
    await expect(page.locator("h1", { hasText: "Document Q&A Assistant" })).toBeVisible();
  });

  test("shows empty state message", async ({ page }) => {
    await page.goto("/ai/rag");
    await expect(
      page.getByText("Upload a PDF using the button above", { exact: false })
    ).toBeVisible();
  });

  test("shows input field and send button", async ({ page }) => {
    await page.goto("/ai/rag");
    await expect(
      page.getByPlaceholder("Ask a question about your documents...")
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Send" })).toBeVisible();
  });

  test("shows upload button", async ({ page }) => {
    await page.goto("/ai/rag");
    await expect(
      page.getByRole("button", { name: "Upload PDF" })
    ).toBeVisible();
  });
});
