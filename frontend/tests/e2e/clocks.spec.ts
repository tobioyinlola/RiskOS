import { expect, test } from "@playwright/test";

import { task } from "./compliance-fixtures";
import { mockApi } from "./mock-api";

/** A breach of a Rwandan data subject's data, booked in Ghana: Rwanda's 48-hour clock. */
const ncsa = () =>
  task("ctk_ncsa", "report.NCSA", "RW-SEC-04", {
    jurisdiction: "RW",
    event_jurisdiction: "GH",
    authority: "NCSA",
    event_id: "evt_breach",
  });

test("the deadlines matrix: work running now, cross-border events and retention", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.tasks.push(ncsa());
  await page.goto("/rules?tab=references&country=ALL");

  const load = page.getByRole("table", {
    name: "Work running against each clock",
  });
  await expect(
    load.getByRole("row").filter({ hasText: "RW-SEC-04" }),
  ).toContainText("no legal clock listed");
  const cross = page.getByRole("table", { name: "Cross-border events" });
  await expect(cross.getByText("evt_breach")).toBeVisible();
  await expect(
    cross.getByText("Report to the National Cyber Security Authority"),
  ).toBeVisible();
  const retention = page.getByRole("table", { name: "Retention in force" });
  await expect(
    retention.getByRole("row").filter({ hasText: "Customer files" }),
  ).toContainText("10 years");
  expect(api.problems).toEqual([]);
});

test("the work queue: each task's country and authority, filtered by country", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  api.activations.push({
    ...api.activations[0],
    id: "act_rw",
    jurisdiction: "RW",
    booking_entity: "rw-emi",
    fallback: false,
    pack: "rw@2026.10.3",
  });
  api.tasks.push(ncsa(), task("ctk_ftr", "report.FTR", "REG-10"));
  await page.goto("/compliance/work");

  const tasks = page.getByRole("table", { name: "Compliance tasks" });
  await expect(tasks.getByText(/to the NCSA/)).toBeVisible();
  await expect(
    tasks.getByText(/data subject in RW, booked in GH/),
  ).toBeVisible();
  await page.getByLabel("Country").selectOption("RW");
  await expect(tasks.getByText("Foreign transaction report")).toHaveCount(0);
  await expect(
    tasks.getByText("Report to the National Cyber Security Authority"),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});

test("a country's public holidays: entered, listed and removed", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules?tab=presets&country=NG");

  const panel = page.getByRole("table", { name: "Nigeria public holidays" });
  await expect(page.getByText("No holidays entered")).toBeVisible();
  await page.getByLabel("Date").fill("2026-12-25");
  await page.getByLabel("Holiday", { exact: true }).fill("Christmas Day");
  await page.getByRole("button", { name: "Add holiday" }).click();
  await expect(panel.getByText("Christmas Day")).toBeVisible();
  expect(api.holidays.NG).toEqual([
    { date: "2026-12-25", name: "Christmas Day" },
  ]);
  await page
    .getByRole("button", { name: "Remove Christmas Day on 2026-12-25" })
    .click();
  await expect(panel.getByText("Christmas Day")).toHaveCount(0);
  expect(api.problems).toEqual([]);
});
