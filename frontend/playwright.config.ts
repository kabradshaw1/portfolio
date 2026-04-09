import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  // smoke.spec.ts hits real production URLs and only runs via the
  // smoke-production CI job (--config=playwright.smoke.config.ts).
  // smoke-ci.spec.ts hits a local docker-compose stack and only runs via
  // the compose-smoke CI job (--config=playwright.smoke-ci.config.ts).
  // Both must be excluded from the default e2e-staging run, which starts
  // its own Next.js dev server and mocks backend APIs.
  testIgnore: ["**/smoke.spec.ts", "**/smoke-ci.spec.ts"],
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: "html",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  webServer: {
    command: "npm run dev",
    url: "http://localhost:3000",
    reuseExistingServer: !process.env.CI,
  },
});
