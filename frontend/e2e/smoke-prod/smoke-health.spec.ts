import { test, expect } from "@playwright/test";

const API_URL =
  process.env.SMOKE_API_URL || "https://api.kylebradshaw.dev";

test.describe("Service health checks", () => {
  test("Go service health endpoints return 200", async ({ request }) => {
    const endpoints = [
      "/go-auth/health",
      "/go-orders/health",
      "/go-products/health",
      "/go-cart/health",
      "/go-analytics/health",
    ];

    for (const endpoint of endpoints) {
      const res = await request.get(`${API_URL}${endpoint}`);
      expect(
        res.ok(),
        `${endpoint} should return 2xx (got ${res.status()})`
      ).toBeTruthy();
    }
  });

  test("Go AI service health and readiness", async ({ request }) => {
    const healthRes = await request.get(`${API_URL}/go-ai/health`);
    expect(healthRes.ok(), "ai-service /health should return 2xx").toBeTruthy();

    const readyRes = await request.get(`${API_URL}/go-ai/ready`);
    expect(readyRes.ok(), "ai-service /ready should return 2xx").toBeTruthy();
    const readyBody = await readyRes.json();
    // /ready returns { "checks": { "llm": "...", "ecommerce": "..." } }
    expect(readyBody.checks).toBeDefined();
  });

  test("debug service health returns healthy", async ({ request }) => {
    const res = await request.get(`${API_URL}/debug/health`);
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.status).toBe("healthy");
  });
});
