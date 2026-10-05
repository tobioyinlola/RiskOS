import { expect, test } from "@playwright/test";

import { mockApi } from "./mock-api";

test("build a rule without code: when, if with and / or, then", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=custom");
  await expect(page.getByText("No rules of your own yet")).toBeVisible();
  await page.getByRole("link", { name: "Create rule" }).click();
  await expect(page).toHaveURL(/\/rules\/new$/);

  await page
    .getByLabel("Rule name")
    .fill("Large transfer to a new beneficiary");
  // When: transactions are chosen to start with.
  await expect(
    page.getByRole("checkbox", { name: "Transaction" }),
  ).toBeChecked();

  // If: the amount is over 1,000,000 and the beneficiary is new, and the channel is card or wallet.
  await page
    .getByLabel("Condition 1: what to check")
    .selectOption("transaction.amount");
  await page.getByLabel("Condition 1: comparison").selectOption("gt");
  await page.getByLabel("Condition 1: value").fill("1000000");
  await page
    .getByRole("button", { name: "Add condition", exact: true })
    .click();
  await page
    .getByLabel("Condition 2: what to check")
    .selectOption("feature.new_beneficiary");
  await page.getByLabel("Condition 2: value").selectOption("yes");
  await page.getByRole("button", { name: "Add a group" }).click();
  await page
    .getByLabel("Condition 3: what to check")
    .selectOption("transaction.channel");
  const channels = page.getByRole("group", { name: "Condition 3: value" });
  await channels.getByRole("checkbox", { name: "Card" }).check();
  await channels.getByRole("checkbox", { name: "Wallet" }).check();
  // A field the chosen events never carry is flagged.
  await page
    .getByRole("button", { name: "Add condition", exact: true })
    .click();
  await page
    .getByLabel("Condition 4: what to check")
    .selectOption("feature.failed_logins_15m");
  await expect(page.getByText(/would never be true/)).toBeVisible();
  await page.getByRole("button", { name: "Remove condition 4" }).click();

  // Countries are picked by name; the rule keeps their codes.
  await page
    .getByRole("button", { name: "Add condition", exact: true })
    .click();
  await page
    .getByLabel("Condition 4: what to check")
    .selectOption("beneficiary.country");
  await page
    .getByRole("combobox", { name: "Condition 4: value" })
    .fill("Ghana");
  await expect(
    page.getByRole("button", { name: "Remove Ghana" }),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Condition 4: value" })
    .fill("united kingdom");
  await page
    .getByRole("combobox", { name: "Condition 4: value" })
    .press("Enter");
  await expect(
    page.getByRole("button", { name: "Remove United Kingdom" }),
  ).toBeVisible();

  // Then: 40 risk points, high.
  await page.getByLabel("Risk points to add").fill("40");
  await page.getByLabel("How serious it is").selectOption("high");
  await expect(
    page.getByText(
      "When a transaction happens, if amount is more than 1,000,000.00 and new beneficiary for this customer is yes and (channel is card or wallet) and beneficiary's country is Ghana or United Kingdom, add 40 risk points (high severity).",
    ),
  ).toBeVisible();

  // Try it on recent events before saving.
  await page.getByLabel("Period to try").selectOption("30");
  await page.getByRole("button", { name: "Try it" }).click();
  await expect(
    page.getByText(/Would have matched 4 of 1,240 events in the last 30 days/),
  ).toBeVisible();
  await expect(
    page.getByRole("link", {
      name: /Transaction · .*2,500,000\.00.* · Review/,
    }),
  ).toHaveAttribute("href", "/events?event=evt_1");
  expect(api.calls).toContain("POST /rules/test-recent");
  await expect(
    page.getByRole("radio", { name: /Monitor first/ }),
  ).toBeChecked();
  // No code on the page: no JSON, no field names, no code blocks.
  await expect(page.locator("pre, code")).toHaveCount(0);
  await expect(page.getByText("transaction.amount")).toHaveCount(0);

  await page.getByRole("button", { name: "Create rule" }).click();
  await expect(page).toHaveURL(/\/rules\/rul_1$/);
  expect(api.rules[0]).toMatchObject({
    code: "large_transfer_to_a_new_beneficiary",
    name: "Large transfer to a new beneficiary",
    weight: 40,
    severity: "high",
    mode: "shadow",
    enabled: true,
    condition: {
      all: [
        { field: "event.type", op: "eq", value: "transaction" },
        { field: "transaction.amount", op: "gt", value: 100_000_000 },
        { field: "feature.new_beneficiary", op: "eq", value: true },
        { field: "transaction.channel", op: "in", value: ["card", "wallet"] },
        { field: "beneficiary.country", op: "in", value: ["GH", "GB"] },
      ],
    },
  });

  // The saved rule opens in the same form, and the list says it in words.
  await expect(page.getByLabel("Condition 1: value")).toHaveValue("1000000");
  await page
    .getByRole("main")
    .getByRole("link", { name: "Rules", exact: true })
    .click();
  const list = page.getByRole("table", { name: "Your rules" });
  await expect(list).toContainText("add 40 risk points (high severity)");
  // How many are on, and what each matched over the last 7 days.
  await expect(
    page.getByText("Monitoring", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    list.getByRole("img", { name: /matches per day: .*4 on/ }),
  ).toBeVisible();
  await expect(list).toContainText("9");

  // The rule's own activity.
  await list
    .getByRole("link", { name: "Large transfer to a new beneficiary" })
    .click();
  await page.getByRole("tab", { name: "Activity" }).click();
  await expect(page.getByText("Events matched")).toBeVisible();
  await expect(
    page.getByRole("img", { name: /^Matches per day:/ }),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});
