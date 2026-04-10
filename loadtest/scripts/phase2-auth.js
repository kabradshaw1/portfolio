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
