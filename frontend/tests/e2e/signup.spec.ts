import { expect, test, type Page } from "@playwright/test";

import { mockApi } from "./mock-api";

/** A platform passkey authenticator that approves every prompt (Chromium only). */
async function virtualAuthenticator(page: Page) {
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
}

/** Fails the test on any Content-Security-Policy violation the page reports. */
function watchCsp(page: Page): string[] {
  const violations: string[] = [];
  page.on("console", (msg) => {
    if (/Content Security Policy/i.test(msg.text()))
      violations.push(msg.text());
  });
  return violations;
}

async function secureWithPasskey(page: Page) {
  await expect(
    page.getByRole("heading", { name: "Add a second factor" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Create a passkey" }).click();
  await expect(
    page.getByRole("heading", { name: "Save your recovery codes" }),
  ).toBeVisible();
  await page.getByLabel("I have saved these codes somewhere safe").check();
  await page.getByRole("button", { name: "Continue to RiskOS" }).click();
}

test("sign up with email: code, passkey, compliance onboarding, submitted", async ({
  page,
}) => {
  const csp = watchCsp(page);
  const api = await mockApi(page);
  await virtualAuthenticator(page);

  // The landing page's call to action leads to sign-up.
  await page.goto("/home");
  await page.getByRole("link", { name: "Get started" }).first().click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(
    page.getByRole("button", { name: "Continue with Google" }),
  ).toBeVisible();

  await page.getByLabel("Full name").fill("Ada Lovelace");
  await page.getByLabel("Work email").fill("ada@acme.example");
  await page.getByLabel("Business name").fill("Acme Payments");
  await page.getByLabel("Password").fill("a long enough passphrase");
  await page.getByRole("button", { name: "Create workspace" }).click();

  await expect(page.getByRole("heading", { name: /Check your/ })).toBeVisible();
  await expect(page.getByText("ada@acme.example")).toBeVisible();
  const code = page.getByLabel("Verification code");
  await code.fill("000000");
  await page
    .getByRole("button", { name: "Verify and create workspace" })
    .click();
  await expect(page.getByText("That code is incorrect.")).toBeVisible();
  await code.fill("123456");
  await page
    .getByRole("button", { name: "Verify and create workspace" })
    .click();

  await secureWithPasskey(page);
  await expect(page).toHaveURL(/\/onboarding$/);

  // Five steps; the answers are saved as you go.
  await expect(
    page.getByRole("heading", { name: /Tell us about your/ }),
  ).toBeVisible();
  await page.getByLabel("Legal name").fill("Acme Payments Ltd");
  await page.getByLabel("Country of incorporation").selectOption("NG");
  await page.getByLabel("Registration number").fill("RC123456");
  await page.getByLabel("Website").fill("https://acme.example");
  await page.getByText("Payments", { exact: true }).click();
  await page.getByRole("button", { name: "Save and continue" }).click();

  await expect(
    page.getByRole("heading", { name: /How you are/ }),
  ).toBeVisible();
  await page
    .getByRole("group", { name: /licensed or registered/ })
    .getByText("No")
    .click();
  await page
    .getByRole("group", { name: /AML\/CFT programme/ })
    .getByText("Yes")
    .click();
  await page
    .getByLabel("Compliance officer", { exact: true })
    .fill("Grace Hopper");
  await page
    .getByLabel("Compliance officer's email")
    .fill("grace@acme.example");
  await page.getByRole("button", { name: "Save and continue" }).click();

  await expect(
    page.getByRole("heading", { name: /What you will/ }),
  ).toBeVisible();
  await page
    .getByRole("group", { name: "Events you will send" })
    .getByText("Payments")
    .click();
  await page.getByText("100k – 1M").click();
  await page.getByLabel("Countries you serve").selectOption("NG");
  await page.getByText("Within 3 months").click();
  await page.getByRole("button", { name: "Save and continue" }).click();

  await expect(
    page.getByRole("heading", { name: /How you handle/ }),
  ).toBeVisible();
  await page.getByText("NDPR (Nigeria)").click();
  await page.getByText("Africa", { exact: true }).click();
  await page.getByText(/I accept the RiskOS terms of service/).click();
  await page.getByText(/I accept the data processing agreement/).click();
  await page.getByRole("button", { name: "Save and continue" }).click();

  await expect(page.getByRole("heading", { name: /Review and/ })).toBeVisible();
  await expect(page.getByText("Acme Payments Ltd")).toBeVisible();
  await page.getByRole("button", { name: "Submit for review" }).click();
  await expect(
    page.getByRole("heading", { name: /Thanks, we are/ }),
  ).toBeVisible();

  // In the console, production stays locked and says why.
  await page.getByRole("link", { name: "Go to the console" }).click();
  await expect(page.getByText(/Production is/).first()).toBeVisible();
  await expect(page.getByRole("radio", { name: "Production" })).toHaveAttribute(
    "aria-disabled",
    "true",
  );

  expect(api.calls).toContain("POST /auth/signup/verify");
  expect(api.calls).toContain("POST /onboarding/submit");
  expect(api.problems).toEqual([]);
  expect(csp).toEqual([]);
});

test("sign up with Google: name the workspace, then secure it", async ({
  page,
}) => {
  const api = await mockApi(page);
  await virtualAuthenticator(page);
  await page.goto("/signup");
  await page.getByRole("button", { name: "Continue with Google" }).click();
  await expect(page).toHaveURL(/\/signup\/google$/);
  await expect(page.getByText("grace@acme.example")).toBeVisible();
  await page.getByLabel("Business name").fill("Hopper Lending");
  await page.getByRole("button", { name: "Create workspace" }).click();
  await secureWithPasskey(page);
  await expect(page).toHaveURL(/\/onboarding$/);
  expect(api.calls).toContain("POST /auth/signup/complete");
  expect(api.problems).toEqual([]);
});
