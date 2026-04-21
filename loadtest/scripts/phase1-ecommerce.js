import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import {
  ORDER_URL,
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
    const listRes = http.get(`${ORDER_URL}/products`);
    check(listRes, {
      "product list 200": (r) => r.status === 200,
      "has products": (r) => JSON.parse(r.body).products.length > 0,
    });

    // Get categories
    const catRes = http.get(`${ORDER_URL}/categories`);
    check(catRes, {
      "categories 200": (r) => r.status === 200,
    });

    // Browse by category
    const categories = JSON.parse(catRes.body).categories || [];
    if (categories.length > 0) {
      const cat = randomItem(categories);
      http.get(`${ORDER_URL}/products?category=${cat}`);
    }

    // View a single product
    const products = JSON.parse(listRes.body).products || [];
    if (products.length > 0) {
      const product = randomItem(products);
      const detailRes = http.get(`${ORDER_URL}/products/${product.id}`);
      check(detailRes, {
        "product detail 200": (r) => r.status === 200,
      });
    }

    // Paginate
    http.get(`${ORDER_URL}/products?page=2&limit=10`);
    http.get(`${ORDER_URL}/products?sort=price_asc&limit=10`);
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
        `${ORDER_URL}/cart`,
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
      `${ORDER_URL}/cart`,
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
        `${ORDER_URL}/cart/${cartItems[0].id}`,
        JSON.stringify({ quantity: 2 }),
        authHeaders(auth.accessToken)
      );
      cartOpDuration.add(Date.now() - start);
    }

    // Remove last item
    if (cartItems.length > 1) {
      http.del(
        `${ORDER_URL}/cart/${cartItems[cartItems.length - 1].id}`,
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
        `${ORDER_URL}/cart`,
        JSON.stringify({ productId: item.id, quantity: 1 }),
        authHeaders(auth.accessToken)
      );
    }

    // Checkout
    const orderRes = http.post(
      `${ORDER_URL}/orders`,
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
        `${ORDER_URL}/orders`,
        authHeaders(auth.accessToken)
      );
      check(listRes, {
        "order in list": (r) => r.body.includes(order.id),
      });

      // Fetch order detail
      http.get(
        `${ORDER_URL}/orders/${order.id}`,
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
      `${ORDER_URL}/cart`,
      JSON.stringify({ productId: target.id, quantity: 1 }),
      authHeaders(auth.accessToken)
    );

    if (addRes.status === 200 || addRes.status === 201) {
      // Attempt checkout
      const orderRes = http.post(
        `${ORDER_URL}/orders`,
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
