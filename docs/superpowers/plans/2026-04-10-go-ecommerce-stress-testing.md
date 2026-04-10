# Go Ecommerce Stress Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a k6 load testing suite for the Go ecommerce/auth/AI services, integrate metrics with Prometheus/Grafana, run tests to find bottlenecks, apply targeted fixes, and document findings in an ADR.

**Architecture:** k6 runs on the Mac, hits services through the SSH tunnel (`localhost:8000`). k6 pushes metrics to Prometheus via remote-write (requires enabling the receiver flag in the Prometheus ConfigMap). A dedicated Grafana dashboard correlates k6 load metrics with existing service metrics. After testing, data-driven fixes are applied and re-tested.

**Tech Stack:** k6 (load testing), Prometheus (metrics), Grafana (dashboards), Go (service fixes), Kubernetes (HPA manifests)

---

## File Structure

```
loadtest/
├── scripts/
│   ├── phase1-ecommerce.js    # Product browse, cart ops, checkout, stock contention
│   ├── phase2-auth.js         # Registration burst, login sustained, token refresh
│   └── phase3-ai-agent.js     # Simple queries, multi-step flows, rate limiter
├── lib/
│   └── helpers.js             # Auth helpers, random data generators, base URL config
├── dashboards/
│   └── k6-load-test.json      # Grafana dashboard for k6 + service correlation
└── README.md                  # How to install, run, and interpret results

# Modified files (performance fixes — only after test data confirms issues):
go/ecommerce-service/cmd/server/main.go                    # pgxpool config, server timeouts
go/ecommerce-service/internal/repository/product.go        # Stock decrement fix
go/ecommerce-service/internal/worker/order_processor.go    # Worker concurrency (env var)
go/k8s/deployments/ecommerce-service.yml                   # Resource tuning
go/k8s/deployments/auth-service.yml                        # Resource tuning
go/k8s/hpa/                                                # New HPA manifests
k8s/monitoring/configmaps/prometheus-config.yml             # Enable remote-write receiver
```

---

### Task 1: Install k6 and Enable Prometheus Remote-Write

**Files:**
- Modify: `k8s/monitoring/configmaps/prometheus-config.yml`
- Modify: `k8s/monitoring/deployments/prometheus.yml`

- [ ] **Step 1: Install k6 on Mac**

Run:
```bash
brew install k6
```

Verify:
```bash
k6 version
```
Expected: version output like `k6 v0.50.0`

- [ ] **Step 2: Enable Prometheus remote-write receiver**

Prometheus needs the `--web.enable-remote-write-receiver` flag to accept k6 metrics. Edit `k8s/monitoring/deployments/prometheus.yml` to add the flag to the container args.

Find the prometheus container args and add `--web.enable-remote-write-receiver` to the list. The args section should look like:

```yaml
args:
  - "--config.file=/etc/prometheus/prometheus.yml"
  - "--storage.tsdb.path=/prometheus"
  - "--storage.tsdb.retention.time=30d"
  - "--web.enable-remote-write-receiver"
```

- [ ] **Step 3: Verify Prometheus is reachable through the tunnel**

The SSH tunnel forwards localhost:8000 to the Windows PC's nginx, which routes to Minikube. Prometheus is at port 9090 in the monitoring namespace. We need to check if Prometheus is exposed through the ingress or if we need a separate tunnel.

Run:
```bash
# Check if Prometheus port is available through existing tunnel
curl -s http://localhost:8000/grafana/api/health | head -5
```

If Grafana is reachable, we can configure k6 to push to Prometheus through an SSH port-forward:
```bash
# Open a separate SSH tunnel for Prometheus remote-write
ssh -f -N -L 9090:localhost:9090 PC@100.79.113.84
```

Then verify:
```bash
curl -s http://localhost:9090/api/v1/status/config | head -5
```

- [ ] **Step 4: Commit the Prometheus config change**

```bash
git add k8s/monitoring/deployments/prometheus.yml
git commit -m "feat(monitoring): enable Prometheus remote-write receiver for k6 integration"
```

---

### Task 2: Shared Helper Library

**Files:**
- Create: `loadtest/lib/helpers.js`

- [ ] **Step 1: Create the helpers module**

```javascript
import http from "k6/http";

// Base URL — services accessed through SSH tunnel to Windows PC
// nginx routes: /go-api/* → ecommerce:8092, /go-auth/* → auth:8091, /ai-api/* → ai:8093
export const BASE_URL = __ENV.BASE_URL || "http://localhost:8000";

export const ECOMMERCE_URL = `${BASE_URL}/go-api`;
export const AUTH_URL = `${BASE_URL}/go-auth`;
export const AI_URL = `${BASE_URL}/ai-api`;

// Default headers
export function authHeaders(token) {
  return {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
  };
}

export function jsonHeaders() {
  return {
    headers: { "Content-Type": "application/json" },
  };
}

// Register a unique test user and return { accessToken, refreshToken, userId }
export function registerUser() {
  const uniqueId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const payload = JSON.stringify({
    email: `loadtest-${uniqueId}@test.local`,
    password: "LoadTest123!",
    name: `Load Test ${uniqueId}`,
  });

  const res = http.post(`${AUTH_URL}/auth/register`, payload, jsonHeaders());
  if (res.status !== 200 && res.status !== 201) {
    console.error(`Registration failed: ${res.status} ${res.body}`);
    return null;
  }

  const body = JSON.parse(res.body);
  return {
    accessToken: body.accessToken,
    refreshToken: body.refreshToken,
    userId: body.userId,
  };
}

// Login with existing credentials, return { accessToken, refreshToken, userId }
export function loginUser(email, password) {
  const payload = JSON.stringify({ email, password });
  const res = http.post(`${AUTH_URL}/auth/login`, payload, jsonHeaders());
  if (res.status !== 200) {
    console.error(`Login failed: ${res.status} ${res.body}`);
    return null;
  }

  const body = JSON.parse(res.body);
  return {
    accessToken: body.accessToken,
    refreshToken: body.refreshToken,
    userId: body.userId,
  };
}

// Fetch product list and return array of product objects
export function getProducts(params) {
  const query = params
    ? "?" + Object.entries(params).map(([k, v]) => `${k}=${v}`).join("&")
    : "";
  const res = http.get(`${ECOMMERCE_URL}/products${query}`);
  if (res.status !== 200) {
    return [];
  }
  return JSON.parse(res.body).products || [];
}

// Pick a random element from an array
export function randomItem(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

// Pick N random elements from an array (no duplicates)
export function randomItems(arr, n) {
  const shuffled = [...arr].sort(() => 0.5 - Math.random());
  return shuffled.slice(0, Math.min(n, arr.length));
}
```

- [ ] **Step 2: Verify the file was created**

Run:
```bash
ls -la loadtest/lib/helpers.js
```
Expected: file exists

- [ ] **Step 3: Commit**

```bash
git add loadtest/lib/helpers.js
git commit -m "feat(loadtest): add shared k6 helper library with auth and data utilities"
```

---

### Task 3: Phase 1 — Ecommerce Load Test Script

**Files:**
- Create: `loadtest/scripts/phase1-ecommerce.js`

- [ ] **Step 1: Create the phase 1 script with all four scenarios**

```javascript
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import {
  ECOMMERCE_URL,
  authHeaders,
  registerUser,
  getProducts,
  randomItem,
  randomItems,
} from "../lib/helpers.js";

// Custom metrics
const stockOverSell = new Counter("stock_oversell_count");
const checkoutSuccess = new Counter("checkout_success_total");
const checkoutFail = new Counter("checkout_fail_total");
const cartOpDuration = new Trend("cart_operation_duration", true);

// --- Scenario Configuration ---
// Run a specific scenario via: k6 run --env SCENARIO=browse scripts/phase1-ecommerce.js
// Or run all scenarios together (default).

const scenarios = {
  browse: {
    executor: "ramping-vus",
    startVUs: 1,
    stages: [
      { duration: "2m", target: 50 },
      { duration: "2m", target: 50 },
      { duration: "1m", target: 0 },
    ],
    exec: "browseProducts",
    tags: { scenario: "browse" },
  },
  cart: {
    executor: "constant-vus",
    vus: 20,
    duration: "3m",
    exec: "cartOperations",
    tags: { scenario: "cart" },
  },
  checkout: {
    executor: "constant-vus",
    vus: 30,
    duration: "3m",
    exec: "checkoutFlow",
    tags: { scenario: "checkout" },
  },
  stockContention: {
    executor: "constant-vus",
    vus: 50,
    duration: "1m",
    exec: "stockContention",
    tags: { scenario: "stock_contention" },
  },
};

// Allow running a single scenario or all
const selected = __ENV.SCENARIO;
export const options = {
  scenarios: selected ? { [selected]: scenarios[selected] } : scenarios,
  thresholds: {
    "http_req_duration{scenario:browse}": ["p(95)<500"],
    "http_req_duration{scenario:cart}": ["p(95)<500"],
    "http_req_duration{scenario:checkout}": ["p(95)<1000"],
    "http_req_failed{scenario:browse}": ["rate<0.01"],
    "http_req_failed{scenario:cart}": ["rate<0.01"],
    "http_req_failed{scenario:checkout}": ["rate<0.01"],
  },
};

// --- Scenario A: Browse Products ---
export function browseProducts() {
  group("browse_products", function () {
    // List products (default page)
    const listRes = http.get(`${ECOMMERCE_URL}/products`);
    check(listRes, {
      "product list 200": (r) => r.status === 200,
      "has products": (r) => JSON.parse(r.body).products.length > 0,
    });

    // Get categories
    const catRes = http.get(`${ECOMMERCE_URL}/categories`);
    check(catRes, {
      "categories 200": (r) => r.status === 200,
    });

    // Browse by category
    const categories = JSON.parse(catRes.body).categories || [];
    if (categories.length > 0) {
      const cat = randomItem(categories);
      http.get(`${ECOMMERCE_URL}/products?category=${cat}`);
    }

    // View a single product
    const products = JSON.parse(listRes.body).products || [];
    if (products.length > 0) {
      const product = randomItem(products);
      const detailRes = http.get(`${ECOMMERCE_URL}/products/${product.id}`);
      check(detailRes, {
        "product detail 200": (r) => r.status === 200,
      });
    }

    // Paginate
    http.get(`${ECOMMERCE_URL}/products?page=2&limit=10`);
    http.get(`${ECOMMERCE_URL}/products?sort=price_asc&limit=10`);
  });

  sleep(1);
}

// --- Scenario B: Cart Operations ---
export function cartOperations() {
  const auth = registerUser();
  if (!auth) return;

  const products = getProducts({ limit: 20 });
  if (products.length === 0) return;

  group("cart_operations", function () {
    const items = randomItems(products, 3);

    // Add items to cart
    for (const item of items) {
      const start = Date.now();
      const res = http.post(
        `${ECOMMERCE_URL}/cart`,
        JSON.stringify({ productId: item.id, quantity: 1 }),
        authHeaders(auth.accessToken)
      );
      cartOpDuration.add(Date.now() - start);
      check(res, {
        "add to cart 200": (r) => r.status === 200 || r.status === 201,
      });
    }

    // View cart
    const cartRes = http.get(
      `${ECOMMERCE_URL}/cart`,
      authHeaders(auth.accessToken)
    );
    check(cartRes, {
      "view cart 200": (r) => r.status === 200,
      "cart has items": (r) => JSON.parse(r.body).items.length > 0,
    });

    // Update quantity on first item
    const cartItems = JSON.parse(cartRes.body).items || [];
    if (cartItems.length > 0) {
      const start = Date.now();
      http.put(
        `${ECOMMERCE_URL}/cart/${cartItems[0].id}`,
        JSON.stringify({ quantity: 2 }),
        authHeaders(auth.accessToken)
      );
      cartOpDuration.add(Date.now() - start);
    }

    // Remove last item
    if (cartItems.length > 1) {
      http.del(
        `${ECOMMERCE_URL}/cart/${cartItems[cartItems.length - 1].id}`,
        null,
        authHeaders(auth.accessToken)
      );
    }
  });

  sleep(1);
}

// --- Scenario C: Checkout Flow ---
export function checkoutFlow() {
  const auth = registerUser();
  if (!auth) return;

  const products = getProducts({ limit: 20 });
  if (products.length === 0) return;

  group("checkout_flow", function () {
    // Add 1-2 items to cart
    const items = randomItems(products, 2);
    for (const item of items) {
      http.post(
        `${ECOMMERCE_URL}/cart`,
        JSON.stringify({ productId: item.id, quantity: 1 }),
        authHeaders(auth.accessToken)
      );
    }

    // Checkout
    const orderRes = http.post(
      `${ECOMMERCE_URL}/orders`,
      null,
      authHeaders(auth.accessToken)
    );
    const orderOk = check(orderRes, {
      "checkout 200": (r) => r.status === 200 || r.status === 201,
    });

    if (orderOk) {
      checkoutSuccess.add(1);
      const order = JSON.parse(orderRes.body);

      // Verify order appears in list
      sleep(0.5);
      const listRes = http.get(
        `${ECOMMERCE_URL}/orders`,
        authHeaders(auth.accessToken)
      );
      check(listRes, {
        "order in list": (r) => r.body.includes(order.id),
      });

      // Fetch order detail
      http.get(
        `${ECOMMERCE_URL}/orders/${order.id}`,
        authHeaders(auth.accessToken)
      );
    } else {
      checkoutFail.add(1);
    }
  });

  sleep(1);
}

// --- Scenario D: Stock Contention ---
// All VUs race to buy the same low-stock item.
// After the test, check: successful orders should not exceed available stock.
export function stockContention() {
  const auth = registerUser();
  if (!auth) return;

  // All VUs target the first product (seed data has stock=50 for headphones).
  // We use a known product — fetch the first one.
  const products = getProducts({ limit: 1 });
  if (products.length === 0) return;
  const target = products[0];

  group("stock_contention", function () {
    // Add the target item to cart
    const addRes = http.post(
      `${ECOMMERCE_URL}/cart`,
      JSON.stringify({ productId: target.id, quantity: 1 }),
      authHeaders(auth.accessToken)
    );

    if (addRes.status === 200 || addRes.status === 201) {
      // Attempt checkout
      const orderRes = http.post(
        `${ECOMMERCE_URL}/orders`,
        null,
        authHeaders(auth.accessToken)
      );

      if (orderRes.status === 200 || orderRes.status === 201) {
        checkoutSuccess.add(1);
      } else {
        checkoutFail.add(1);
        // Check if this is a stock-related error
        if (orderRes.body && orderRes.body.includes("stock")) {
          // Expected behavior — stock exhausted
        } else {
          stockOverSell.add(1);
        }
      }
    }
  });

  // No sleep — we want maximum contention
}
```

- [ ] **Step 2: Dry-run the script to check for syntax errors**

Run:
```bash
k6 inspect loadtest/scripts/phase1-ecommerce.js
```
Expected: JSON output of test options without errors

- [ ] **Step 3: Commit**

```bash
git add loadtest/scripts/phase1-ecommerce.js
git commit -m "feat(loadtest): add phase 1 ecommerce k6 stress test scenarios"
```

---

### Task 4: Phase 2 — Auth Load Test Script

**Files:**
- Create: `loadtest/scripts/phase2-auth.js`

- [ ] **Step 1: Create the phase 2 script**

```javascript
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Trend } from "k6/metrics";
import { AUTH_URL, jsonHeaders, registerUser } from "../lib/helpers.js";

// Custom metrics
const bcryptDuration = new Trend("bcrypt_operation_duration", true);

const scenarios = {
  registrationBurst: {
    executor: "constant-vus",
    vus: 50,
    duration: "1m",
    exec: "registrationBurst",
    tags: { scenario: "registration_burst" },
  },
  loginSustained: {
    executor: "constant-arrival-rate",
    rate: 20,
    timeUnit: "1s",
    duration: "3m",
    preAllocatedVUs: 50,
    maxVUs: 100,
    exec: "loginSustained",
    tags: { scenario: "login_sustained" },
  },
  tokenRefresh: {
    executor: "constant-vus",
    vus: 30,
    duration: "2m",
    exec: "tokenRefresh",
    tags: { scenario: "token_refresh" },
  },
};

const selected = __ENV.SCENARIO;
export const options = {
  scenarios: selected ? { [selected]: scenarios[selected] } : scenarios,
  thresholds: {
    "http_req_duration{scenario:registration_burst}": ["p(95)<3000"],
    "http_req_duration{scenario:login_sustained}": ["p(95)<2000"],
    "http_req_duration{scenario:token_refresh}": ["p(95)<500"],
    "http_req_failed{scenario:login_sustained}": ["rate<0.01"],
    "http_req_failed{scenario:token_refresh}": ["rate<0.01"],
  },
};

// Pre-create a pool of users during setup for login and refresh scenarios.
// Each VU gets its own credentials to avoid auth conflicts.
const userPool = [];

export function setup() {
  // Create 100 users for login and refresh tests
  const users = [];
  for (let i = 0; i < 100; i++) {
    const uniqueId = `${Date.now()}-${i}-${Math.random().toString(36).slice(2, 8)}`;
    const email = `loadtest-pool-${uniqueId}@test.local`;
    const password = "LoadTest123!";

    const res = http.post(
      `${AUTH_URL}/auth/register`,
      JSON.stringify({ email, password, name: `Pool User ${i}` }),
      jsonHeaders()
    );

    if (res.status === 200 || res.status === 201) {
      const body = JSON.parse(res.body);
      users.push({
        email,
        password,
        refreshToken: body.refreshToken,
      });
    }
  }
  return { users };
}

// --- Scenario A: Registration Burst ---
export function registrationBurst() {
  group("registration_burst", function () {
    const uniqueId = `${Date.now()}-${__VU}-${__ITER}-${Math.random().toString(36).slice(2, 8)}`;
    const payload = JSON.stringify({
      email: `burst-${uniqueId}@test.local`,
      password: "LoadTest123!",
      name: `Burst User ${uniqueId}`,
    });

    const start = Date.now();
    const res = http.post(`${AUTH_URL}/auth/register`, payload, jsonHeaders());
    bcryptDuration.add(Date.now() - start);

    check(res, {
      "register 200": (r) => r.status === 200 || r.status === 201,
      "has access token": (r) => {
        const body = JSON.parse(r.body);
        return body.accessToken && body.accessToken.length > 0;
      },
    });
  });
}

// --- Scenario B: Login Sustained Load ---
export function loginSustained(data) {
  if (!data.users || data.users.length === 0) return;

  const user = data.users[__VU % data.users.length];

  group("login_sustained", function () {
    const start = Date.now();
    const res = http.post(
      `${AUTH_URL}/auth/login`,
      JSON.stringify({ email: user.email, password: user.password }),
      jsonHeaders()
    );
    bcryptDuration.add(Date.now() - start);

    check(res, {
      "login 200": (r) => r.status === 200,
      "has tokens": (r) => {
        const body = JSON.parse(r.body);
        return body.accessToken && body.refreshToken;
      },
    });
  });
}

// --- Scenario C: Token Refresh ---
export function tokenRefresh(data) {
  if (!data.users || data.users.length === 0) return;

  const user = data.users[__VU % data.users.length];

  group("token_refresh", function () {
    // Login first to get a fresh refresh token
    const loginRes = http.post(
      `${AUTH_URL}/auth/login`,
      JSON.stringify({ email: user.email, password: user.password }),
      jsonHeaders()
    );

    if (loginRes.status !== 200) return;
    const tokens = JSON.parse(loginRes.body);

    // Now refresh the token
    const res = http.post(
      `${AUTH_URL}/auth/refresh`,
      JSON.stringify({ refreshToken: tokens.refreshToken }),
      jsonHeaders()
    );

    check(res, {
      "refresh 200": (r) => r.status === 200,
      "new access token": (r) => {
        const body = JSON.parse(r.body);
        return body.accessToken && body.accessToken !== tokens.accessToken;
      },
    });
  });

  sleep(1);
}
```

- [ ] **Step 2: Dry-run syntax check**

Run:
```bash
k6 inspect loadtest/scripts/phase2-auth.js
```
Expected: JSON output without errors

- [ ] **Step 3: Commit**

```bash
git add loadtest/scripts/phase2-auth.js
git commit -m "feat(loadtest): add phase 2 auth service k6 stress test scenarios"
```

---

### Task 5: Phase 3 — AI Agent Load Test Script

**Files:**
- Create: `loadtest/scripts/phase3-ai-agent.js`

- [ ] **Step 1: Create the phase 3 script**

The AI service uses Server-Sent Events (SSE). k6 doesn't natively parse SSE, but since the endpoint returns a streaming HTTP response, we can send the request and measure the full response time. The response body will contain the concatenated SSE events.

```javascript
import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import { AI_URL, AUTH_URL, jsonHeaders, registerUser } from "../lib/helpers.js";

// Custom metrics
const rateLimited = new Counter("rate_limited_total");
const agentTurnDuration = new Trend("agent_turn_duration", true);

const scenarios = {
  simpleQuery: {
    executor: "constant-vus",
    vus: 10,
    duration: "3m",
    exec: "simpleQuery",
    tags: { scenario: "ai_simple" },
  },
  multiStep: {
    executor: "constant-vus",
    vus: 5,
    duration: "3m",
    exec: "multiStepFlow",
    tags: { scenario: "ai_multistep" },
  },
  rateLimiter: {
    executor: "constant-vus",
    vus: 5,
    duration: "2m",
    exec: "rateLimiterTest",
    tags: { scenario: "ai_ratelimit" },
  },
};

const selected = __ENV.SCENARIO;
export const options = {
  scenarios: selected ? { [selected]: scenarios[selected] } : scenarios,
  thresholds: {
    "http_req_duration{scenario:ai_simple}": ["p(95)<15000"],
    "http_req_duration{scenario:ai_multistep}": ["p(95)<30000"],
  },
};

// Simple product search queries that should trigger SearchProducts tool (cached after first call)
const simpleQueries = [
  "What electronics do you have?",
  "Show me books under $40",
  "What sports equipment is available?",
  "Do you have any headphones?",
  "What clothing items do you sell?",
  "Show me home products",
  "What are your most popular items?",
  "Do you have any keyboards?",
];

// Multi-step queries requiring auth + multiple tool calls
const multiStepQueries = [
  "Search for headphones and add the first result to my cart",
  "Find me a book about Go programming and add it to my cart",
  "What's in my cart right now?",
  "Search for electronics under $100 and check inventory on the cheapest one",
];

function sendChatMessage(messages, token) {
  const headers = { "Content-Type": "application/json" };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const payload = JSON.stringify({ messages });

  const start = Date.now();
  const res = http.post(`${AI_URL}/chat`, payload, {
    headers,
    timeout: "60s",
  });
  agentTurnDuration.add(Date.now() - start);

  return res;
}

// --- Scenario A: Simple Queries (Cached Tools) ---
export function simpleQuery() {
  group("ai_simple_query", function () {
    const query = simpleQueries[Math.floor(Math.random() * simpleQueries.length)];

    const res = sendChatMessage(
      [{ role: "user", content: query }],
      null
    );

    check(res, {
      "chat 200": (r) => r.status === 200,
      "has response body": (r) => r.body && r.body.length > 0,
      "not rate limited": (r) => r.status !== 429,
    });

    if (res.status === 429) {
      rateLimited.add(1);
    }
  });

  // Longer sleep — Ollama is slow, avoid queuing too many requests
  sleep(3);
}

// --- Scenario B: Multi-Step Flows ---
export function multiStepFlow() {
  const auth = registerUser();
  if (!auth) return;

  group("ai_multistep_flow", function () {
    const query = multiStepQueries[Math.floor(Math.random() * multiStepQueries.length)];

    const res = sendChatMessage(
      [{ role: "user", content: query }],
      auth.accessToken
    );

    check(res, {
      "chat 200": (r) => r.status === 200,
      "has tool calls": (r) => r.body && r.body.includes("tool_call"),
      "not rate limited": (r) => r.status !== 429,
    });

    if (res.status === 429) {
      rateLimited.add(1);
    }
  });

  sleep(5);
}

// --- Scenario C: Rate Limiter Behavior ---
export function rateLimiterTest() {
  group("ai_rate_limiter", function () {
    // Fire requests rapidly to exceed the 20 req/min limit
    for (let i = 0; i < 5; i++) {
      const res = sendChatMessage(
        [{ role: "user", content: "What products do you have?" }],
        null
      );

      if (res.status === 429) {
        rateLimited.add(1);
        check(res, {
          "429 has retry-after": (r) =>
            r.headers["Retry-After"] !== undefined ||
            r.headers["retry-after"] !== undefined,
        });
        // Stop hammering once rate limited
        sleep(5);
        return;
      }

      check(res, {
        "request ok before limit": (r) => r.status === 200,
      });
    }
  });

  sleep(2);
}
```

- [ ] **Step 2: Dry-run syntax check**

Run:
```bash
k6 inspect loadtest/scripts/phase3-ai-agent.js
```
Expected: JSON output without errors

- [ ] **Step 3: Commit**

```bash
git add loadtest/scripts/phase3-ai-agent.js
git commit -m "feat(loadtest): add phase 3 AI agent k6 stress test scenarios"
```

---

### Task 6: Grafana Load Test Dashboard

**Files:**
- Create: `loadtest/dashboards/k6-load-test.json`

- [ ] **Step 1: Create the Grafana dashboard JSON**

This dashboard combines k6 remote-write metrics (prefixed `k6_`) with the existing service metrics. k6 remote-write sends metrics as: `k6_http_req_duration`, `k6_http_reqs`, `k6_vus`, `k6_iterations`, etc.

```json
{
  "dashboard": {
    "id": null,
    "uid": "k6-load-test",
    "title": "k6 Load Test Results",
    "tags": ["k6", "load-test", "go"],
    "timezone": "browser",
    "refresh": "5s",
    "time": { "from": "now-30m", "to": "now" },
    "panels": [
      {
        "title": "── k6 Load Generator ──",
        "type": "row",
        "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
        "collapsed": false
      },
      {
        "title": "Virtual Users",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 0, "y": 1 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "k6_vus",
            "legendFormat": "VUs"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "short", "color": { "mode": "palette-classic" } }
        }
      },
      {
        "title": "Request Rate",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 8, "y": 1 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "rate(k6_http_reqs_total[30s])",
            "legendFormat": "req/s"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "reqps" }
        }
      },
      {
        "title": "Error Rate",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 16, "y": 1 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "rate(k6_http_req_failed_total[30s]) / rate(k6_http_reqs_total[30s]) * 100",
            "legendFormat": "error %"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "percent", "max": 100 }
        }
      },
      {
        "title": "Response Time Percentiles",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 12, "x": 0, "y": 9 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "k6_http_req_duration_p50",
            "legendFormat": "p50"
          },
          {
            "expr": "k6_http_req_duration_p95",
            "legendFormat": "p95"
          },
          {
            "expr": "k6_http_req_duration_p99",
            "legendFormat": "p99"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "ms" }
        }
      },
      {
        "title": "Requests by Status Code",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 12, "x": 12, "y": 9 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "rate(k6_http_reqs_total[30s])",
            "legendFormat": "{{ expected_response }}"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "reqps" }
        }
      },
      {
        "title": "── Service Side (During Load) ──",
        "type": "row",
        "gridPos": { "h": 1, "w": 24, "x": 0, "y": 17 },
        "collapsed": false
      },
      {
        "title": "Service Request Rate",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 0, "y": 18 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "sum(rate(http_requests_total{service=~\"go-.*\"}[30s])) by (service)",
            "legendFormat": "{{ service }}"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "reqps" }
        }
      },
      {
        "title": "Service p95 Latency",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 8, "y": 18 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{service=~\"go-.*\"}[30s])) by (le, service))",
            "legendFormat": "{{ service }} p95"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "s" }
        }
      },
      {
        "title": "Service Error Rate (5xx)",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 16, "y": 18 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "sum(rate(http_requests_total{service=~\"go-.*\",status=~\"5..\"}[30s])) by (service) / sum(rate(http_requests_total{service=~\"go-.*\"}[30s])) by (service) * 100",
            "legendFormat": "{{ service }} 5xx %"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "percent" }
        }
      },
      {
        "title": "── Correlation ──",
        "type": "row",
        "gridPos": { "h": 1, "w": 24, "x": 0, "y": 26 },
        "collapsed": false
      },
      {
        "title": "Cache Hit Rate Under Load",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 0, "y": 27 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "rate(ecommerce_cache_operations_total{result=\"hit\"}[30s]) / (rate(ecommerce_cache_operations_total{result=\"hit\"}[30s]) + rate(ecommerce_cache_operations_total{result=\"miss\"}[30s])) * 100",
            "legendFormat": "cache hit %"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "percent", "max": 100 }
        }
      },
      {
        "title": "Orders vs Worker Processing",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 8, "y": 27 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "rate(ecommerce_orders_placed_total[30s]) * 60",
            "legendFormat": "orders created/min"
          },
          {
            "expr": "rate(orders_total[30s]) * 60",
            "legendFormat": "orders processed/min"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "short" }
        }
      },
      {
        "title": "Go Runtime (Goroutines + Heap)",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 16, "y": 27 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "go_goroutines{service=~\"go-.*\"}",
            "legendFormat": "{{ service }} goroutines"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "short" }
        }
      },
      {
        "title": "── AI Agent (During Load) ──",
        "type": "row",
        "gridPos": { "h": 1, "w": 24, "x": 0, "y": 35 },
        "collapsed": false
      },
      {
        "title": "Agent Turn Duration",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 0, "y": 36 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "histogram_quantile(0.50, sum(rate(ai_agent_turn_duration_seconds_bucket[30s])) by (le))",
            "legendFormat": "p50"
          },
          {
            "expr": "histogram_quantile(0.95, sum(rate(ai_agent_turn_duration_seconds_bucket[30s])) by (le))",
            "legendFormat": "p95"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "s" }
        }
      },
      {
        "title": "Tool Call Rate by Tool",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 8, "y": 36 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "sum(rate(ai_tool_calls_total[30s])) by (name)",
            "legendFormat": "{{ name }}"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "reqps" }
        }
      },
      {
        "title": "Ollama Token Throughput",
        "type": "timeseries",
        "gridPos": { "h": 8, "w": 8, "x": 16, "y": 36 },
        "datasource": { "type": "prometheus", "uid": "prometheus" },
        "targets": [
          {
            "expr": "sum(rate(ollama_tokens_total[30s])) by (kind)",
            "legendFormat": "{{ kind }} tokens/s"
          }
        ],
        "fieldConfig": {
          "defaults": { "unit": "short" }
        }
      }
    ]
  },
  "overwrite": true
}
```

- [ ] **Step 2: Commit**

```bash
git add loadtest/dashboards/k6-load-test.json
git commit -m "feat(loadtest): add Grafana dashboard for k6 load test correlation"
```

---

### Task 7: README

**Files:**
- Create: `loadtest/README.md`

- [ ] **Step 1: Create the README**

```markdown
# Load Testing Suite

k6-based stress tests for the Go ecommerce, auth, and AI agent services.

## Prerequisites

```bash
brew install k6
```

Services must be running (either locally via Docker Compose or on Minikube via SSH tunnel).

## Quick Start

```bash
# Ensure SSH tunnel is active
ssh -f -N -L 8000:localhost:8000 PC@100.79.113.84

# Run a single phase
k6 run loadtest/scripts/phase1-ecommerce.js

# Run a single scenario within a phase
k6 run --env SCENARIO=browse loadtest/scripts/phase1-ecommerce.js
k6 run --env SCENARIO=stockContention loadtest/scripts/phase1-ecommerce.js

# Override base URL (e.g., for local Docker Compose)
k6 run --env BASE_URL=http://localhost:8092 loadtest/scripts/phase1-ecommerce.js
```

## Pushing Metrics to Prometheus

To see k6 metrics in Grafana alongside service metrics:

```bash
# Open SSH tunnel for Prometheus (separate from the nginx tunnel)
ssh -f -N -L 9090:localhost:9090 PC@100.79.113.84

# Run with Prometheus remote-write output
k6 run \
  -o experimental-prometheus-rw \
  --env K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  loadtest/scripts/phase1-ecommerce.js
```

Then open the "k6 Load Test Results" dashboard in Grafana.

## Phases

| Phase | Script | What it tests |
|-------|--------|---------------|
| 1 | `phase1-ecommerce.js` | Product browsing, cart ops, checkout, stock contention |
| 2 | `phase2-auth.js` | Registration burst, login sustained load, token refresh |
| 3 | `phase3-ai-agent.js` | Simple AI queries, multi-step flows, rate limiter |

## Scenarios

Each script supports `--env SCENARIO=<name>` to run a single scenario:

**Phase 1:** `browse`, `cart`, `checkout`, `stockContention`
**Phase 2:** `registrationBurst`, `loginSustained`, `tokenRefresh`
**Phase 3:** `simpleQuery`, `multiStep`, `rateLimiter`

## Thresholds

| Endpoint | p95 Target | Error Rate |
|----------|-----------|------------|
| Product browse | < 500ms | < 1% |
| Cart operations | < 500ms | < 1% |
| Checkout | < 1s | < 1% |
| Auth login | < 2s | < 1% |
| AI agent turn | < 15s | N/A |

## Grafana Dashboard

Import `dashboards/k6-load-test.json` into Grafana, or copy it to the Grafana provisioning directory on the Windows PC.
```

- [ ] **Step 2: Commit**

```bash
git add loadtest/README.md
git commit -m "docs(loadtest): add README with setup and usage instructions"
```

---

### Task 8: Run Baseline Tests and Capture Results

This task is manual/interactive — run each phase at low load to establish baseline metrics.

- [ ] **Step 1: Run phase 1 browse scenario at low VUs**

Run:
```bash
k6 run --env SCENARIO=browse --vus 3 --duration 1m loadtest/scripts/phase1-ecommerce.js
```

Capture the summary output (p50, p95, p99, error rate). Save notable numbers for the ADR.

- [ ] **Step 2: Run phase 1 cart scenario at low VUs**

Run:
```bash
k6 run --env SCENARIO=cart --vus 3 --duration 1m loadtest/scripts/phase1-ecommerce.js
```

- [ ] **Step 3: Run phase 1 checkout scenario at low VUs**

Run:
```bash
k6 run --env SCENARIO=checkout --vus 3 --duration 1m loadtest/scripts/phase1-ecommerce.js
```

- [ ] **Step 4: Run phase 2 login scenario at low rate**

Run:
```bash
k6 run --env SCENARIO=loginSustained --vus 3 --duration 1m loadtest/scripts/phase2-auth.js
```

- [ ] **Step 5: Run phase 3 simple query at 2 VUs**

Run:
```bash
k6 run --env SCENARIO=simpleQuery --vus 2 --duration 1m loadtest/scripts/phase3-ai-agent.js
```

- [ ] **Step 6: Note baseline numbers**

Record the baseline p50/p95/p99 response times and error rates from each run. These become the "before" numbers in the ADR.

---

### Task 9: Run Full Stress Tests

Run each phase at full configured load. Analyze results between phases.

- [ ] **Step 1: Run phase 1 full (with Prometheus output)**

Run:
```bash
k6 run \
  -o experimental-prometheus-rw \
  --env K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  loadtest/scripts/phase1-ecommerce.js
```

Note: this runs all four scenarios (browse, cart, checkout, stockContention). Takes ~5 minutes total.

Review output for:
- Any threshold failures (marked with ✗ in k6 output)
- p95 latency spikes
- Error rates above 1%
- `checkout_success_total` vs `checkout_fail_total` in stock contention

- [ ] **Step 2: Run phase 2 full**

Run:
```bash
k6 run \
  -o experimental-prometheus-rw \
  --env K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  loadtest/scripts/phase2-auth.js
```

Review output for:
- bcrypt_operation_duration p95 (expect > 200ms due to bcrypt cost)
- Registration burst error rate
- Login sustained throughput (target: 20 req/s sustained)

- [ ] **Step 3: Run phase 3 full**

Run:
```bash
k6 run \
  -o experimental-prometheus-rw \
  --env K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  loadtest/scripts/phase3-ai-agent.js
```

Review output for:
- Agent turn duration (p95 target < 15s)
- rate_limited_total count
- Whether Ollama saturates (visible in Grafana via ollama_request_duration_seconds)

- [ ] **Step 4: Capture Grafana screenshots**

Take screenshots of the k6 Load Test Results dashboard during/after each phase for the ADR. Save to `docs/adr/images/` or reference inline.

---

### Task 10: Apply Performance Fixes (Data-Driven)

Only apply fixes for issues confirmed by the stress test data. Skip any fix whose bottleneck wasn't observed. Each fix follows the same pattern: fix → re-run the specific scenario → verify improvement.

**Files:**
- Modify: `go/ecommerce-service/cmd/server/main.go`
- Modify: `go/ecommerce-service/internal/repository/product.go` (if stock race confirmed)
- Modify: `go/ecommerce-service/internal/worker/order_processor.go` (if queue backup confirmed)

- [ ] **Step 1: Add pgxpool configuration**

In `go/ecommerce-service/cmd/server/main.go`, replace the bare `pgxpool.New(ctx, databaseURL)` call with explicit pool configuration:

```go
// Replace this (line 66):
pool, err := pgxpool.New(ctx, databaseURL)

// With this:
poolConfig, err := pgxpool.ParseConfig(databaseURL)
if err != nil {
	log.Fatalf("failed to parse database URL: %v", err)
}
poolConfig.MaxConns = 25
poolConfig.MinConns = 5
poolConfig.MaxConnIdleTime = 5 * time.Minute
poolConfig.MaxConnLifetime = 30 * time.Minute
poolConfig.HealthCheckPeriod = 30 * time.Second

pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
```

- [ ] **Step 2: Add HTTP server timeouts**

In `go/ecommerce-service/cmd/server/main.go`, add timeouts to the `http.Server` (lines 170-173):

```go
// Replace this:
srv := &http.Server{
	Addr:    ":" + port,
	Handler: router,
}

// With this:
srv := &http.Server{
	Addr:         ":" + port,
	Handler:      router,
	ReadTimeout:  10 * time.Second,
	WriteTimeout: 30 * time.Second,
	IdleTimeout:  60 * time.Second,
}
```

- [ ] **Step 3: Make worker concurrency configurable**

In `go/ecommerce-service/cmd/server/main.go`, read worker concurrency from an environment variable instead of hardcoding 3 (line 135):

```go
// Replace this:
if err := processor.StartConsumer(ctx, ch, 3); err != nil {

// With this:
workerConcurrency := 3
if v := os.Getenv("WORKER_CONCURRENCY"); v != "" {
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		workerConcurrency = n
	}
}
if err := processor.StartConsumer(ctx, ch, workerConcurrency); err != nil {
```

Add `"strconv"` to the imports.

- [ ] **Step 4: Fix stock decrement race condition (if confirmed)**

Read `go/ecommerce-service/internal/repository/product.go` to find the `DecrementStock` method. Replace the bare UPDATE with a transactional SELECT FOR UPDATE:

```go
func (r *ProductRepository) DecrementStock(ctx context.Context, productID string, quantity int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var stock int
	err = tx.QueryRow(ctx,
		"SELECT stock FROM products WHERE id = $1 FOR UPDATE",
		productID,
	).Scan(&stock)
	if err != nil {
		return fmt.Errorf("select for update: %w", err)
	}

	if stock < quantity {
		return fmt.Errorf("insufficient stock: have %d, need %d", stock, quantity)
	}

	_, err = tx.Exec(ctx,
		"UPDATE products SET stock = stock - $1 WHERE id = $2",
		quantity, productID,
	)
	if err != nil {
		return fmt.Errorf("update stock: %w", err)
	}

	return tx.Commit(ctx)
}
```

Note: Read the actual file first to confirm the existing signature and adjust if needed.

- [ ] **Step 5: Run preflight checks**

Run:
```bash
make preflight-go
```
Expected: all lint and tests pass

- [ ] **Step 6: Re-run the failing scenarios to verify improvement**

Re-run whichever phase 1 scenarios showed issues:
```bash
k6 run --env SCENARIO=<scenario_name> \
  -o experimental-prometheus-rw \
  --env K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  loadtest/scripts/phase1-ecommerce.js
```

Compare p95 latency and error rates against the pre-fix run.

- [ ] **Step 7: Commit fixes**

```bash
git add go/ecommerce-service/cmd/server/main.go go/ecommerce-service/internal/repository/product.go
git commit -m "perf(ecommerce): add pool config, server timeouts, fix stock race condition

Data-driven fixes from k6 stress testing:
- Explicit pgxpool config (25 max, 5 min conns, 5m idle timeout)
- HTTP server read/write/idle timeouts
- Configurable worker concurrency via WORKER_CONCURRENCY env var
- SELECT FOR UPDATE for stock decrement to prevent overselling"
```

---

### Task 11: Add K8s HPA Manifests

**Files:**
- Create: `go/k8s/hpa/ecommerce-hpa.yml`
- Create: `go/k8s/hpa/auth-hpa.yml`

- [ ] **Step 1: Create ecommerce HPA**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ecommerce-service-hpa
  namespace: go-ecommerce
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-ecommerce-service
  minReplicas: 1
  maxReplicas: 3
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
```

- [ ] **Step 2: Create auth HPA**

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: auth-service-hpa
  namespace: go-ecommerce
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: go-auth-service
  minReplicas: 1
  maxReplicas: 3
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 120
```

- [ ] **Step 3: Commit**

```bash
git add go/k8s/hpa/
git commit -m "feat(k8s): add HPA manifests for ecommerce and auth services

CPU-based autoscaling: 70% target, 1-3 replicas, conservative scale-down."
```

---

### Task 12: Write the ADR

**Files:**
- Create: `docs/adr/go-stress-testing.md`

- [ ] **Step 1: Write the ADR with findings**

This ADR should be populated with actual data from the test runs. The template below has placeholders marked `[DATA]` that must be filled with real numbers from the k6 output and Grafana observations.

```markdown
# ADR: Go Ecommerce Stress Testing & Scalability Analysis

**Date:** 2026-04-10
**Status:** Accepted

## Context

The Go ecommerce stack (ecommerce-service, auth-service, ai-service) runs as single-replica deployments in Minikube. Before promoting this as a portfolio piece, we needed to validate performance characteristics under load, identify bottlenecks, and demonstrate scalability thinking.

## Approach

Used k6 to stress test three phases:
1. **Ecommerce** — product browsing, cart operations, checkout flow, stock contention
2. **Auth** — registration burst, sustained login load, token refresh
3. **AI Agent** — simple queries, multi-step tool-calling flows, rate limiter behavior

Tests ran from macOS through an SSH tunnel, matching real-world access patterns. k6 metrics were pushed to Prometheus via remote-write for Grafana correlation with service-side metrics.

## Findings

### Phase 1: Ecommerce

| Metric | Baseline (3 VUs) | Stress (50 VUs) | Threshold |
|--------|------------------|-----------------|-----------|
| Product browse p95 | [DATA]ms | [DATA]ms | < 500ms |
| Cart ops p95 | [DATA]ms | [DATA]ms | < 500ms |
| Checkout p95 | [DATA]ms | [DATA]ms | < 1000ms |
| Error rate | [DATA]% | [DATA]% | < 1% |

**Stock contention:** [DATA] successful orders out of 50 attempts on item with stock=[DATA]. [Describe whether overselling was observed.]

### Phase 2: Auth

| Metric | Baseline (3 VUs) | Stress | Threshold |
|--------|------------------|--------|-----------|
| Registration p95 | [DATA]ms | [DATA]ms (50 VUs) | < 3000ms |
| Login p95 | [DATA]ms | [DATA]ms (20 req/s) | < 2000ms |
| Token refresh p95 | [DATA]ms | [DATA]ms (30 VUs) | < 500ms |

**bcrypt observation:** [DATA — describe CPU saturation behavior]

### Phase 3: AI Agent

| Metric | Baseline (2 VUs) | Stress | Threshold |
|--------|------------------|--------|-----------|
| Simple query p95 | [DATA]s | [DATA]s (10 VUs) | < 15s |
| Multi-step p95 | [DATA]s | [DATA]s (5 VUs) | < 30s |
| Rate limited requests | 0 | [DATA] total | N/A |

**Ollama observation:** [DATA — describe throughput ceiling and queueing behavior]

## Bottlenecks Identified

1. [Describe each confirmed bottleneck with supporting data]

## Fixes Applied

1. **pgxpool configuration** — explicit pool sizing (25 max, 5 min conns). Before: [DATA]. After: [DATA].
2. **HTTP server timeouts** — added read (10s), write (30s), idle (60s) timeouts.
3. **Worker concurrency** — configurable via env var, default still 3.
4. **Stock race condition** — SELECT FOR UPDATE prevents overselling. Before: [DATA] oversells. After: 0.

## Scaling Recommendations

- HPA manifests added for ecommerce and auth services (CPU target: 70%, max 3 replicas)
- Current single-replica handles [DATA] req/s for product browsing, [DATA] orders/min for checkout
- Auth service is bcrypt-CPU-bound; horizontal scaling is the only option beyond reducing bcrypt cost

## Decision

The Go ecommerce stack handles its expected load profile adequately after the targeted fixes. The main constraints are:
- Ollama throughput for AI agent turns (GPU-bound, not horizontally scalable without additional GPUs)
- bcrypt cost for auth operations (intentional security trade-off)
- Single PostgreSQL instance (shared with Java services)

Load test scripts and Grafana dashboard are committed for future regression testing.
```

- [ ] **Step 2: Fill in [DATA] placeholders with real test results**

Read the k6 output from Tasks 8 and 9, and replace every `[DATA]` marker with the actual measured values.

- [ ] **Step 3: Commit**

```bash
git add docs/adr/go-stress-testing.md
git commit -m "docs(adr): add stress testing findings and scalability analysis"
```

---

### Task 13: Final Commit and Cleanup

- [ ] **Step 1: Verify all files are committed**

Run:
```bash
git status
```
Expected: clean working tree

- [ ] **Step 2: Run full Go preflight**

Run:
```bash
make preflight-go
```
Expected: all checks pass

- [ ] **Step 3: Review the commit log**

Run:
```bash
git log --oneline -10
```

Verify commits are clean and in logical order.
