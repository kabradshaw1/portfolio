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
