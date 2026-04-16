import { test, expect } from "./fixtures";

test.describe("Go ecommerce AI assistant drawer", () => {
  test("opens, streams a tool call and a final answer", async ({ page }) => {
    // Mock ai-service /chat SSE: tool_call -> tool_result -> final.
    // Each event block must end with \n\n so the SSE parser's buffer is flushed.
    await page.route("**/chat", (route) => {
      const sseBody = [
        "event: tool_call",
        'data: {"name":"search_products","args":{"query":"waterproof jacket","max_price":150}}',
        "",
        "event: tool_result",
        'data: {"name":"search_products","display":{"kind":"product_list","products":[{"id":"p1","name":"Waterproof Jacket","price":12999}]}}',
        "",
        "event: final",
        'data: {"text":"I found a waterproof jacket under $150."}',
        "",
      ].join("\n");

      return route.fulfill({
        status: 200,
        contentType: "text/event-stream",
        body: sseBody,
      });
    });

    // Loosely stub ecommerce product listings that the Go ecommerce page may fetch on load.
    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
      }),
    );

    await page.goto("/go/ecommerce");

    await page.getByTestId("ai-assistant-open").click();
    await expect(page.getByTestId("ai-assistant-drawer")).toBeVisible();

    const input = page.getByTestId("ai-assistant-input");
    await input.fill("find me a waterproof jacket under $150");
    await page.getByTestId("ai-assistant-send").click();

    // User bubble appears with the original text.
    await expect(
      page.getByText("find me a waterproof jacket under $150"),
    ).toBeVisible();

    // Tool call card renders the tool name.
    await expect(page.getByText("search_products")).toBeVisible();

    // Tool args render as pretty-printed JSON in the tool card.
    await expect(page.getByText(/"max_price": 150/)).toBeVisible();

    // Final answer streamed into the assistant bubble.
    await expect(page.getByTestId("ai-assistant-final")).toHaveText(
      "I found a waterproof jacket under $150.",
    );
  });

  test("shows an error when ai-service returns 500", async ({ page }) => {
    await page.route("**/chat", (route) =>
      route.fulfill({ status: 500, body: "internal" }),
    );
    await page.route("**/products**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ products: [], total: 0, page: 1, limit: 20 }),
      }),
    );

    await page.goto("/go/ecommerce");
    await page.getByTestId("ai-assistant-open").click();
    await page.getByTestId("ai-assistant-input").fill("hi");
    await page.getByTestId("ai-assistant-send").click();

    await expect(page.getByTestId("ai-assistant-error")).toBeVisible();
  });
});
