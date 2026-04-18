import { test, expect } from "@playwright/test";
import path from "path";

const FRONTEND_URL =
  process.env.SMOKE_FRONTEND_URL || "https://kylebradshaw.dev";
const API_URL =
  process.env.SMOKE_API_URL || "https://api.kylebradshaw.dev";

test.describe("Production smoke tests", () => {
  test("frontend loads", async ({ page }) => {
    await page.goto(`${FRONTEND_URL}/ai/rag`);
    await expect(
      page.locator("h1", { hasText: "Document Q&A Assistant" })
    ).toBeVisible();
    await expect(
      page.getByPlaceholder("Ask a question about your documents...")
    ).toBeVisible();
  });

  test("backend health checks pass", async ({ request }) => {
    const chatHealth = await request.get(`${API_URL}/chat/health`);
    expect(chatHealth.ok()).toBeTruthy();
    const chatData = await chatHealth.json();
    expect(chatData.status).toBe("healthy");

    const ingestionHealth = await request.get(`${API_URL}/ingestion/health`);
    expect(ingestionHealth.ok()).toBeTruthy();
    const ingestionData = await ingestionHealth.json();
    expect(ingestionData.status).toBe("healthy");
  });

  test("grafana dashboard is reachable", async ({ request }) => {
    const grafanaUrl =
      process.env.SMOKE_GRAFANA_URL || "https://grafana.kylebradshaw.dev";
    const res = await request.get(`${grafanaUrl}/api/health`);
    expect(res.ok(), "grafana /api/health should return 2xx").toBeTruthy();
    const body = await res.json();
    expect(body.database).toBe("ok");
  });

  test("full E2E flow with cleanup", async ({ request }) => {
    const testCollection = "e2e-test";

    // Step 1: Upload test PDF to dedicated collection
    const pdfPath = path.join(__dirname, "..", "fixtures", "test.pdf");
    const fs = await import("fs");
    const pdfBuffer = fs.readFileSync(pdfPath);

    const uploadResponse = await request.post(
      `${API_URL}/ingestion/ingest?collection=${testCollection}`,
      {
        multipart: {
          file: {
            name: "test.pdf",
            mimeType: "application/pdf",
            buffer: pdfBuffer,
          },
        },
      }
    );
    expect(uploadResponse.ok()).toBeTruthy();
    const uploadData = await uploadResponse.json();
    expect(uploadData.status).toBe("success");
    expect(uploadData.chunks_created).toBeGreaterThan(0);

    // Step 2: Ask a question against the test collection
    const chatResponse = await request.post(`${API_URL}/chat/chat`, {
      data: {
        question: "What is artificial intelligence?",
        collection: testCollection,
      },
    });
    expect(chatResponse.ok()).toBeTruthy();
    const chatBody = await chatResponse.text();
    expect(chatBody).toContain("data:");

    // Step 3: Cleanup — delete the test collection
    const deleteResponse = await request.delete(
      `${API_URL}/ingestion/collections/${testCollection}`
    );
    expect(deleteResponse.ok()).toBeTruthy();
    const deleteData = await deleteResponse.json();
    expect(deleteData.status).toBe("deleted");
  });
});

const GRAPHQL_URL =
  process.env.SMOKE_GRAPHQL_URL || "https://api.kylebradshaw.dev/graphql";

test.describe("Java task management smoke tests", () => {
  // Shared state for cleanup. authCookieHeader carries the httpOnly access_token
  // cookie that the browser session received; we replay it via the Cookie header
  // on cleanup mutations since Playwright's test-level `request` fixture doesn't
  // share cookie jars with the `page` fixture.
  let authCookieHeader: string = "";
  let projectId: string;
  let taskId: string;
  const testEmail = `smoke-${Date.now()}@test.com`;
  const testPassword = "SmokeTest123!";

  test("java tasks page loads", async ({ page }) => {
    await page.goto(`${FRONTEND_URL}/java/tasks`);
    await expect(
      page.locator("h1", { hasText: "Task Manager" })
    ).toBeVisible();
    await expect(page.getByPlaceholder("Email")).toBeVisible();
    await expect(page.getByPlaceholder("Password")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Sign in", exact: true })
    ).toBeVisible();
  });

  test("register, create project and task", async ({ page, request }) => {
    // Step 1: Navigate to login page and click "Create account"
    await page.goto(`${FRONTEND_URL}/java/tasks`);
    await page.getByRole("button", { name: "Create account" }).click();

    // Step 2: Fill registration form
    await expect(
      page.locator("h1", { hasText: "Create Account" })
    ).toBeVisible();
    await page.getByPlaceholder("Name").fill("Smoke Test");
    await page.getByPlaceholder("Email").fill(testEmail);
    await page.getByPlaceholder("Password (min 8 characters)").fill(testPassword);
    await page.getByPlaceholder("Confirm password").fill(testPassword);
    await page.getByRole("button", { name: "Create account" }).click();

    // Step 3: Verify login succeeded — project list appears
    await expect(page.locator("h2", { hasText: "My Projects" })).toBeVisible({
      timeout: 10000,
    });

    // Step 4: Create a project
    await page.getByRole("button", { name: "New Project" }).click();
    await expect(
      page.locator("h3", { hasText: "New Project" })
    ).toBeVisible();
    await page.getByPlaceholder("My Project").fill("Smoke Test Project");
    await page.getByPlaceholder("Optional description").first().fill("Automated smoke test");
    await page.getByRole("button", { name: "Create" }).click();

    // Step 5: Verify project appears and click into it
    // Timeout bumped from 5s to 10s to tolerate post-rollout prod latency.
    await expect(page.getByText("Smoke Test Project")).toBeVisible({
      timeout: 10000,
    });
    await page.getByText("Smoke Test Project").click();

    // Step 6: Verify project page loaded
    await expect(
      page.locator("h1", { hasText: "Smoke Test Project" })
    ).toBeVisible({ timeout: 5000 });

    // Step 7: Create a task
    await page.getByRole("button", { name: "New Task" }).click();
    await expect(
      page.locator("h3", { hasText: "New Task" })
    ).toBeVisible();
    await page.getByPlaceholder("Task title").fill("Smoke Test Task");
    await page.locator("select").selectOption("HIGH");
    await page.getByRole("button", { name: "Create" }).click();

    // Step 8: Verify task appears on the board
    await expect(page.getByText("Smoke Test Task")).toBeVisible({
      timeout: 5000,
    });

    // Step 9: Capture the httpOnly access_token cookie from the browser session
    // and extract project/task IDs for cleanup. The backend moved access tokens
    // to httpOnly cookies, so we replay the cookie on cleanup mutations instead
    // of re-logging-in for a Bearer token.
    const cookies = await page.context().cookies();
    const accessCookie = cookies.find((c) => c.name === "access_token");
    if (accessCookie) {
      authCookieHeader = `access_token=${accessCookie.value}`;
    }

    // Get project ID from the URL
    const url = page.url();
    const urlParts = url.split("/");
    projectId = urlParts[urlParts.indexOf("tasks") + 1];

    // Get task ID by querying the page
    const taskLink = page.getByText("Smoke Test Task");
    const taskHref = await taskLink.evaluate((el) => {
      const link = el.closest("a");
      return link ? link.getAttribute("href") : null;
    });
    if (taskHref) {
      const taskParts = taskHref.split("/");
      taskId = taskParts[taskParts.length - 1];
    }
  });

  test.afterAll(async ({ request }) => {
    if (!authCookieHeader) return;

    const headers = {
      Cookie: authCookieHeader,
      "Content-Type": "application/json",
    };

    // Delete task
    if (taskId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `mutation { deleteTask(id: "${taskId}") }`,
        },
      });
    }

    // Delete project
    if (projectId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `mutation { deleteProject(id: "${projectId}") }`,
        },
      });
    }

    // Delete user account
    await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation { deleteAccount }`,
      },
    });
  });
});

test.describe("Go ecommerce smoke tests", () => {
  const SMOKE_EMAIL = "smoke@kylebradshaw.dev";
  const SMOKE_PASSWORD = process.env.SMOKE_GO_PASSWORD;

  test("products endpoint returns a non-empty catalog", async ({ request }) => {
    const res = await request.get(`${API_URL}/go-api/products`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body.products)).toBe(true);
    expect(body.products.length).toBeGreaterThan(0);
  });

  test("categories endpoint returns a non-empty list", async ({ request }) => {
    const res = await request.get(`${API_URL}/go-api/categories`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(Array.isArray(body.categories)).toBe(true);
    expect(body.categories.length).toBeGreaterThan(0);
  });

  test("login to Go auth service issues httpOnly access_token cookie", async ({
    request,
  }) => {
    expect(
      SMOKE_PASSWORD,
      "SMOKE_GO_PASSWORD env var must be set for this test"
    ).toBeTruthy();

    const res = await request.post(`${API_URL}/go-auth/auth/login`, {
      data: { email: SMOKE_EMAIL, password: SMOKE_PASSWORD },
    });
    expect(res.status()).toBe(200);

    // Response body carries user profile only; the access token lives in an
    // httpOnly cookie (security hardening: XSS can no longer read the token).
    const body = await res.json();
    expect(body.email).toBe(SMOKE_EMAIL);
    expect(typeof body.userId).toBe("string");

    // Verify the Set-Cookie header delivers access_token. Use headersArray so
    // multiple Set-Cookie entries are distinguishable.
    const setCookies = res
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value);
    const accessCookie = setCookies.find((v) => v.startsWith("access_token="));
    expect(
      accessCookie,
      "access_token cookie must be present in Set-Cookie headers"
    ).toBeDefined();
    expect(accessCookie).toContain("HttpOnly");
  });

  test("full checkout lifecycle: cart → order → verify", async ({
    playwright,
  }) => {
    expect(
      SMOKE_PASSWORD,
      "SMOKE_GO_PASSWORD env var must be set for this test"
    ).toBeTruthy();

    // Step 1: Login and get an authenticated API context with cookies
    const authContext = await playwright.request.newContext();
    const loginRes = await authContext.post(`${API_URL}/go-auth/auth/login`, {
      data: { email: SMOKE_EMAIL, password: SMOKE_PASSWORD },
    });
    expect(loginRes.status()).toBe(200);

    // Step 2: Find the Smoke Test Widget product
    const productsRes = await authContext.get(
      `${API_URL}/go-api/products?limit=50`
    );
    expect(productsRes.status()).toBe(200);
    const productsBody = await productsRes.json();
    const smokeProduct = productsBody.products.find(
      (p: { name: string }) => p.name === "Smoke Test Widget"
    );
    expect(
      smokeProduct,
      "Smoke Test Widget must exist in product catalog (check seed.sql)"
    ).toBeDefined();

    // Step 3: Add to cart
    const addRes = await authContext.post(`${API_URL}/go-api/cart`, {
      data: { productId: smokeProduct.id, quantity: 1 },
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(addRes.status()).toBe(201);

    // Step 4: Verify cart contents
    const cartRes = await authContext.get(`${API_URL}/go-api/cart`);
    expect(cartRes.status()).toBe(200);
    const cartBody = await cartRes.json();
    expect(cartBody.items.length).toBeGreaterThan(0);
    const cartItem = cartBody.items.find(
      (i: { productId: string }) => i.productId === smokeProduct.id
    );
    expect(cartItem, "Smoke product must be in cart").toBeDefined();

    // Step 5: Checkout
    const orderRes = await authContext.post(`${API_URL}/go-api/orders`, {
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(orderRes.status()).toBe(201);
    const order = await orderRes.json();
    expect(order.status).toBe("pending");
    expect(order.total).toBe(smokeProduct.price);

    // Step 6: Cart should be empty after checkout
    const emptyCartRes = await authContext.get(`${API_URL}/go-api/cart`);
    expect(emptyCartRes.status()).toBe(200);
    const emptyCartBody = await emptyCartRes.json();
    expect(emptyCartBody.items).toHaveLength(0);

    // Step 7: Checkout on empty cart should fail
    const emptyCheckoutRes = await authContext.post(
      `${API_URL}/go-api/orders`,
      {
        headers: { "Idempotency-Key": crypto.randomUUID() },
      }
    );
    expect(emptyCheckoutRes.status()).toBe(400);
    const emptyErr = await emptyCheckoutRes.json();
    expect(emptyErr.error.code).toBe("EMPTY_CART");

    await authContext.dispose();
  });
});
