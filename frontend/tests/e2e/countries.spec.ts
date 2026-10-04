import { expect, test } from "@playwright/test";

import { mockApi } from "./mock-api";

test("another country's rules: Rwanda's STR deadline is a design placeholder", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets");

  const country = page.getByRole("navigation", { name: "Country" });
  await expect(
    country.getByRole("button", { name: /Nigeria · Default country/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await country
    .getByRole("button", { name: /Rwanda · Not switched on/ })
    .click();
  await expect(page).toHaveURL(/country=RW/);
  await expect(page.getByText("Rwanda is not switched on")).toBeVisible();

  const table = page.getByRole("table", { name: "Preset rules" });
  const str = table.getByRole("row", { name: /Suspicious transaction report/ });
  await expect(str.getByText("Regulatory")).toBeVisible();
  await expect(str.getByText("On by default")).toBeVisible();
  // Nothing here can be switched on or off: the country is read, not configured.
  await expect(table.getByRole("button")).toHaveCount(0);
  const tm = table.getByRole("row", { name: /New device, new payee/ });
  await expect(tm.getByText("Industry practice")).toBeVisible();
  await expect(tm.getByText("Based on ATO-01")).toBeVisible();

  await page
    .getByLabel("Where it comes from")
    .selectOption("industry_practice");
  await expect(table.getByRole("row")).toHaveCount(2); // header and RW-TM-05
  await page.getByLabel("Where it comes from").selectOption("");

  await str
    .getByRole("link", { name: "Suspicious transaction report (STR)" })
    .click();
  await expect(page).toHaveURL(
    /\/compliance\/rules\/RW-REG-04\?jurisdiction=RW/,
  );
  await expect(
    page.getByText(
      "The 24-hour clock is a design placeholder, not a statutory number.",
      { exact: false },
    ),
  ).toBeVisible();
  await expect(
    page.getByText("A country version of Nigeria's REG-14."),
  ).toBeVisible();
  await expect(page.getByText("[Regulatory]. Param of REG-14")).toBeVisible();
  await expect(page.getByRole("button", { name: "Configure" })).toHaveCount(0);
  await expect(page.getByRole("table", { name: "Penalties" })).toBeVisible();
});

test("Rwanda's deadlines: placeholders and clocks not found are labelled", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=references&country=RW");
  const clocks = page.getByRole("table", { name: "Deadlines" });
  const str = clocks.getByRole("row", {
    name: /suspicious transaction report/,
  });
  await expect(
    str.getByText("Design placeholder", { exact: true }),
  ).toBeVisible();
  const bnr = clocks.getByRole("row", { name: /central bank/ });
  await expect(bnr.getByText("Not found").first()).toBeVisible();
  await expect(bnr.getByText("0", { exact: true })).toHaveCount(0);

  await page.getByRole("tab", { name: "Open questions" }).click();
  await expect(page.getByText("Sources conflict")).toBeVisible();
  await expect(page.getByText("Until it is settled")).toBeVisible();

  await page.getByRole("tab", { name: "Countries side by side" }).click();
  await expect(
    page.getByRole("cell", { name: /numeric deadline not found/ }),
  ).toBeVisible();

  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Spreadsheet (CSV)" }).click();
  expect((await download).suggestedFilename()).toBe(
    "riskos-rules-rw-2026-09-30.csv",
  );
});

test("all countries: where the workspace stands in each", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets&country=ALL");
  await expect(
    page.getByRole("button", { name: "All countries" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByText("Africa/Kigali")).toBeVisible();
  await expect(page.getByText("Default country").first()).toBeVisible();
  await page.getByRole("button", { name: "See Rwanda's rules" }).click();
  await expect(page).toHaveURL(/country=RW/);
  await expect(page.getByRole("table", { name: "Preset rules" })).toBeVisible();
});
