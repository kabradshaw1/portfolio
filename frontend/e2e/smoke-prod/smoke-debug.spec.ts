import { test, expect } from "@playwright/test";

const API_URL =
  process.env.SMOKE_API_URL || "https://api.kylebradshaw.dev";

test.describe("Debug service smoke test", () => {
  test("debug endpoint responds to invalid collection with 400", async ({
    request,
  }) => {
    // The /debug endpoint requires a previously-indexed collection.
    // We can't index in prod smoke tests (requires server filesystem path),
    // so we verify the endpoint is alive by sending a request with a
    // non-existent collection and asserting the expected 400 error.
    const res = await request.post(`${API_URL}/debug/debug`, {
      data: {
        collection: "smoke-nonexistent",
        description: "What does this code do?",
      },
    });
    // 400 = collection not indexed (expected). 401 = auth required.
    // Either proves the endpoint is alive and routing works.
    expect(
      [400, 401, 422].includes(res.status()),
      `debug endpoint should return 400/401/422 for invalid request (got ${res.status()})`
    ).toBeTruthy();
  });
});
