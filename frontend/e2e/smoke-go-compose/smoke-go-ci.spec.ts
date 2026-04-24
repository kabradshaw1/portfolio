import { test, expect } from "@playwright/test";

// Go services expose individual ports (no gateway in compose).
const AUTH_URL = process.env.SMOKE_AUTH_URL || "http://localhost:8091";
const ORDER_URL = process.env.SMOKE_ORDER_URL || "http://localhost:8092";
const PRODUCT_URL = process.env.SMOKE_PRODUCT_URL || "http://localhost:8095";
const CART_URL = process.env.SMOKE_CART_URL || "http://localhost:8096";
const ANALYTICS_URL =
  process.env.SMOKE_ANALYTICS_URL || "http://localhost:8094";

test.describe("Go compose-smoke CI tests", () => {
  test("health checks pass for all services", async ({ request }) => {
    const services = [
      { name: "auth", url: AUTH_URL },
      { name: "order", url: ORDER_URL },
      { name: "product", url: PRODUCT_URL },
      { name: "cart", url: CART_URL },
      { name: "analytics", url: ANALYTICS_URL },
    ];

    for (const svc of services) {
      const res = await request.get(`${svc.url}/health`);
      expect(
        res.ok(),
        `${svc.name} /health should return 2xx (got ${res.status()})`
      ).toBeTruthy();
    }
  });

  test("auth flow: register → login → cookie", async ({ playwright }) => {
    const testEmail = `ci-smoke-${Date.now()}@test.com`;
    const testPassword = "CiSmokeTest123!";

    const ctx = await playwright.request.newContext();

    // Register
    const registerRes = await ctx.post(`${AUTH_URL}/auth/register`, {
      data: { email: testEmail, password: testPassword, name: "CI Smoke" },
    });
    expect(registerRes.status()).toBe(200);
    const registerBody = await registerRes.json();
    expect(registerBody.email).toBe(testEmail);

    // Login
    const loginRes = await ctx.post(`${AUTH_URL}/auth/login`, {
      data: { email: testEmail, password: testPassword },
    });
    expect(loginRes.status()).toBe(200);

    // Verify httpOnly access_token cookie
    const setCookies = loginRes
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value);
    const accessCookie = setCookies.find((v) =>
      v.startsWith("access_token=")
    );
    expect(
      accessCookie,
      "access_token cookie must be present"
    ).toBeDefined();
    expect(accessCookie).toContain("HttpOnly");

    await ctx.dispose();
  });

  test("product catalog returns data", async ({ request }) => {
    const productsRes = await request.get(`${PRODUCT_URL}/products`);
    expect(productsRes.status()).toBe(200);
    const productsBody = await productsRes.json();
    expect(Array.isArray(productsBody.products)).toBe(true);
    expect(productsBody.products.length).toBeGreaterThan(0);

    const categoriesRes = await request.get(`${PRODUCT_URL}/categories`);
    expect(categoriesRes.status()).toBe(200);
    const categoriesBody = await categoriesRes.json();
    expect(Array.isArray(categoriesBody.categories)).toBe(true);
    expect(categoriesBody.categories.length).toBeGreaterThan(0);
  });

  test("cart flow: add item and verify", async ({ playwright }) => {
    // Full checkout saga requires payment-service (Stripe) which isn't
    // available in CI compose. Test cart operations here; the full
    // checkout lifecycle is covered in smoke-prod where all services run.
    const testEmail = `ci-cart-${Date.now()}@test.com`;
    const testPassword = "CiCartTest123!!";

    const authCtx = await playwright.request.newContext();
    await authCtx.post(`${AUTH_URL}/auth/register`, {
      data: { email: testEmail, password: testPassword, name: "CI Cart" },
    });
    const loginRes = await authCtx.post(`${AUTH_URL}/auth/login`, {
      data: { email: testEmail, password: testPassword },
    });
    expect(loginRes.status()).toBe(200);

    // Find Smoke Test Widget
    const productsRes = await authCtx.get(`${PRODUCT_URL}/products?limit=50`);
    const productsBody = await productsRes.json();
    const smokeProduct = productsBody.products.find(
      (p: { name: string }) => p.name === "Smoke Test Widget"
    );
    expect(
      smokeProduct,
      "Smoke Test Widget must exist (check product-service seed.sql)"
    ).toBeDefined();

    // Add to cart
    const addRes = await authCtx.post(`${CART_URL}/cart`, {
      data: { productId: smokeProduct.id, quantity: 1 },
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(addRes.status()).toBe(201);

    // Verify cart contents
    const cartRes = await authCtx.get(`${CART_URL}/cart`);
    expect(cartRes.status()).toBe(200);
    const cartBody = await cartRes.json();
    expect(cartBody.items.length).toBeGreaterThan(0);
    const cartItem = cartBody.items.find(
      (i: { productId: string }) => i.productId === smokeProduct.id
    );
    expect(cartItem, "Smoke product must be in cart").toBeDefined();

    await authCtx.dispose();
  });
});
