import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

import { task } from "./compliance-fixtures";
import { mockApi } from "./mock-api";

// Automated accessibility checks (WCAG 2.1 A and AA) of the rule pack's pages, with their data
// and dialogs open. Automated rules catch about a third of issues; keyboard and screen-reader
// checks are in docs/compliance/ACCESSIBILITY.md.
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

test("preset rules, the rule builder and a rule", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets");
  await expect(page.getByRole("table", { name: "Preset rules" })).toBeVisible();
  await audit(page);

  await page.goto("/rules/new");
  await expect(
    page.getByRole("heading", { level: 1, name: "Create rule" }),
  ).toBeVisible();
  await page
    .getByLabel("Condition 1: what to check")
    .selectOption("transaction.amount");
  await audit(page);

  await page.goto("/compliance/rules/REG-01");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await audit(page);
});

test("another country: its rules, a rule, its deadlines and all countries", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets&country=RW");
  await expect(page.getByRole("table", { name: "Preset rules" })).toBeVisible();
  await audit(page);

  await page.goto("/compliance/rules/RW-REG-04?jurisdiction=RW");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await audit(page);

  await page.goto("/rules?tab=references&country=RW");
  await expect(page.getByRole("table", { name: "Deadlines" })).toBeVisible();
  await audit(page);

  await page.goto("/rules?tab=presets&country=ALL");
  await expect(page.getByText("Africa/Kigali")).toBeVisible();
  await audit(page);
});

test("the work queue, its panels and a report", async ({ page }) => {
  const api = await mockApi(page);
  api.signedIn = true;
  const ago = (minutes: number) =>
    new Date(Date.now() - minutes * 60_000).toISOString();
  api.tasks.push(
    task("ctk_ftr", "report.FTR", "REG-10", {
      report: { amount: 1_600_000_000, currency: "NGN" },
    }),
    task("ctk_str", "report.STR", "REG-14"),
    task("ctk_close", "close_account", "REG-16", { account_ref: "acc_77" }),
  );
  api.escalations.push(
    {
      alert_id: "alt_urgent",
      rule: "GOV-11",
      title: "Card testing burst",
      subject_ref: "usr_4812",
      severity: "critical",
      created_at: ago(40),
      due_at: ago(10),
    },
    {
      alert_id: "alt_late",
      rule: "REG-14",
      title: "Unusual inflows",
      subject_ref: "usr_1001",
      severity: "medium",
      created_at: ago(5000),
      due_at: ago(680),
    },
  );
  api.unreportedFraud.push({
    case_id: "cas_fraud",
    title: "Account takeover",
    resolved_at: ago(60),
  });
  await page.goto("/compliance/work");
  await expect(
    page.getByRole("table", { name: "Confirmed fraud not yet reported" }),
  ).toBeVisible();
  await audit(page);

  await page
    .getByRole("table", { name: "Compliance tasks" })
    .getByRole("row", { name: /Suspicious transaction report/ })
    .getByRole("button", { name: "Open" })
    .click();
  await expect(page.getByRole("dialog")).toHaveAccessibleDescription(
    /Required by REG-14 \(NG\) for customer usr_4812/,
  );
  await audit(page);
});
