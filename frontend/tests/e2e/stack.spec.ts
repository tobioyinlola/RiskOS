import { expect, test } from "@playwright/test";

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
    page.getByRole("heading", { level: 1, name: "Overview" }),
  ).toBeVisible();

  for (const section of navSections) {
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: section.label, exact: true })
      .click();
    await expect(
      page.getByRole("heading", { level: 1, name: section.label }),
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
  await page.getByRole("button", { name: "Continue" }).click();
  await page.getByRole("button", { name: "Use your passkey" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "Overview" }),
  ).toBeVisible();
});
