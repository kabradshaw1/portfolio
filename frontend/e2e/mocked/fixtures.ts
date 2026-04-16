import { test as base } from "@playwright/test";

/**
 * Extend the base test to automatically mock all backend health endpoints.
 *
 * Every page wraps its content in a <HealthGate> that fetches a health URL
 * on mount. Without a running backend the gate renders "Server Maintenance"
 * instead of the real page, causing every assertion to fail.
 *
 * This fixture intercepts all health requests and returns 200 so the gate
 * passes and the page content renders.
 */
export const test = base.extend({
  page: async ({ page }, use) => {
    // Mock health endpoints for all backend stacks
    await page.route("**/health", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "healthy" }),
      }),
    );
    await page.route("**/actuator/health", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "UP" }),
      }),
    );
    // eslint-disable-next-line react-hooks/rules-of-hooks
    await use(page);
  },
});

export { expect } from "@playwright/test";
