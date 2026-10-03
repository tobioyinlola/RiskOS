import { expect, test, type BrowserContext, type Page } from "@playwright/test";

import { navSections } from "../../lib/navigation";

/**
 * Against a running stack (`make up`), with no mocks: accepts an invite, registers a passkey
 * with a virtual authenticator (verified by the real API), opens every section, then signs out
 * and back in with that passkey. Run with `make test-e2e-stack`, which creates the tenant and
 * the invite; each invite link works once.
 */

const invite = process.env.E2E_INVITE_URL ?? "";
const email = process.env.E2E_EMAIL ?? "";
const PASSWORD = "stack test password 0123";

test.skip(
  !invite || !email,
  "set E2E_INVITE_URL and E2E_EMAIL (make test-e2e-stack)",
);

test("invite, passkey, every section, and back in", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "one browser is enough against the stack");
  // A whole journey against the real stack: every section, sign-out and back in.
  test.setTimeout(90_000);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });

  // The invite link carries the token in its fragment; open it on this console's origin.
  const link = new URL(invite);
  await page.goto(`/accept-invite${link.hash}`);
  await page.getByLabel("Your name").fill("Stack Tester");
  await page.getByLabel("Password", { exact: true }).fill(PASSWORD);
  await page.getByLabel("Confirm password").fill(PASSWORD);
  await page.getByRole("button", { name: "Create account" }).click();

  await page.getByRole("button", { name: "Create a passkey" }).click();
  await page.getByLabel("I have saved these codes somewhere safe").check();
  await page.getByRole("button", { name: "Continue to RiskOS" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: /^Welcome back/ }),
  ).toBeVisible();

  for (const section of navSections) {
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: section.label, exact: true })
      .click();
    await expect(
      page.getByRole("heading", {
        level: 1,
        // The overview greets the user; every other section is titled by its label.
        name: section.href === "/" ? /^Welcome back/ : section.label,
      }),
    ).toBeVisible();
    await page.waitForLoadState("networkidle");
    await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
  }

  // The same pages in production.
  await page.getByRole("radio", { name: "Production" }).click();
  await page.waitForLoadState("networkidle");
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
  await page.getByRole("radio", { name: "Test" }).click();

  await page.getByRole("button", { name: "Stack Tester" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByRole("button", { name: "Use your passkey" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: /^Welcome back/ }),
  ).toBeVisible();
});

const staffA = process.env.E2E_STAFF_A_URL ?? "";
const staffB = process.env.E2E_STAFF_B_URL ?? "";
const tenantID = process.env.E2E_TENANT ?? "";

/** A page with a virtual passkey authenticator, verified by the real API. */
async function withAuthenticator(context: BrowserContext): Promise<Page> {
  const page = await context.newPage();
  const cdp = await context.newCDPSession(page);
  await cdp.send("WebAuthn.enable");
  await cdp.send("WebAuthn.addVirtualAuthenticator", {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });
  return page;
}

async function enrollStaff(page: Page, link: string) {
  await page.goto(`/admin/enroll${new URL(link).hash}`);
  await page.getByLabel("Passkey name").fill("Virtual key");
  await page.getByRole("button", { name: "Register passkey" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Security across tenants" }),
  ).toBeVisible();
}

test("staff: passkeys, four-eyes support access, read-only view", async ({
  browser,
  isMobile,
}) => {
  test.skip(
    isMobile || !staffA || !staffB || !tenantID,
    "set the staff links (make test-e2e-stack)",
  );
  test.setTimeout(90_000);
  const a = await withAuthenticator(await browser.newContext());
  const b = await withAuthenticator(await browser.newContext());
  await enrollStaff(a, staffA);
  await enrollStaff(b, staffB);
  await expect(
    a.getByRole("table", { name: "Security across tenants" }),
  ).toContainText(tenantID);

  // A requests access; A cannot approve it, B can.
  await a.getByRole("link", { name: "Support access" }).click();
  // Wait for the page itself: until it renders, the overview's "Security across tenants" table
  // is still there, and a loose "Tenant" label match finds it instead of the form field.
  await expect(
    a.getByRole("heading", { level: 1, name: "Support access" }),
  ).toBeVisible();
  await a.getByLabel("Tenant", { exact: true }).selectOption(tenantID);
  await a.getByLabel("Reason").fill("Stack test: checking the read-only view");
  await a.getByRole("button", { name: "Request access" }).click();
  const mine = a.getByRole("table", { name: "Support access requests" });
  await expect(
    mine.getByText("Awaiting another staff member").first(),
  ).toBeVisible();

  await b.getByRole("link", { name: "Support access" }).click();
  const theirs = b.getByRole("table", { name: "Support access requests" });
  await theirs.getByRole("button", { name: "Approve" }).first().click();
  await expect(
    theirs.getByRole("button", { name: "End access" }).first(),
  ).toBeVisible();

  // A reads the tenant, read-only, through the real customer API.
  await a.reload();
  await mine.getByRole("link", { name: "Open read-only view" }).first().click();
  await expect(a.getByRole("heading", { level: 1 })).toContainText(
    "(read-only)",
  );
  await expect(
    a.getByRole("table", { name: "Tenant audit log" }),
  ).toContainText("support_access.approved");
  await expect(a.getByRole("main").getByRole("alert")).toHaveCount(0);

  // Signing out and back in uses the passkey only (discoverable, no email).
  await a.getByRole("button", { name: "Sign out" }).click();
  await expect(a).toHaveURL(/\/admin\/login$/);
  await a.getByRole("button", { name: "Sign in with passkey" }).click();
  await expect(
    a.getByRole("heading", { level: 1, name: "Security across tenants" }),
  ).toBeVisible();
});
