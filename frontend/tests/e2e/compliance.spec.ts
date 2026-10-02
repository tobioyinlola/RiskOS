import { expect, test } from "@playwright/test";

import { mockApi } from "./mock-api";

test("the rules library: wording, conflicts, settings and what applies", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance");
  await expect(
    page.getByRole("heading", { level: 1, name: "Rules library" }),
  ).toBeVisible();
  await expect(page.getByText("Nigeria · version 2026.10.0")).toBeVisible();
  await expect(page.getByText("3 of 3 rules")).toBeVisible();

  // Conflicts stay visible.
  await page.getByLabel("Verification").selectOption("conflicting");
  await expect(page.getByText("1 of 3 rules")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "REG-13", exact: true }),
  ).toBeVisible();
  await page.getByLabel("Verification").selectOption("");

  // The licence type decides what applies; setting it is a person's decision.
  await page.getByRole("button", { name: "Set licence type" }).click();
  const dialog = page.getByRole("dialog", { name: "Your institution" });
  await dialog.getByLabel("Licence type").selectOption("mfb");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toBeHidden();
  await expect(
    page.getByText("Microfinance bank · NG pack 2026.10.0"),
  ).toBeVisible();
  await page.getByLabel("Only rules that apply to us").check();
  await expect(page.getByText("2 of 3 rules")).toBeVisible();
  await expect(
    page.getByRole("link", { name: "REG-01", exact: true }),
  ).toBeHidden();
  await page.getByLabel("Only rules that apply to us").uncheck();

  // A rule's page: the library's words, the caveat, bounded settings and the logic.
  await page.getByRole("link", { name: "REG-01", exact: true }).click();
  await expect(page).toHaveURL(/\/compliance\/rules\/REG-01$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "New-device limit" }),
  ).toBeVisible();
  await expect(
    page.getByText(
      "A customer sends more than ₦20,000 within 24 hours of linking a new phone or app, or within 24 hours of opening the account",
    ),
  ).toBeVisible();
  await expect(page.getByText("Declines the transfer")).toBeVisible();
  await expect(
    page.getByText(/summaries of the CBN circular, not the circular itself/),
  ).toBeVisible();
  const settings = page.getByRole("table", { name: "Settings" });
  await expect(settings.getByText("20,000.00 NGN").first()).toBeVisible();
  await expect(settings.getByText("1 day to 7 days")).toBeVisible();
  await expect(page.getByText("Does not apply")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Engineering logic" }),
  ).toBeVisible();
  await expect(page.getByText(/RULE REG-01 \| control=hard/)).toBeVisible();

  expect(api.problems).toEqual([]);
  expect(api.calls).toContain("PUT /compliance/profile");
});

test("deadlines, open questions and the glossary", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance");
  await page.getByRole("tab", { name: "Deadlines" }).click();
  await expect(
    page.getByText("Report cyber incidents to the CBN"),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Open questions" }).click();
  await expect(page.getByText("CBN circulars not read directly")).toBeVisible();
  await page.getByRole("tab", { name: "Glossary" }).click();
  await expect(page.getByText("Bank Verification Number")).toBeVisible();
  await page.getByRole("tab", { name: "Sources" }).click();
  await expect(page.getByRole("link", { name: "Afriwise" })).toHaveAttribute(
    "rel",
    "noopener noreferrer",
  );
  expect(api.problems).toEqual([]);
});

test("an auditor reads the plain wording and changes nothing", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.permissions = [
    "events:read",
    "rules:read",
    "alerts:read",
    "cases:read",
    "audit_logs:read",
    "team:read",
    "compliance:read",
  ];
  await page.goto("/compliance");
  await expect(
    page.getByText(/An administrator or compliance officer sets/),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Set licence type" }),
  ).toHaveCount(0);

  await page.goto("/compliance/rules/REG-01");
  await expect(page.getByText("Declines the transfer")).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Engineering logic" }),
  ).toHaveCount(0);
  await expect(page.getByText(/RULE REG-01/)).toHaveCount(0);
  expect(api.problems).toEqual([]);
});

test("an unknown rule says so", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance/rules/ZZZ-99");
  await expect(page.getByText("No such rule")).toBeVisible();
});
