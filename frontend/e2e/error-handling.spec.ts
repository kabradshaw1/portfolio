import { test, expect } from "@playwright/test";

test.describe("Error handling", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );
  });

  test("shows error when chat service is down", async ({ page }) => {
    await page.route("**/chat", (route) =>
      route.fulfill({ status: 500, body: "Internal Server Error" })
    );

    await page.goto("/");

    const input = page.getByPlaceholder(
      "Ask a question about your documents..."
    );
    await input.fill("test question");
    await page.getByRole("button", { name: "Send" }).click();

    // page.tsx throws Error("Failed to connect to chat service") on non-ok response
    await expect(
      page.getByText("Failed to connect to chat service")
    ).toBeVisible();
  });

  test("shows error when upload fails", async ({ page }) => {
    await page.route("**/ingest**", (route) =>
      route.fulfill({
        status: 422,
        contentType: "application/json",
        body: JSON.stringify({ detail: "No text content found in PDF" }),
      })
    );

    await page.goto("/");

    // Use setInputFiles with a buffer to avoid needing a real file on disk
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles({
      name: "empty.pdf",
      mimeType: "application/pdf",
      buffer: Buffer.from("%PDF-1.4 fake"),
    });

    // FileUpload catches the error and sets status to err.detail
    await expect(
      page.getByText("No text content found in PDF")
    ).toBeVisible();
  });
});
