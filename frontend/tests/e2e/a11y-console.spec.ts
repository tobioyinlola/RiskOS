import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { mockApi } from "./mock-api";

// Automated accessibility checks (WCAG 2.1 A and AA) of the console's pages outside the rule
// pack, signed in and with the mock API's data (the rule pack's pages are in a11y.spec.ts).
// Automated rules catch only part of what matters; docs/compliance/ACCESSIBILITY.md says what was
// checked by hand.
async function audit(page: Page) {
  // Pure decoration (the footer's oversized wordmark) is exempt from contrast (WCAG 1.4.3), and is
  // hidden from assistive technology.
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .exclude("[data-decorative]")
    .analyze();
  const summary = result.violations.map(
    (v) =>
      `${v.id} (${v.impact}): ${v.help}\n${v.nodes
        .slice(0, 3)
        .map((n) => `  ${n.target.join(" ")}\n    ${n.html.slice(0, 240)}`)
        .join("\n")}`,
  );
  expect(summary, summary.join("\n")).toEqual([]);
}

const consolePages: { name: string; path: string }[] = [
  { name: "overview", path: "/" },
  { name: "alerts", path: "/alerts" },
  { name: "cases", path: "/cases" },
  { name: "a case", path: "/cases/cas_1" },
  { name: "events", path: "/events" },
  { name: "transactions", path: "/transactions" },
  { name: "live monitor", path: "/live" },
  { name: "an entity profile", path: "/entities/user/usr_4812" },
  { name: "analytics", path: "/analytics" },
  { name: "deny lists", path: "/deny-lists" },
  { name: "audit log", path: "/audit-logs" },
  { name: "account security", path: "/account" },
  { name: "settings", path: "/settings" },
  { name: "team", path: "/team" },
  { name: "developer", path: "/developer" },
  { name: "a webhook endpoint", path: "/developer/webhooks/whk_1" },
  { name: "single sign-on", path: "/sso" },
];

for (const { name, path } of consolePages) {
  test(`console: ${name}`, async ({ page }) => {
    const api = await mockApi(page);
    api.signedIn = true;
    await page.goto(path);
    await expect(page.locator("main")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    // Let data replace the loading state: an audit of a skeleton proves little.
    await page.waitForLoadState("networkidle");
    await audit(page);
  });
}

for (const { name, path } of [
  { name: "sign in", path: "/login" },
  { name: "sign up", path: "/signup" },
  { name: "landing page", path: "/home" },
]) {
  test(`public: ${name}`, async ({ page }) => {
    await mockApi(page);
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await page.waitForLoadState("networkidle");
    await audit(page);
  });
}
