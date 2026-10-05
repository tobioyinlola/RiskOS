import { expect, test } from "@playwright/test";

import { mockApi } from "./mock-api";

test("activating Rwanda: a draft, its readiness, monitoring, and live with a second person", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets&country=RW");

  await page.getByRole("button", { name: "Activate Rwanda" }).click();
  const wizard = page.getByRole("dialog", { name: "Activate Rwanda" });
  await wizard.getByLabel("Booking entity").fill("rw-bank");
  await wizard.getByLabel("Licence type").selectOption("bank");
  await wizard.getByLabel("Regulated by the National Bank of Rwanda").check();
  await wizard.getByLabel("Business day starts at").selectOption("6");
  await wizard
    .getByLabel("Rationale")
    .fill("Rwanda banking subsidiary licensed by BNR");
  await wizard.getByRole("button", { name: "Create draft" }).click();
  await expect(
    page.getByText("Rwanda activated for rw-bank as a draft."),
  ).toBeVisible();

  const card = page.getByRole("region", { name: "Rwanda activations" });
  await expect(card.getByText("Draft", { exact: true })).toBeVisible();
  await expect(card.getByText("06:00 local time")).toBeVisible();
  await expect(card.getByText("1 thing blocks going live:")).toBeVisible();
  await expect(
    card.getByText(/RW-REG-01: set a RWF to US dollar rate/),
  ).toBeVisible();
  await expect(card.getByRole("button", { name: "Go live" })).toBeDisabled();

  await card.getByRole("button", { name: "Start monitoring" }).click();
  await card.getByLabel("Rationale").fill("Watch results for two weeks");
  await card.getByRole("button", { name: "Start monitoring" }).click();
  await expect(card.getByText("Monitoring", { exact: true })).toBeVisible();

  // With the exchange rate set nothing blocks going live, which needs a second person.
  api.fxRates.RWF = 0.00072;
  await page.reload();
  await card.getByRole("button", { name: "Go live" }).click();
  await expect(card.getByText("This needs a second person")).toBeVisible();
  await card.getByLabel("Rationale").fill("Shadow results reviewed");
  await card.getByRole("button", { name: "Go live" }).click();
  await expect(
    page.getByText("Proposed. Another person with approval rights"),
  ).toBeVisible();
  await expect(card.getByText(/Awaiting approval:/)).toBeVisible();
  await expect(card.getByRole("button", { name: "Approve" })).toHaveCount(0);

  await card.getByRole("button", { name: /Change history/ }).click();
  await expect(
    card.getByText("Created as a draft", { exact: false }),
  ).toBeVisible();
  await expect(
    card.getByText("Draft → Monitoring", { exact: false }),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});

test("Your rules lists the country rules running and what they still need", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=custom");
  const table = page.getByRole("table", { name: "Preset rules running" });
  const row = table.getByRole("row", { name: /RW-REG-05/ });
  await expect(row.getByText("Monitoring")).toBeVisible();
  await expect(row.getByText("Set ctr_due")).toBeVisible();
  await expect(row.getByText("rw-bank")).toBeVisible();
  await row.getByRole("link").click();
  await expect(page).toHaveURL(
    /\/compliance\/rules\/RW-REG-05\?jurisdiction=RW/,
  );
  expect(api.problems).toEqual([]);
});

test("an open question is marked verified against the original text", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=references&country=RW");
  await page.getByRole("tab", { name: "Open questions" }).click();
  await page.getByRole("button", { name: "Mark verified" }).first().click();
  await page
    .getByLabel("What you checked")
    .fill("Read Law 001/2025 article 12: no hour count");
  await page
    .getByLabel("Link to the original (optional)")
    .fill("https://www.fic.gov.rw/");
  await page.getByRole("button", { name: "Record verification" }).click();
  await expect(
    page.getByText("Verified against the original text").first(),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});
