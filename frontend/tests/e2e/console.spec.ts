import { expect, test, type Page } from "@playwright/test";

import { navSections } from "../../lib/navigation";
import { eventSummary, mockApi } from "./mock-api";

const PASSWORD = "correct horse battery staple";

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

async function signIn(page: Page) {
  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
}

test("sign-in with a new passkey, every section, sign-out and back in", async ({
  page,
  isMobile,
}) => {
  const csp = watchCsp(page);
  const api = await mockApi(page);
  await virtualAuthenticator(page);

  // Without a session the console sends you to sign in and back afterwards.
  await page.goto("/alerts");
  await expect(page).toHaveURL(/\/login\?next=%2Falerts$/);

  await page.getByLabel("Email").fill("ada@example.com");
  await page.getByLabel("Password").fill("wrong password");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(
    page.getByText("The email or password is incorrect."),
  ).toBeVisible();

  // First sign-in: a second factor is mandatory; the passkey is created by the browser.
  await page.getByLabel("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Add a second factor" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Create a passkey" }).click();

  await expect(
    page.getByRole("heading", { name: "Save your recovery codes" }),
  ).toBeVisible();
  const cont = page.getByRole("button", { name: "Continue to RiskOS" });
  await expect(cont).toBeDisabled();
  await page.getByLabel("I have saved these codes somewhere safe").check();
  await cont.click();

  await expect(page).toHaveURL(/\/alerts$/);
  await expect(page).toHaveTitle("Alerts · RiskOS");
  await expect(
    page.getByText("High-risk transfer to a new beneficiary"),
  ).toBeVisible();
  await expect(page.getByLabel("High, score 81").first()).toBeVisible();

  // Every section is reachable from the navigation.
  for (const section of navSections) {
    if (isMobile) {
      await page.getByRole("button", { name: "Open navigation" }).click();
    }
    await page
      .getByRole("navigation", { name: "Primary" })
      .getByRole("link", { name: section.label, exact: true })
      .click();
    await expect(page).toHaveURL(section.href);
    await expect(
      page.getByRole("heading", {
        level: 1,
        // The overview greets the user; every other section is titled by its label.
        name: section.href === "/" ? /^Welcome back/ : section.label,
      }),
    ).toBeVisible();
  }

  // Production is marked unmistakably.
  await page.getByRole("radio", { name: "Production" }).click();
  await expect(
    page.getByText("Production: live data and live keys"),
  ).toBeVisible();
  await page.getByRole("radio", { name: "Test" }).click();

  await page.getByRole("button", { name: "Ada Lovelace" }).click();
  await page.getByRole("menuitem", { name: "Sign out" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // Signing in again uses the passkey just created.
  await signIn(page);
  await expect(
    page.getByRole("heading", { name: "Verify it's you" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Use your passkey" }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: /^Welcome back/ }),
  ).toBeVisible();

  expect(api.problems).toEqual([]);
  expect(api.calls).toContain("POST /auth/mfa/passkeys/register");
  expect(api.calls).toContain("POST /auth/mfa/passkeys/login");
  expect(csp).toEqual([]);
});

test("the alert queue works from the keyboard", async ({ page, isMobile }) => {
  test.skip(isMobile, "keyboard shortcuts are for desktop");
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/alerts");
  await expect(
    page.getByText("High-risk transfer to a new beneficiary"),
  ).toBeVisible();

  await page.keyboard.press("j");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();

  await page.keyboard.press("?");
  await expect(
    page.getByRole("dialog", { name: "Keyboard shortcuts" }),
  ).toBeVisible();
  expect(api.problems).toEqual([]);
});

test("a production rule change is approved by a second person", async ({
  page,
}) => {
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/rules");
  await page.getByRole("radio", { name: "Production" }).click();

  const pending = page.getByRole("table", {
    name: "Rule changes awaiting approval",
  });
  await expect(pending.getByText("Large transfer")).toBeVisible();
  await pending.getByRole("button", { name: "Review" }).click();

  const drawer = page.getByRole("dialog");
  await expect(drawer.getByText("A new rule")).toBeVisible();
  await expect(drawer.getByText("+25")).toBeVisible();
  await drawer.getByRole("button", { name: "Approve and apply" }).click();

  await expect(drawer).toBeHidden();
  await expect(pending).toBeHidden();
  expect(api.calls).toContain("POST /rule-changes/rch_01/approve");
  expect(api.problems).toEqual([]);
});

test("the shell: collapsible navigation, command palette and density", async ({
  page,
  isMobile,
}) => {
  test.skip(
    isMobile,
    "the navigation rail and palette shortcut are for desktop",
  );
  const csp = watchCsp(page);
  const api = await mockApi(page);
  api.signedIn = true;
  await page.goto("/");
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(nav.getByRole("link", { name: "Alerts" })).toContainText(
    "Alerts",
  );

  // Collapse to the icon rail; links keep their names; the choice survives a reload.
  await page.getByRole("button", { name: "Collapse navigation" }).click();
  await expect(nav.getByRole("link", { name: "Alerts" })).not.toContainText(
    "Alerts",
  );
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Expand navigation" }),
  ).toBeVisible();
  await page.keyboard.press("[");
  await expect(
    page.getByRole("button", { name: "Collapse navigation" }),
  ).toBeVisible();

  // The command palette jumps to a section.
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog", { name: "Command palette" });
  await expect(palette).toBeVisible();
  await page.keyboard.type("audit");
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/audit-logs$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "Audit log" }),
  ).toBeVisible();

  // Density is a preference that survives a reload. The product is light only.
  await expect(page.locator("html")).not.toHaveAttribute("data-theme");
  await page.getByRole("button", { name: "Use compact rows" }).click();
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Use default rows" }),
  ).toBeVisible();

  expect(api.problems).toEqual([]);
  expect(csp).toEqual([]);
});

test("investigate: live monitor, transactions, entity profile, analytics", async ({
  page,
  isMobile,
}) => {
  test.skip(isMobile, "hover holds are a desktop interaction");
  const csp = watchCsp(page);
  const api = await mockApi(page);
  api.signedIn = true;
  api.events = [eventSummary(12, "usr_1001")];

  // The live monitor shows arrivals; while the pointer is over the table they wait.
  await page.goto("/live");
  const table = page.getByRole("table", { name: "Live events" });
  await expect(table.getByText("usr_1001")).toBeVisible();
  api.events.unshift(eventSummary(81));
  await expect(table.getByText("usr_4812")).toBeVisible({ timeout: 6000 });
  await table.hover();
  api.events.unshift(eventSummary(55, "usr_2002"));
  await expect(
    page.getByRole("button", { name: "Show 1 new event" }),
  ).toBeVisible({ timeout: 6000 });
  await expect(table.getByText("usr_2002")).toHaveCount(0);
  await page.getByRole("button", { name: "Show 1 new event" }).click();
  await expect(table.getByText("usr_2002")).toBeVisible();

  // Transactions show the amount; the user opens their profile.
  await page.goto("/transactions");
  const txns = page.getByRole("table", { name: "Transactions" });
  await expect(txns.getByText("4,500.00 NGN").first()).toBeVisible();
  await txns.getByRole("link", { name: "usr_4812" }).first().click();
  await expect(page).toHaveURL(/\/entities\/user\/usr_4812$/);
  await expect(
    page.getByRole("heading", { level: 1, name: "User profile" }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("table", { name: "Devices" })
      .getByRole("link", { name: "dev_2" }),
  ).toBeVisible();

  // The user goes on the deny list from their profile, and comes off it on the deny-list page.
  await page.getByRole("button", { name: "Add to deny list" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Identifier")).toHaveValue("usr_4812");
  await dialog.getByLabel("Reason (optional)").fill("Confirmed mule");
  await dialog.getByRole("button", { name: "Add to deny list" }).click();
  await expect(
    page.getByRole("button", { name: "On the deny list" }),
  ).toBeDisabled();
  expect(api.deny).toHaveLength(1);
  await page.goto("/deny-lists");
  const deny = page.getByRole("table", { name: "Deny list" });
  await expect(deny).toContainText("Confirmed mule");
  await deny.getByRole("button", { name: "Remove" }).click();
  await page.getByRole("button", { name: "Remove from deny list" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByText("Nothing on the deny list")).toBeVisible();

  // Analytics: a chart with a table view.
  await page.goto("/analytics");
  await expect(
    page.getByRole("img", { name: /Events per bucket by risk band/ }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Table" }).click();
  await expect(
    page.getByRole("table", { name: "Events per bucket" }),
  ).toContainText("12");
  await expect(
    page.getByRole("table", { name: "Signals that fired most" }),
  ).toContainText("New device for this user");

  expect(api.problems).toEqual([]);
  expect(csp).toEqual([]);
});

test("single sign-on: the provider round trip ends signed in", async ({
  page,
}) => {
  const csp = watchCsp(page);
  const api = await mockApi(page);
  await page.goto("/login");
  await page
    .getByRole("button", { name: "Sign in with single sign-on" })
    .click();
  await page.getByLabel("Work email").fill("ada@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page).not.toHaveURL(/\/(login|sso)/);
  expect(api.calls).toContain("POST /auth/sso/callback");
  expect(api.calls.filter((c) => c === "POST /auth/sso/callback")).toHaveLength(
    1,
  );
  expect(api.problems).toEqual([]);
  expect(csp).toEqual([]);
});

test("pages carry security headers and a nonce-based CSP", async ({
  request,
}) => {
  const res = await request.get("/login");
  expect(res.status()).toBe(200);
  const h = res.headers();
  expect(h["x-content-type-options"]).toBe("nosniff");
  expect(h["x-frame-options"]).toBe("DENY");
  expect(h["x-powered-by"]).toBeUndefined();
  const csp = h["content-security-policy"] ?? "";
  const nonce = /'nonce-([^']+)'/.exec(csp)?.[1];
  expect(nonce).toBeTruthy();
  expect(csp).toContain("frame-ancestors 'none'");
  expect(csp).not.toContain("unsafe-eval");

  // Every script on the page carries this response's nonce.
  const html = await res.text();
  const scripts = html.match(/<script\b[^>]*>/gi) ?? [];
  expect(scripts.length).toBeGreaterThan(0);
  for (const tag of scripts) expect(tag).toContain(`nonce="${nonce}"`);

  // A second request gets a different nonce.
  const again = await request.get("/login");
  expect(again.headers()["content-security-policy"]).not.toContain(
    `'nonce-${nonce}'`,
  );
});

test("the public site: landing page, indexable, no session needed", async ({
  page,
}) => {
  const csp = watchCsp(page);
  const api = await mockApi(page);
  await page.goto("/home");
  await expect(
    page.getByRole("heading", { level: 1, name: /Stop fraud before it/ }),
  ).toBeVisible();
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute(
    "content",
    "index, follow",
  );
  await page.getByRole("tab", { name: "Developers" }).click();
  await expect(
    page.getByRole("heading", { name: /One API call per/ }),
  ).toBeVisible();
  await page
    .getByRole("navigation", { name: "Site" })
    .getByRole("link", { name: "Log in" })
    .click();
  await expect(page).toHaveURL(/\/login$/);
  expect(api.calls.filter((c) => !c.startsWith("GET /auth"))).toEqual([]);
  expect(csp).toEqual([]);
});
