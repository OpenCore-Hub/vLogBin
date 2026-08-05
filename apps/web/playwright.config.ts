import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright configuration for end-to-end Web console tests.
 *
 * The tests run against the long-lived dev stack started by
 * docker-compose.dev.yml:
 *   - Web console:  http://localhost:3001
 *   - Platform API: http://localhost:8082
 *
 * Prerequisites:
 *   docker compose -f docker-compose.dev.yml up -d --build
 *
 * Run:
 *   cd apps/web
 *   pnpm exec playwright test
 *   pnpm exec playwright test --ui      # interactive mode
 *   pnpm exec playwright test --headed  # show browser
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false, // tests share the same DB; sequential is safer
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  use: {
    baseURL: process.env.WEB_URL || "http://localhost:3001",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
