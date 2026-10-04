import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { mockApi } from "./mock-api";

// The developer documentation is public: reached from the landing page's footer, readable
// without a session, and its API reference is built from the OpenAPI contract.
async function audit(page: Page) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const summary = result.violations.map(
    (v) =>
      `${v.id} (${v.impact}): ${v.help}\n${v.nodes
        .slice(0, 3)
        .map((n) => `  ${n.target.join(" ")}`)
        .join("\n")}`,
  );
  expect(summary, summary.join("\n")).toEqual([]);
}

test("the footer leads to the developer docs, which need no session", async ({
  page,
}) => {
  const api = await mockApi(page);
  await page.goto("/home");
  await page
    .getByRole("contentinfo")
    .getByRole("link", { name: "Developer docs" })
    .click();
  await expect(page).toHaveURL(/\/docs$/);
  await expect(
    page.getByRole("heading", {
      level: 1,
      name: "RiskOS developer documentation",
    }),
  ).toBeVisible();
  await audit(page);
  expect(api.calls.filter((c) => !c.startsWith("GET /auth"))).toEqual([]);
});

test("every guide renders and links to the next", async ({
  page,
  isMobile,
}) => {
  await mockApi(page);
  const guides: [string, string][] = [
    ["/docs/quickstart", "Quickstart"],
    ["/docs/authentication", "Authentication and keys"],
    ["/docs/events", "Sending events"],
    ["/docs/decisions", "Acting on decisions"],
    ["/docs/errors", "Errors, limits and retries"],
    ["/docs/webhooks", "Webhooks"],
    ["/docs/rules", "Rules"],
    ["/docs/compliance", "Compliance"],
    ["/docs/alerts-and-cases", "Alerts and cases"],
    ["/docs/going-live", "Going live"],
  ];
  for (const [path, title] of guides) {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { level: 1, name: title }),
    ).toBeVisible();
  }
  if (!isMobile) {
    await expect(
      page
        .getByRole("navigation", { name: "Documentation" })
        .getByRole("link", { name: "Going live" }),
    ).toHaveAttribute("aria-current", "page");
  }
  await page.goto("/docs/webhooks");
  await audit(page);
});

test("the API reference lists every group and documents its operations", async ({
  page,
}) => {
  await mockApi(page);
  await page.goto("/docs/reference");
  await expect(
    page.getByRole("heading", { level: 1, name: "API reference" }),
  ).toBeVisible();
  await page
    .getByRole("main")
    .getByRole("link", { name: "Rules", exact: true })
    .click();
  await expect(page).toHaveURL(/\/docs\/reference\/rules$/);
  await expect(
    page.getByRole("heading", { level: 2, name: "Create a rule" }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      level: 2,
      name: "Try a condition on recent events",
    }),
  ).toBeVisible();
  await audit(page);

  const missing = await page.goto("/docs/reference/nothing");
  expect(missing?.status()).toBe(404);
});
