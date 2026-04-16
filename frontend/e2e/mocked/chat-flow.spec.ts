import { test, expect } from "./fixtures";

test.describe("Chat flow", () => {
  test("sends a question and receives a streamed response", async ({
    page,
  }) => {
    await page.route("**/documents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ documents: [] }),
      })
    );

    await page.route("**/chat", (route) => {
      const sseBody = [
        'data: {"token": "Hello"}',
        "",
        'data: {"token": " world"}',
        "",
        'data: {"done": true, "sources": [{"file": "test.pdf", "page": 1}]}',
        "",
      ].join("\n");

      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: sseBody,
      });
    });

    await page.goto("/ai/rag");

    const input = page.getByPlaceholder(
      "Ask a question about your documents..."
    );
    await input.fill("What is this about?");
    await page.getByRole("button", { name: "Send" }).click();

    await expect(page.getByText("What is this about?")).toBeVisible();
    await expect(page.getByText("Hello world")).toBeVisible();
    // SourceBadge renders: "{filename}, p.{page}" e.g. "test.pdf, p.1"
    await expect(page.getByText("test.pdf, p.1")).toBeVisible();
  });
});
