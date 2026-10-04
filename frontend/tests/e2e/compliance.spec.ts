import { expect, test } from "@playwright/test";

import { PACK, task } from "./compliance-fixtures";
import { mockApi } from "./mock-api";

test("preset rules: confirm the institution, activate, turn off; no code anywhere", async ({
  page,
  isMobile,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/compliance");
  await expect(page).toHaveURL(/\/rules\?tab=presets$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Rules" }),
  ).toBeVisible();
  await expect(page.getByText("Nigeria · version 2026.10.0")).toBeVisible();
  await expect(page.getByText("3 of 3 rules")).toBeVisible();

  // Every workspace starts with a profile; a person confirms what kind of institution it is.
  await page.getByRole("button", { name: "Confirm institution type" }).click();
  const dialog = page.getByRole("dialog", { name: "Your institution" });
  await dialog.getByLabel("Licence type").selectOption("mfb");
  await dialog.getByRole("button", { name: "Save" }).click();
  await expect(dialog).toBeHidden();
  await expect(
    page.getByText("Microfinance bank · NG pack 2026.10.0"),
  ).toBeVisible();

  // Legal controls start on; risk signals start off until someone activates them.
  const table = page.getByRole("table", { name: "Preset rules" });
  const row = (name: RegExp) => table.getByRole("row", { name });
  await expect(row(/Person of interest/)).toContainText("Required by law");
  await expect(row(/Person of interest/)).toContainText("Active");
  await expect(row(/Burst of transfers/)).toContainText("Off");
  await expect(row(/New-device limit/)).toContainText(
    "Not for your institution type",
  );
  await page.getByLabel("Only rules for our institution type").check();
  await expect(page.getByText("2 of 3 rules")).toBeVisible();

  await page
    .getByRole("button", { name: "Activate Burst of transfers" })
    .click();
  await expect(row(/Burst of transfers/)).toContainText("Active");
  expect(api.calls).toContain("PUT /compliance/rules/VEL-01/config");

  // Turning off a control the law requires asks why.
  await page
    .getByRole("button", { name: "Turn off Person of interest" })
    .click();
  const off = page.getByRole("dialog", {
    name: "Turn off Person of interest?",
  });
  await expect(off).toContainText("required by");
  await off.getByLabel("Why").fill("Replaced by our own screening provider");
  await off.getByRole("button", { name: "Turn off" }).click();
  await expect(off).toBeHidden();
  await expect(row(/Person of interest/)).toContainText("Off");

  // A rule's page: the library's words and its settings, never engineering logic or code.
  await page.getByLabel("Only rules for our institution type").uncheck();
  await page.getByRole("link", { name: "New-device limit" }).click();
  await expect(page).toHaveURL(/\/compliance\/rules\/REG-01\?jurisdiction=NG$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "New-device limit" }),
  ).toBeVisible();
  await expect(page.getByText("Declines the transfer")).toBeVisible();
  const settings = page.getByRole("table", { name: "Settings" });
  await expect(settings.getByText("20,000.00 NGN").first()).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Engineering logic" }),
  ).toHaveCount(0);
  await expect(page.getByText(/RULE REG-01/)).toHaveCount(0);
  if (!isMobile) {
    // A preset rule's page belongs to Rules in the navigation.
    await expect(
      page.getByRole("navigation").getByRole("link", { name: "Rules" }),
    ).toHaveAttribute("aria-current", "page");
  }

  expect(api.problems).toEqual([]);
  expect(api.calls).toContain("PUT /compliance/profile");
});

test("deadlines, open questions and the glossary", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=references");
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
  await page.goto("/rules?tab=presets");
  await expect(
    page.getByText(/A compliance officer or administrator should confirm/),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Confirm institution type" }),
  ).toHaveCount(0);
  await expect(page.getByRole("button", { name: /^Activate / })).toHaveCount(0);

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
  // A profile set up before automatic profiles: risk signals at the pack's defaults (shadow).
  api.complianceProfile = { ...api.complianceProfile, signalsOff: false };
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
  await page.goto("/rules?tab=presets");
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
  await page.goto("/rules?tab=references");
  await page.getByRole("tab", { name: "Exchange rates" }).click();
  await expect(page.getByText("No exchange rates")).toBeVisible();
  await page.getByLabel("US dollars per unit").fill("0.00065");
  await page.getByRole("button", { name: "Save rate" }).click();
  await expect(
    page.getByRole("table", { name: "Exchange rates" }),
  ).toContainText("0.00065");
  expect(api.problems).toEqual([]);
});

test("the work queue: approve a report and record its filing", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.tasks.push(
    task("ctk_ftr", "report.FTR", "REG-10", {
      report: { amount: 1_600_000_000, currency: "NGN" },
    }),
    task("ctk_str", "report.STR", "REG-14"),
  );
  await page.goto("/compliance/work");
  await expect(
    page.getByRole("heading", { level: 1, name: "Work queue" }),
  ).toBeVisible();
  const table = page.getByRole("table", { name: "Compliance tasks" });
  await expect(table).toContainText("Foreign transaction report");
  await expect(table).toContainText("Suspicious transaction report");
  await expect(table).toContainText(/due in 4h 5\dm/);

  await table
    .getByRole("row", { name: /Foreign transaction report/ })
    .getByRole("button", { name: "Open" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("16,000,000.00 NGN");
  await dialog.getByRole("button", { name: "Approve for filing" }).click();
  await expect(dialog.getByRole("button", { name: "Export" })).toBeVisible();
  await dialog.getByLabel("Filing reference").fill("NFIU-FTR-0001");
  await dialog.getByRole("button", { name: "Record filing" }).click();
  await expect(dialog).toContainText("filed as NFIU-FTR-0001");
  expect(api.problems).toEqual([]);
});

test("without compliance:sensitive, suspicious transaction reports do not exist", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.permissions = api.permissions.filter((p) => p !== "compliance:sensitive");
  api.tasks.push(
    task("ctk_ftr", "report.FTR", "REG-10"),
    task("ctk_str", "report.STR", "REG-14"),
  );
  await page.goto("/compliance/work");
  const table = page.getByRole("table", { name: "Compliance tasks" });
  await expect(table).toContainText("Foreign transaction report");
  await expect(table).not.toContainText("Suspicious transaction report");
  await expect(
    page.getByRole("table", { name: "Compliance-only signals" }),
  ).toHaveCount(0);
  await table.getByRole("button", { name: "Open" }).click();
  await expect(
    page.getByRole("button", { name: "Approve for filing" }),
  ).toHaveCount(0);
  await expect(
    page.getByText("A compliance officer approves reports"),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});

test("a newer pack version can be adopted", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.complianceProfile = {
    jurisdiction: "NG",
    licence_type: "bank",
    capabilities: [],
    confirmed: true,
    signalsOff: false,
  };
  await page.route("**/api/v1/compliance/pack", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ ...PACK, latest_version: "2026.10.1" }),
    }),
  );
  await page.goto("/rules?tab=presets");
  await expect(
    page.getByText(/Version 2026.10.1 of the Nigeria pack is available/),
  ).toBeVisible();
  await page.getByRole("button", { name: "Adopt 2026.10.1" }).click();
  await expect
    .poll(() => api.calls.filter((c) => c === "PUT /compliance/profile").length)
    .toBe(1);
});

test("governance: urgent alerts escalate and confirmed fraud is reported", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  const ago = (minutes: number) =>
    new Date(Date.now() - minutes * 60_000).toISOString();
  api.escalations.push({
    alert_id: "alt_urgent",
    rule: "GOV-11",
    title: "Card testing burst",
    subject_ref: "usr_4812",
    severity: "critical",
    created_at: ago(40),
    due_at: ago(10),
  });
  api.unreportedFraud.push({
    case_id: "cas_fraud",
    title: "Account takeover",
    resolved_at: ago(60),
  });
  await page.goto("/compliance/work");
  await expect(
    page.getByRole("alert").filter({ hasText: "GOV-11" }),
  ).toContainText(
    "1 urgent alert nobody has picked up within the fraud response target (GOV-11)",
  );
  await expect(
    page.getByRole("link", { name: "Card testing burst" }),
  ).toHaveAttribute("href", /alert=alt_urgent/);

  const unreported = page.getByRole("table", {
    name: "Confirmed fraud not yet reported",
  });
  await unreported
    .getByRole("button", { name: "Prepare a report for Account takeover" })
    .click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Industry fraud report");
  await expect(dialog).toContainText("Credentials phished.");
  await expect(unreported).toHaveCount(0);
  await dialog.getByRole("button", { name: "Approve for filing" }).click();
  await expect(dialog).toContainText("industry fraud channel (NIBSS)");
  expect(api.problems).toEqual([]);
});
