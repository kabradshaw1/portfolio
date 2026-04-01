import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "smoke.spec.ts",
  fullyParallel: false,
  retries: 1,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
  },
});
