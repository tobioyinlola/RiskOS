import { expect, test } from "@playwright/test";

import { navSections } from "../../lib/navigation";

test("console shell loads and every section is reachable", async ({
  page,
  isMobile,
}) => {
  await page.goto("/");
  await expect(page).toHaveTitle("Overview · RiskOS");
  await expect(
    page.getByRole("heading", { level: 1, name: "Overview" }),
  ).toBeVisible();

  for (const section of navSections.slice(1)) {
    if (isMobile) {
      await page.getByRole("button", { name: "Open navigation" }).click();
    }
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: section.label, exact: true })
      .click();
    await expect(page).toHaveURL(section.href);
    await expect(
      page.getByRole("heading", { level: 1, name: section.label }),
    ).toBeVisible();
  }
});

test("security headers are set", async ({ request }) => {
  const res = await request.get("/");
  expect(res.status()).toBe(200);
  expect(res.headers()["x-content-type-options"]).toBe("nosniff");
  expect(res.headers()["x-frame-options"]).toBe("DENY");
  expect(res.headers()["x-powered-by"]).toBeUndefined();
});
