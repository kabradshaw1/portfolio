import http from "k6/http";
import { check, group, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";
import { AI_URL, jsonHeaders, registerUser } from "../lib/helpers.js";

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
