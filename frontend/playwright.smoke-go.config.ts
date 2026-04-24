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
