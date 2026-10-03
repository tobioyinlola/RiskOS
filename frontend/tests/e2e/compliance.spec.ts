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

test("configure a rule: settings in natural units, a backtest, a version", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance/rules/VEL-01");
  await expect(page.getByText("Shadow in test")).toBeVisible();
  await page.getByRole("button", { name: "Configure" }).click();
  const dialog = page.getByRole("dialog", { name: "Configure VEL-01" });
  await dialog.getByLabel("Mode", { exact: true }).selectOption("live");
  // The window reads in minutes, not seconds.
  await expect(dialog.getByLabel("Time window", { exact: true })).toHaveValue(
    "10",
  );
  await dialog.getByLabel("Transfers out that count as a burst").fill("4");
  await dialog.getByRole("button", { name: "Backtest" }).click();
  await expect(dialog.getByRole("table", { name: "Backtest" })).toContainText(
    "5",
  );
  await dialog.getByLabel("Transfers out that count as a burst").fill("1");
  await expect(dialog.getByText(/must be between 2 and 100/)).toBeVisible();
  await dialog.getByLabel("Transfers out that count as a burst").fill("4");
  await dialog.getByLabel("Reason").fill("Bursts of four are enough");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText("Saved as version 1.")).toBeVisible();
  await expect(page.getByText("Live in test")).toBeVisible();
  const settings = page.getByRole("table", { name: "Settings" });
  await expect(
    settings.getByRole("cell", { name: "4", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Configuration history" }),
  ).toContainText("Bursts of four are enough");
  expect(api.problems).toEqual([]);
  expect(api.calls).toContain("PUT /compliance/rules/VEL-01/config");
});

test("production: a control's change waits for a second person", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.ruleChanges.push({
    id: "pch_seed",
    env: "production",
    rule: "REG-13",
    base_version: 0,
    mode: "shadow",
    settings: {},
    reason: "Screen the list elsewhere",
    restored_from: null,
    status: "pending",
    proposed_by: "usr_other",
    proposed_at: "2026-09-30T12:00:00Z",
    decided_by: null,
    decided_at: null,
    decision_note: null,
    applied_version: null,
  });
  await page.goto("/compliance");
  await page.getByRole("radio", { name: "Production" }).click();
  await expect(page.getByText("1 change awaits approval")).toBeVisible();
  await page.getByRole("link", { name: "REG-13", exact: true }).first().click();
  await expect(page.getByText(/Awaiting approval/)).toBeVisible();
  await page.getByRole("button", { name: "Approve" }).click();
  await expect(page.getByText("Shadow in production")).toBeVisible();
  await expect(page.getByText(/Awaiting approval/)).toBeHidden();

  // My own change to REG-01 is only proposed.
  await page.goto("/compliance/rules/REG-01");
  await page.getByRole("button", { name: "Configure" }).click();
  const dialog = page.getByRole("dialog", { name: "Configure REG-01" });
  await dialog
    .getByLabel(
      "Most a customer can send within 24 hours of a new device or a new account",
    )
    .fill("10000");
  await dialog.getByLabel("Reason").fill("Stricter cap");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(page.getByText(/^Proposed\./)).toBeVisible();
  await expect(
    page.getByText(/Another person must approve your change/),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
  expect(api.problems).toEqual([]);
});

test("exchange rates", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance");
  await page.getByRole("tab", { name: "Exchange rates" }).click();
  await expect(page.getByText("No exchange rates")).toBeVisible();
  await page.getByLabel("US dollars per unit").fill("0.00065");
  await page.getByRole("button", { name: "Save rate" }).click();
  await expect(
    page.getByRole("table", { name: "Exchange rates" }),
  ).toContainText("0.00065");
  expect(api.problems).toEqual([]);
});
