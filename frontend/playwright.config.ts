import { defineConfig } from "@playwright/test";

const isCI = !!process.env.CI;

export default defineConfig({
  // Default config runs only the mocked-backend tests. Runtime-specific
  // suites live in sibling directories and are invoked by dedicated configs:
  //   e2e/smoke-prod/    → playwright.smoke.config.ts    (smoke-production CI)
  //   e2e/smoke-compose/ → playwright.smoke-ci.config.ts (compose-smoke CI)
  testDir: "./e2e/mocked",
  fullyParallel: true,
  forbidOnly: isCI,
  retries: isCI ? 2 : 0,
  workers: isCI ? 1 : undefined,
  reporter: "html",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  webServer: {
    // CI: build then serve in production mode (pre-compiled, fast page loads).
    // Local dev: use the dev server (HMR, on-demand compilation).
    command: isCI ? "npm run build && npm start" : "npm run dev",
    url: "http://localhost:3000",
    reuseExistingServer: !isCI,
    timeout: isCI ? 120_000 : 60_000,
  },
});
