import { test, expect } from "./fixtures";

test.describe("Document delete", () => {
  test("opens document list and deletes a document", async ({ page }) => {
    let deleted = false;

    await page.route("**/documents", (route) => {
      if (route.request().method() === "GET") {
        const docs = deleted
          ? []
          : [
              {
                document_id: "test-id",
                filename: "test.pdf",
                chunks: 3,
              },
            ];
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ documents: docs }),
        });
      }
      return route.continue();
    });

    await page.route("**/documents/test-id", (route) => {
      if (route.request().method() === "DELETE") {
        deleted = true;
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            status: "deleted",
            document_id: "test-id",
            chunks_deleted: 3,
          }),
        });
      }
      return route.continue();
    });

    await page.goto("/ai/rag");

    // DocumentList trigger text: "1 document uploaded"
    await expect(page.getByText("1 document uploaded")).toBeVisible();
    await page.getByText("1 document uploaded").click();

    // Popover content shows the filename and chunk count
    await expect(page.getByText("test.pdf")).toBeVisible();
    // DocumentList renders "{chunks} chunk{s}" e.g. "3 chunks"
    await expect(page.getByText("3 chunks")).toBeVisible();

    // The delete button is a ghost Button containing only a Trash2 SVG icon,
    // scoped inside the list item for "test.pdf"
    const listItem = page.locator("li").filter({ hasText: "test.pdf" });
    const deleteButton = listItem.locator("button");
    await deleteButton.click();

    // After delete, fetchDocuments returns empty list so DocumentList is
    // no longer rendered (documents.length === 0 hides it in page.tsx)
    await expect(page.getByText("1 document uploaded")).not.toBeVisible();
  });
});
