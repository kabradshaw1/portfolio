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
