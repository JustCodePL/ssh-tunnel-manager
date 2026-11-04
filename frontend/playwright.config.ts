import { defineConfig } from "@playwright/test";

const port = Number(process.env.STM_E2E_PORT ?? 34115);

export default defineConfig({
  testDir: "./e2e",
  timeout: 45_000,
  expect: { timeout: 10_000 },
  // Every test shares one backend, whose state is what the tests exercise.
  workers: 1,
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts",
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    viewport: { width: 1100, height: 760 },
    trace: "retain-on-failure",
    launchOptions: process.env.STM_E2E_CHROMIUM ? { executablePath: process.env.STM_E2E_CHROMIUM } : {},
  },
});
