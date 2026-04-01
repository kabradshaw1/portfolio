import { test, expect } from "@playwright/test";
import path from "path";

test.describe("Upload flow", () => {
  test("uploads a PDF and shows status", async ({ page }) => {
    await page.route("**/documents", (route) => {
      if (route.request().method() === "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            documents: [
              {
                document_id: "test-id",
                filename: "test.pdf",
                chunks: 3,
              },
            ],
          }),
        });
      }
      return route.continue();
    });

    await page.route("**/ingest**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          status: "success",
          document_id: "test-id",
          chunks_created: 3,
          filename: "test.pdf",
        }),
      })
    );

    await page.goto("/");

    // The file input is hidden — set files directly on it
    const fileInput = page.locator('input[type="file"]');
    await fileInput.setInputFiles(
      path.join(__dirname, "fixtures", "test.pdf")
    );

    // FileUpload sets status to "{filename} ({chunks_created} chunks)"
    await expect(page.getByText("test.pdf (3 chunks)")).toBeVisible();
    // DocumentList trigger shows "1 document uploaded"
    await expect(page.getByText("1 document uploaded")).toBeVisible();
  });
});
