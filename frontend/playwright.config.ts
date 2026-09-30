import { defineConfig, devices } from "@playwright/test";

// With E2E_STACK_URL, only the stack test runs, against that running console and its real API
// (`make test-e2e-stack`). Otherwise the console tests run against the production build with a
// mocked API.
const stackURL = process.env.E2E_STACK_URL;
const port = Number(process.env.E2E_PORT ?? 3100);
// localhost, not an IP address: passkeys need a domain as the relying party ID.
const baseURL = stackURL ?? `http://localhost:${port}`;

/**
 * End-to-end tests run against the production build (standalone server), not the dev server,
 * so they exercise what ships. Build first: `pnpm build && pnpm test:e2e`.
 */
export default defineConfig({
  testDir: "tests/e2e",
  ...(stackURL
    ? { testMatch: "stack.spec.ts" }
    : { testIgnore: "stack.spec.ts" }),
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "mobile", use: { ...devices["Pixel 7"] } },
  ],
  webServer: stackURL
    ? undefined
    : {
        command: "pnpm start",
        url: baseURL,
        env: { PORT: String(port), HOSTNAME: "localhost" },
        reuseExistingServer: !process.env.CI,
        timeout: 60_000,
      },
});
