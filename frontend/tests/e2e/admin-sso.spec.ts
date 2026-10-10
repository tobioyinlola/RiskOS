import { expect, test, type Page, type Route } from "@playwright/test";

/**
 * Staff sign-in with single sign-on, against a mocked admin API: the options the sign-in page
 * offers and how the callback page handles a failed provider step. (The passkey step needs a
 * real authenticator and is covered by the integration tests.)
 */
const json = (route: Route, body: unknown, code = 200) =>
  route.fulfill({
    status: code,
    contentType: "application/json",
    body: JSON.stringify(body),
  });

const error = (code: string, message: string) => ({
  error: { code, message, request_id: "r" },
});

async function mockAdmin(
  page: Page,
  methods: { sso_enabled: boolean; sso_required: boolean },
) {
  const calls: { path: string; body: unknown }[] = [];
  await page.route("**/api/admin/v1/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace("/api/admin/v1", "");
    calls.push({
      path,
      body: req.method() === "POST" ? req.postDataJSON() : undefined,
    });
    switch (path) {
      case "/auth/methods":
        return json(route, methods);
      case "/auth/sso/start":
        return json(route, {
          redirect_url: "/admin/sso/callback?code=c1&state=s1",
        });
      case "/auth/sso/callback":
        return json(
          route,
          error(
            "SSO_FAILED",
            "Single sign-on did not complete. Start again from the sign-in page.",
          ),
          401,
        );
      default:
        return json(route, error("UNAUTHENTICATED", "Sign in."), 401);
    }
  });
  return calls;
}

test("single sign-on required: only the provider is offered, and a failure starts over", async ({
  page,
}) => {
  const calls = await mockAdmin(page, {
    sso_enabled: true,
    sso_required: true,
  });
  await page.goto("/admin/login");

  await expect(
    page.getByRole("button", { name: "Continue with single sign-on" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Sign in with passkey" }),
  ).toHaveCount(0);

  await page
    .getByRole("button", { name: "Continue with single sign-on" })
    .click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Single sign-on did not complete",
  );
  expect(page.url()).not.toContain("code=");
  await expect(
    page.getByRole("link", { name: "Back to sign-in" }),
  ).toBeVisible();

  const callback = calls.filter((c) => c.path === "/auth/sso/callback");
  expect(callback).toHaveLength(1);
  expect(callback[0]?.body).toEqual({ code: "c1", state: "s1" });
});

test("single sign-on optional: both ways are offered", async ({ page }) => {
  await mockAdmin(page, { sso_enabled: true, sso_required: false });
  await page.goto("/admin/login");
  await expect(
    page.getByRole("button", { name: "Continue with single sign-on" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Sign in with passkey" }),
  ).toBeVisible();
});

test("no provider: passkey only", async ({ page }) => {
  await mockAdmin(page, { sso_enabled: false, sso_required: false });
  await page.goto("/admin/login");
  await expect(
    page.getByRole("button", { name: "Sign in with passkey" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Continue with single sign-on" }),
  ).toHaveCount(0);
});

test("the provider's own error is shown without calling the API", async ({
  page,
}) => {
  const calls = await mockAdmin(page, {
    sso_enabled: true,
    sso_required: true,
  });
  await page.goto("/admin/sso/callback?error=access_denied&state=s1");
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "did not complete the sign-in",
  );
  expect(calls.filter((c) => c.path.startsWith("/auth/sso"))).toHaveLength(0);
});
