import { test, expect } from "./fixtures";

const MOCK_DLQ_MESSAGES = {
  messages: [
    {
      index: 0,
      routing_key: "saga.cart.commands",
      exchange: "ecommerce.saga",
      timestamp: new Date().toISOString(),
      retry_count: 0,
      headers: { "x-retry-count": 0 },
      body: { command: "reserve.items", order_id: "test-order-1" },
    },
    {
      index: 1,
      routing_key: "saga.order.events",
      exchange: "ecommerce.saga",
      timestamp: new Date(Date.now() - 900_000).toISOString(),
      retry_count: 1,
      headers: { "x-retry-count": 1 },
      body: { event: "items.reserved", order_id: "test-order-2" },
    },
  ],
  count: 2,
};

test.describe("DLQ Admin Panel", () => {
  test.beforeEach(async ({ page }) => {
    await page.route("**/admin/dlq/messages*", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(MOCK_DLQ_MESSAGES),
      }),
    );
  });

  test("renders the admin page with demo banner", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.locator("h1", { hasText: "DLQ Admin" })).toBeVisible();
    await expect(page.getByText("Portfolio Demo")).toBeVisible();
  });

  test("displays DLQ messages in a table", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.getByText("saga.cart.commands")).toBeVisible();
    await expect(page.getByText("saga.order.events")).toBeVisible();
  });

  test("shows message count", async ({ page }) => {
    await page.goto("/go/admin");
    await expect(page.getByText("2", { exact: true })).toBeVisible();
  });

  test("expands row to show message body", async ({ page }) => {
    await page.goto("/go/admin");
    await page.getByText("saga.cart.commands").click();
    await expect(page.getByText("reserve.items")).toBeVisible();
    await expect(page.getByText("test-order-1")).toBeVisible();
  });

  test("replay button sends POST request", async ({ page }) => {
    let replayRequested = false;

    await page.route("**/admin/dlq/replay", (route) => {
      replayRequested = true;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          replayed: { ...MOCK_DLQ_MESSAGES.messages[0], retry_count: 1 },
        }),
      });
    });

    await page.goto("/go/admin");
    const replayButtons = page.getByRole("button", { name: "Replay" });
    await replayButtons.first().click();

    await expect(page.getByText("Replayed")).toBeVisible();
    expect(replayRequested).toBe(true);
  });

  test("shows unreachable state when backend is down", async ({ page }) => {
    await page.route("**/admin/dlq/messages*", (route) => route.abort());

    await page.goto("/go/admin");
    await expect(
      page.getByText("Admin endpoints are not publicly exposed"),
    ).toBeVisible();
  });
});
