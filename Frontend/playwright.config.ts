import { defineConfig, devices } from "@playwright/test";

/**
 * Browser verification of E2. The tests talk to the same-origin Next proxy, so
 * the Go API must already be running on 127.0.0.1:8080 and the demo practice
 * must be seeded (`scripts/local-seed.ps1`).
 *
 * They reuse a running `pnpm dev` when one exists, which is the local workflow.
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: 0,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "pnpm dev",
    url: "http://localhost:3000",
    reuseExistingServer: true,
    timeout: 120_000,
  },
});
