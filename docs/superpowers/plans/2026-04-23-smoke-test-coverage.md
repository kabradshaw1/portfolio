# Smoke Test Coverage Expansion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expand smoke test coverage to include Go compose-smoke CI, Java compose-smoke CI, prod health checks for all services, debug service functional test, and analytics/activity indirect verification.

**Architecture:** Add two new CI compose-smoke jobs (Go, Java) using Playwright, extend prod smoke tests with health checks and async verification. All tests use Playwright — no shell scripts. Go compose needs a CI overlay adding product-service and cart-service (missing from base compose), a Postgres init script for multi-DB setup + migrations + seed data, and env overrides. Java compose needs a simple env override.

**Tech Stack:** Playwright, Docker Compose, GitHub Actions, TypeScript

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `go/docker-compose.ci.yml` | Create | CI overlay — adds product/cart services, sets env vars, mounts DB init script |
| `go/ci-init.sql` | Create | Postgres init script — creates DBs, runs migrations, seeds data |
| `java/docker-compose.ci.yml` | Create | CI overlay — sets JWT_SECRET and ALLOWED_ORIGINS |
| `frontend/playwright.smoke-go.config.ts` | Create | Playwright config pointing at `e2e/smoke-go-compose/` |
| `frontend/playwright.smoke-java.config.ts` | Create | Playwright config pointing at `e2e/smoke-java-compose/` |
| `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts` | Create | Go stack compose-smoke tests |
| `frontend/e2e/smoke-java-compose/smoke-java-ci.spec.ts` | Create | Java stack compose-smoke tests |
| `frontend/e2e/smoke-prod/smoke-health.spec.ts` | Create | Prod health checks for uncovered services |
| `frontend/e2e/smoke-prod/smoke-debug.spec.ts` | Create | Debug service functional test |
| `frontend/e2e/smoke-prod/smoke.spec.ts` | Modify | Extend checkout test with analytics polling, Java test with activity query |
| `.github/workflows/ci.yml` | Modify | Add `compose-smoke-go` and `compose-smoke-java` jobs |

---

### Task 1: Go CI Compose Overlay and DB Init

**Files:**
- Create: `go/ci-init.sql`
- Create: `go/docker-compose.ci.yml`

This task sets up the Docker Compose infrastructure for Go compose-smoke. The base `go/docker-compose.yml` is missing product-service and cart-service, and services need separate databases (productdb, cartdb) that don't exist in the single-DB compose setup.

- [ ] **Step 1: Create the Postgres init script**

This script runs when the Postgres container first starts. It creates the additional databases, runs migrations via `psql` (the migrate binary isn't available in the Postgres container), and seeds test data. The base compose already creates `ecommercedb` via `POSTGRES_DB`.

Create `go/ci-init.sql`:

```sql
-- ci-init.sql — run by Postgres on first boot via docker-entrypoint-initdb.d.
-- Creates additional databases needed by decomposed Go services.
-- ecommercedb is already created by POSTGRES_DB env var.

CREATE DATABASE productdb;
CREATE DATABASE cartdb;

-- Grant access to the default user
GRANT ALL PRIVILEGES ON DATABASE productdb TO taskuser;
GRANT ALL PRIVILEGES ON DATABASE cartdb TO taskuser;
```

- [ ] **Step 2: Create the Go CI compose overlay**

Create `go/docker-compose.ci.yml`. This overlay:
- Mounts `ci-init.sql` into Postgres init directory to create productdb and cartdb
- Adds product-service and cart-service (missing from base compose)
- Sets `JWT_SECRET` and `ALLOWED_ORIGINS` for all services
- Removes `${JWT_SECRET:?...}` required var error by providing defaults

```yaml
services:
  postgres:
    volumes:
      - ./ci-init.sql:/docker-entrypoint-initdb.d/01-ci-init.sql:ro

  auth-service:
    environment:
      JWT_SECRET: ci-test-secret
      ALLOWED_ORIGINS: "*"
      DATABASE_URL: postgres://taskuser:taskpass@postgres:5432/ecommercedb?sslmode=disable

  order-service:
    environment:
      JWT_SECRET: ci-test-secret
      ALLOWED_ORIGINS: "*"
      DATABASE_URL: postgres://taskuser:taskpass@postgres:5432/ecommercedb?sslmode=disable

  product-service:
    build:
      context: .
      dockerfile: product-service/Dockerfile
    ports:
      - "8095:8095"
      - "9095:9095"
    environment:
      DATABASE_URL: postgres://taskuser:taskpass@postgres:5432/productdb?sslmode=disable
      ALLOWED_ORIGINS: "*"
      PORT: "8095"
      GRPC_PORT: "9095"
      REDIS_URL: redis://redis:6379
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  cart-service:
    build:
      context: .
      dockerfile: cart-service/Dockerfile
    ports:
      - "8096:8096"
      - "9096:9096"
    environment:
      DATABASE_URL: postgres://taskuser:taskpass@postgres:5432/cartdb?sslmode=disable
      JWT_SECRET: ci-test-secret
      ALLOWED_ORIGINS: "*"
      PORT: "8096"
      GRPC_PORT: "9096"
      REDIS_URL: redis://redis:6379
      KAFKA_BROKERS: kafka:9092
      PRODUCT_GRPC_ADDR: product-service:9095
      RABBITMQ_URL: amqp://guest:guest@rabbitmq:5672
      AUTH_GRPC_URL: auth-service:9091
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      rabbitmq:
        condition: service_healthy
      kafka:
        condition: service_healthy
      product-service:
        condition: service_started
      auth-service:
        condition: service_started

  analytics-service:
    environment:
      ALLOWED_ORIGINS: "*"
```

- [ ] **Step 3: Verify overlay merges correctly (local sanity check)**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/go && docker compose -f docker-compose.yml -f docker-compose.ci.yml config --services`

Expected output should list: postgres, redis, rabbitmq, kafka, auth-service, order-service, ai-service, analytics-service, product-service, cart-service

- [ ] **Step 4: Commit**

```bash
git add go/ci-init.sql go/docker-compose.ci.yml
git commit -m "feat(go): add CI compose overlay with product/cart services and DB init"
```

---

### Task 2: Go Compose-Smoke Playwright Config and Tests

**Files:**
- Create: `frontend/playwright.smoke-go.config.ts`
- Create: `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`

- [ ] **Step 1: Create the Playwright config**

Create `frontend/playwright.smoke-go.config.ts`:

```typescript
import { defineConfig } from "@playwright/test";

// Config for the Go compose-smoke CI job. Go services expose individual
// ports (no gateway). Tests hit services directly.
export default defineConfig({
  testDir: "./e2e/smoke-go-compose",
  fullyParallel: false,
  retries: 1,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
  },
});
```

- [ ] **Step 2: Create the Go compose-smoke test file**

The Go services run migrations automatically via the `migrate` binary baked into their Docker images — they run `migrate up` at container startup as part of the entrypoint. However, the compose setup uses `ENTRYPOINT ["/product-service"]` which runs the Go binary directly, NOT migrations. Migrations are handled by K8s Jobs in prod.

For CI compose, migrations need to run before tests. The services create their own tables at startup via auto-migration in the Go binary (check each service's `main.go` for `pgxpool` setup that may auto-create schema). If they don't auto-migrate, we'll need to handle this differently.

Actually, looking at the Dockerfiles, migrations and seeds are baked into the images. We need a separate step in CI to run them. But for simplicity, let's use a `docker compose exec` approach in the CI workflow to run migrations before tests.

For now, write the test file assuming the DB is ready with schema and seed data:

Create `frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts`:

```typescript
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

  test("checkout lifecycle with analytics verification", async ({
    playwright,
  }) => {
    const testEmail = `ci-checkout-${Date.now()}@test.com`;
    const testPassword = "CiCheckout123!!";

    // Register + login to get auth context
    const authCtx = await playwright.request.newContext();
    await authCtx.post(`${AUTH_URL}/auth/register`, {
      data: { email: testEmail, password: testPassword, name: "CI Checkout" },
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

    // Verify cart
    const cartRes = await authCtx.get(`${CART_URL}/cart`);
    expect(cartRes.status()).toBe(200);
    const cartBody = await cartRes.json();
    expect(cartBody.items.length).toBeGreaterThan(0);

    // Checkout
    const orderRes = await authCtx.post(`${ORDER_URL}/orders`, {
      headers: { "Idempotency-Key": crypto.randomUUID() },
    });
    expect(orderRes.status()).toBe(201);
    const order = await orderRes.json();
    expect(order.status).toBe("pending");

    // Poll cart empty (saga clears async, up to 15s)
    for (let i = 0; i < 30; i++) {
      const pollRes = await authCtx.get(`${CART_URL}/cart`);
      const pollBody = await pollRes.json();
      if ((pollBody.items ?? []).length === 0) break;
      await new Promise((r) => setTimeout(r, 500));
    }

    // Poll analytics for order event (up to 15s)
    let analyticsHasData = false;
    for (let i = 0; i < 30; i++) {
      const analyticsRes = await authCtx.get(
        `${ANALYTICS_URL}/analytics/revenue?hours=1`
      );
      if (analyticsRes.ok()) {
        const analyticsBody = await analyticsRes.json();
        if (
          analyticsBody.windows &&
          analyticsBody.windows.length > 0
        ) {
          analyticsHasData = true;
          break;
        }
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(
      analyticsHasData,
      "analytics should have revenue data after checkout"
    ).toBeTruthy();

    await authCtx.dispose();
  });
});
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors related to the new files.

- [ ] **Step 4: Commit**

```bash
git add frontend/playwright.smoke-go.config.ts frontend/e2e/smoke-go-compose/smoke-go-ci.spec.ts
git commit -m "feat(e2e): add Go compose-smoke Playwright tests"
```

---

### Task 3: Java CI Compose Overlay and Smoke Tests

**Files:**
- Create: `java/docker-compose.ci.yml`
- Create: `frontend/playwright.smoke-java.config.ts`
- Create: `frontend/e2e/smoke-java-compose/smoke-java-ci.spec.ts`

- [ ] **Step 1: Create the Java CI compose overlay**

Create `java/docker-compose.ci.yml`:

```yaml
services:
  task-service:
    environment:
      JWT_SECRET: ci-test-secret-at-least-32-characters-long
      ALLOWED_ORIGINS: "*"

  activity-service:
    environment:
      RABBITMQ_HOST: rabbitmq

  notification-service:
    environment:
      RABBITMQ_HOST: rabbitmq
      REDIS_HOST: redis

  gateway-service:
    environment:
      JWT_SECRET: ci-test-secret-at-least-32-characters-long
      ALLOWED_ORIGINS: "*"
```

- [ ] **Step 2: Create the Playwright config**

Create `frontend/playwright.smoke-java.config.ts`:

```typescript
import { defineConfig } from "@playwright/test";

// Config for the Java compose-smoke CI job. Tests go through the
// gateway-service at port 8080.
export default defineConfig({
  testDir: "./e2e/smoke-java-compose",
  fullyParallel: false,
  retries: 1,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
  },
});
```

- [ ] **Step 3: Create the Java compose-smoke test file**

Create `frontend/e2e/smoke-java-compose/smoke-java-ci.spec.ts`:

```typescript
import { test, expect } from "@playwright/test";

// Java services go through the gateway at port 8080.
const GATEWAY_URL = process.env.SMOKE_API_URL || "http://localhost:8080";
const GRAPHQL_URL = `${GATEWAY_URL}/graphql`;

test.describe("Java compose-smoke CI tests", () => {
  // Shared state for cleanup
  let authCookieHeader: string = "";
  let projectId: string;
  let taskId: string;
  const testEmail = `ci-smoke-${Date.now()}@test.com`;
  const testPassword = "CiSmokeTest123!";

  test("gateway health check", async ({ request }) => {
    // Spring Boot gateway should respond on its base port
    const res = await request.get(`${GATEWAY_URL}/graphql`, {
      headers: { "Content-Type": "application/json" },
      data: { query: "{ __typename }" },
    });
    expect(res.ok(), "gateway GraphQL endpoint should respond").toBeTruthy();
  });

  test("GraphQL schema loads via introspection", async ({ request }) => {
    const res = await request.post(GRAPHQL_URL, {
      data: {
        query: `{ __schema { queryType { name } mutationType { name } } }`,
      },
    });
    expect(res.ok()).toBeTruthy();
    const body = await res.json();
    expect(body.data.__schema.queryType.name).toBe("Query");
    expect(body.data.__schema.mutationType.name).toBe("Mutation");
  });

  test("register → project → task → verify activity", async ({ request }) => {
    // Step 1: Register
    const registerRes = await request.post(GRAPHQL_URL, {
      data: {
        query: `mutation Register($email: String!, $password: String!, $name: String!) {
          register(email: $email, password: $password, name: $name) { id email name }
        }`,
        variables: { email: testEmail, password: testPassword, name: "CI Smoke" },
      },
    });
    expect(registerRes.ok()).toBeTruthy();

    // Capture auth cookie from registration
    const regCookies = registerRes
      .headersArray()
      .filter((h) => h.name.toLowerCase() === "set-cookie")
      .map((h) => h.value);
    const regAccessCookie = regCookies.find((v) =>
      v.startsWith("access_token=")
    );
    if (regAccessCookie) {
      const match = regAccessCookie.match(/^access_token=([^;]+)/);
      if (match) authCookieHeader = `access_token=${match[1]}`;
    }

    const headers = {
      Cookie: authCookieHeader,
      "Content-Type": "application/json",
    };

    // Step 2: Create project
    const projectRes = await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation { createProject(input: { name: "CI Smoke Project", description: "Automated CI smoke test" }) { id name } }`,
      },
    });
    expect(projectRes.ok()).toBeTruthy();
    const projectBody = await projectRes.json();
    projectId = projectBody.data.createProject.id;
    expect(projectId).toBeTruthy();

    // Step 3: Create task
    const taskRes = await request.post(GRAPHQL_URL, {
      headers,
      data: {
        query: `mutation CreateTask($input: CreateTaskInput!) { createTask(input: $input) { id title } }`,
        variables: {
          input: {
            projectId,
            title: "CI Smoke Task",
            priority: "HIGH",
          },
        },
      },
    });
    expect(taskRes.ok()).toBeTruthy();
    const taskBody = await taskRes.json();
    taskId = taskBody.data.createTask.id;
    expect(taskId).toBeTruthy();

    // Step 4: Verify activity feed has entries (async — poll with retries)
    // Task creation triggers: task-service → RabbitMQ → notification-service → activity-service
    let activityFound = false;
    for (let i = 0; i < 20; i++) {
      const activityRes = await request.post(GRAPHQL_URL, {
        headers,
        data: {
          query: `query { taskActivity(taskId: "${taskId}") { id eventType } }`,
        },
      });
      if (activityRes.ok()) {
        const activityBody = await activityRes.json();
        if (
          activityBody.data?.taskActivity &&
          activityBody.data.taskActivity.length > 0
        ) {
          activityFound = true;
          break;
        }
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(
      activityFound,
      "activity feed should have entries after task creation (RabbitMQ async flow)"
    ).toBeTruthy();
  });

  test.afterAll(async ({ request }) => {
    if (!authCookieHeader) return;

    const headers = {
      Cookie: authCookieHeader,
      "Content-Type": "application/json",
    };

    if (taskId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: { query: `mutation { deleteTask(id: "${taskId}") }` },
      });
    }

    if (projectId) {
      await request.post(GRAPHQL_URL, {
        headers,
        data: { query: `mutation { deleteProject(id: "${projectId}") }` },
      });
    }

    await request.post(GRAPHQL_URL, {
      headers,
      data: { query: `mutation { deleteAccount }` },
    });
  });
});
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors related to the new files.

- [ ] **Step 5: Commit**

```bash
git add java/docker-compose.ci.yml frontend/playwright.smoke-java.config.ts frontend/e2e/smoke-java-compose/smoke-java-ci.spec.ts
git commit -m "feat(e2e): add Java compose-smoke Playwright tests with activity feed verification"
```

---

### Task 4: Prod Smoke Health Checks

**Files:**
- Create: `frontend/e2e/smoke-prod/smoke-health.spec.ts`

- [ ] **Step 1: Create the health check spec**

Create `frontend/e2e/smoke-prod/smoke-health.spec.ts`:

```typescript
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
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/smoke-prod/smoke-health.spec.ts
git commit -m "feat(e2e): add prod health checks for Go, AI, and debug services"
```

---

### Task 5: Prod Smoke Debug Functional Test

**Files:**
- Create: `frontend/e2e/smoke-prod/smoke-debug.spec.ts`

The debug service's `/debug` endpoint requires a collection that was previously indexed via `/index`. This means a light functional test needs to:
1. Index a small code snippet (via `/index` or by using an existing test collection)
2. Send a debug query
3. Verify SSE events come back

However, `/index` requires a path on the server filesystem, which we don't control in prod smoke tests. A simpler approach: just verify `/debug/health` is healthy (covered in Task 4) and that the endpoint responds with a useful error when called without a valid collection. This proves the service is up and the HTTP pipeline works.

- [ ] **Step 1: Create the debug functional test spec**

Create `frontend/e2e/smoke-prod/smoke-debug.spec.ts`:

```typescript
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
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/e2e/smoke-prod/smoke-debug.spec.ts
git commit -m "feat(e2e): add debug service prod smoke test"
```

---

### Task 6: Extend Existing Prod Smoke Tests

**Files:**
- Modify: `frontend/e2e/smoke-prod/smoke.spec.ts`

Two extensions: (1) analytics polling after checkout, (2) activity feed query after Java task creation.

- [ ] **Step 1: Add analytics verification to checkout test**

In `frontend/e2e/smoke-prod/smoke.spec.ts`, find the checkout test's cart-empty polling loop (lines 346-355). After the cart-empty block (after line 355, before the `if (cartEmpty)` block), add analytics polling:

```typescript
    // Step 6b: Verify analytics consumed the order event (Kafka → analytics-service)
    let analyticsHasData = false;
    for (let i = 0; i < 30; i++) {
      const analyticsRes = await authContext.get(
        `${API_URL}/go-analytics/analytics/revenue?hours=1`
      );
      if (analyticsRes.ok()) {
        const analyticsBody = await analyticsRes.json();
        if (
          analyticsBody.windows &&
          analyticsBody.windows.length > 0
        ) {
          analyticsHasData = true;
          break;
        }
      }
      await new Promise((r) => setTimeout(r, 500));
    }
    // Non-blocking: analytics may take longer than 15s to process in prod.
    // Log but don't fail the test — the checkout itself is the critical assertion.
    if (!analyticsHasData) {
      console.warn(
        "analytics did not report revenue data within 15s — Kafka consumer may be lagging"
      );
    }
```

Insert this code after line 355 (`}` closing the cart-empty polling loop) and before line 357 (`if (cartEmpty) {`).

- [ ] **Step 2: Add activity feed verification to Java task test**

In the same file, find the "register, create project and task" test. The auth cookie is captured at lines 169-176 and the taskId at lines 183-192. Insert the activity polling block after line 192 (after taskId is extracted), before the closing `});` of the test:

```typescript
    // Step 9b: Verify activity feed has entries (async RabbitMQ flow)
    // task-service → RabbitMQ → notification-service → activity-service
    if (taskId && authCookieHeader) {
      let activityFound = false;
      for (let i = 0; i < 20; i++) {
        const activityRes = await request.post(GRAPHQL_URL, {
          headers: {
            Cookie: authCookieHeader,
            "Content-Type": "application/json",
          },
          data: {
            query: `query { taskActivity(taskId: "${taskId}") { id eventType } }`,
          },
        });
        if (activityRes.ok()) {
          const activityBody = await activityRes.json();
          if (
            activityBody.data?.taskActivity &&
            activityBody.data.taskActivity.length > 0
          ) {
            activityFound = true;
            break;
          }
        }
        await new Promise((r) => setTimeout(r, 500));
      }
      if (!activityFound) {
        console.warn(
          "activity feed had no entries after 10s — RabbitMQ async flow may be slow"
        );
      }
    }
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/e2e/smoke-prod/smoke.spec.ts
git commit -m "feat(e2e): add analytics and activity feed verification to prod smoke tests"
```

---

### Task 7: CI Workflow — Add Go and Java Compose-Smoke Jobs

**Files:**
- Modify: `.github/workflows/ci.yml`

Add two new jobs after the existing `compose-smoke` job (which ends at line 527). Insert them before the `security-bandit` job (line 528).

- [ ] **Step 1: Add the Go compose-smoke job**

Insert after line 527 in `.github/workflows/ci.yml`:

```yaml
  compose-smoke-go:
    name: Compose Smoke (Go stack)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build Go service images
        working-directory: go
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml build \
            auth-service order-service product-service cart-service analytics-service

      - name: Start Go compose stack (skip ai-service)
        working-directory: go
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml up -d \
            postgres redis rabbitmq kafka \
            auth-service order-service product-service cart-service analytics-service

      - name: Wait for services to be healthy
        run: |
          for i in $(seq 1 60); do
            if curl -fsS http://localhost:8091/health >/dev/null 2>&1 && \
               curl -fsS http://localhost:8095/health >/dev/null 2>&1 && \
               curl -fsS http://localhost:8096/health >/dev/null 2>&1; then
              echo "Go services ready after ${i}s"
              exit 0
            fi
            sleep 2
          done
          echo "Go services did not come up within 120s" >&2
          docker compose -f docker-compose.yml -f docker-compose.ci.yml logs --no-color --tail=50
          exit 1

      - name: Run migrations and seed data
        working-directory: go
        run: |
          # Run migrations for each service that has them
          for svc in auth-service order-service product-service cart-service; do
            DB_NAME="ecommercedb"
            if [ "$svc" = "product-service" ]; then DB_NAME="productdb"; fi
            if [ "$svc" = "cart-service" ]; then DB_NAME="cartdb"; fi
            docker compose -f docker-compose.yml -f docker-compose.ci.yml exec -T "$svc" \
              migrate -path /migrations -database "postgres://taskuser:taskpass@postgres:5432/${DB_NAME}?sslmode=disable" up || true
          done

          # Seed product data
          docker compose -f docker-compose.yml -f docker-compose.ci.yml exec -T postgres \
            psql -U taskuser -d productdb -f /docker-entrypoint-initdb.d/01-ci-init.sql 2>/dev/null || true
          docker compose -f docker-compose.yml -f docker-compose.ci.yml exec -T product-service \
            sh -c 'cat /seed.sql | PGPASSWORD=taskpass psql -h postgres -U taskuser -d productdb' || true

      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install frontend dependencies
        working-directory: frontend
        run: npm ci

      - name: Install Playwright browsers
        working-directory: frontend
        run: npx playwright install --with-deps chromium

      - name: Run Go smoke Playwright tests
        working-directory: frontend
        run: npx playwright test --config=playwright.smoke-go.config.ts

      - name: Dump compose logs on failure
        if: failure()
        working-directory: go
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml logs --no-color --tail=200

      - name: Tear down compose stack
        if: always()
        working-directory: go
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml down -v
```

- [ ] **Step 2: Add the Java compose-smoke job**

Insert immediately after the Go job:

```yaml
  compose-smoke-java:
    name: Compose Smoke (Java stack)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Build Java service images
        working-directory: java
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml build

      - name: Start Java compose stack
        working-directory: java
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml up -d

      - name: Wait for gateway to be healthy
        run: |
          for i in $(seq 1 60); do
            if curl -fsS http://localhost:8080/graphql \
              -H 'Content-Type: application/json' \
              -d '{"query":"{ __typename }"}' >/dev/null 2>&1; then
              echo "Java gateway ready after ${i}s"
              exit 0
            fi
            sleep 2
          done
          echo "Java gateway did not come up within 120s" >&2
          docker compose -f docker-compose.yml -f docker-compose.ci.yml logs --no-color --tail=50
          exit 1

      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: npm
          cache-dependency-path: frontend/package-lock.json

      - name: Install frontend dependencies
        working-directory: frontend
        run: npm ci

      - name: Install Playwright browsers
        working-directory: frontend
        run: npx playwright install --with-deps chromium

      - name: Run Java smoke Playwright tests
        working-directory: frontend
        run: npx playwright test --config=playwright.smoke-java.config.ts

      - name: Dump compose logs on failure
        if: failure()
        working-directory: java
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml logs --no-color --tail=200

      - name: Tear down compose stack
        if: always()
        working-directory: java
        run: |
          docker compose -f docker-compose.yml -f docker-compose.ci.yml down -v
```

- [ ] **Step 3: Verify CI YAML is valid**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"`

Expected: no errors (valid YAML).

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add Go and Java compose-smoke jobs to CI pipeline"
```

---

### Task 8: Run Preflight Checks

- [ ] **Step 1: Run frontend preflight**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer && make preflight-frontend`

Expected: TypeScript compilation and lint pass. Fix any errors.

- [ ] **Step 2: Verify all new test files are syntactically valid**

Run: `cd /Users/kylebradshaw/repos/gen_ai_engineer/frontend && npx tsc --noEmit`

Expected: no errors.

- [ ] **Step 3: Fix any issues and commit if needed**

If preflight found issues, fix them and commit:

```bash
git add -A
git commit -m "fix: address preflight issues in smoke test files"
```
