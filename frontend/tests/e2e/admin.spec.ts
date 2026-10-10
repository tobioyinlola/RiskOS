import { expect, test, type Route } from "@playwright/test";

/**
 * The staff console against a mocked admin API: the session gate, the cross-tenant security
 * view and four-eyes approval of support access.
 */
const NOW = "2026-10-01T10:00:00Z";
const CSRF = "staff-csrf";

const me = { id: "stf_me", name: "Ops A" };
const other = { id: "stf_other", name: "Sec B" };

const tenant = {
  id: "ten_1",
  name: "Acme Payments",
  region: "eu-west-1",
  status: "active",
  origin: "self_serve",
  production_enabled: false,
  created_at: NOW,
  active_users: 4,
  active_api_keys: 2,
  onboarding_status: "submitted",
};

function request(status: "pending" | "approved", decidedBy?: typeof me) {
  return {
    id: "sac_1",
    tenant_id: "ten_1",
    tenant_name: "Acme Payments",
    requested_by: other,
    reason: "Customer ticket 4812: webhook failures",
    duration_minutes: 60,
    status,
    active: status === "approved",
    created_at: NOW,
    ...(decidedBy
      ? {
          decided_by: decidedBy,
          decided_at: NOW,
          expires_at: "2026-10-01T11:00:00Z",
        }
      : {}),
  };
}

test("staff console: session gate, security view, four-eyes approval", async ({
  page,
}) => {
  let signedIn = false;
  let status: "pending" | "approved" = "pending";
  const problems: string[] = [];
  const json = (route: Route, body: unknown, code = 200) =>
    route.fulfill({
      status: code,
      contentType: "application/json",
      body: JSON.stringify(body),
    });

  await page.route("**/api/admin/v1/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace("/api/admin/v1", "");
    if (
      req.method() !== "GET" &&
      signedIn &&
      req.headers()["x-csrf-token"] !== CSRF
    ) {
      problems.push(`${req.method()} ${path} without the CSRF token`);
    }
    if (path === "/auth/session") {
      return signedIn
        ? json(route, {
            staff: {
              ...me,
              email: "ops-a@riskos.example",
              role: "operations_admin",
            },
            csrf_token: CSRF,
            expires_at: "2026-10-01T14:00:00Z",
            idle_expires_at: "2026-10-01T10:15:00Z",
          })
        : json(
            route,
            {
              error: {
                code: "UNAUTHENTICATED",
                message: "Sign in.",
                request_id: "r",
              },
            },
            401,
          );
    }
    switch (path) {
      case "/security":
        return json(route, {
          since: "2026-09-30T10:00:00Z",
          data: [
            {
              tenant_id: "ten_1",
              tenant_name: "Acme Payments",
              tenant_status: "active",
              degraded_evaluations: 3,
              open_security_alerts: 1,
              failed_logins: 7,
              disabled_webhooks: 0,
            },
          ],
        });
      case "/overview":
        return json(route, {
          tenants_total: 12,
          tenants_active: 10,
          tenants_suspended: 2,
          onboarding_submitted: 3,
          support_pending: 1,
          support_active: 0,
          staff_active: 4,
        });
      case "/tenants":
        return json(route, { data: [tenant] });
      case "/tenants/ten_1":
        return json(route, {
          tenant,
          since: "2026-09-30T10:00:00Z",
          security: {
            tenant_id: "ten_1",
            tenant_name: "Acme Payments",
            tenant_status: "active",
            degraded_evaluations: 3,
            open_security_alerts: 1,
            failed_logins: 7,
            disabled_webhooks: 0,
          },
          recent_staff_actions: [
            {
              id: "1",
              occurred_at: NOW,
              actor: "stf_other",
              action: "support_access.requested",
              tenant_id: "ten_1",
              resource_id: "sac_1",
              outcome: "success",
              metadata: {},
            },
          ],
        });
      case "/support-access":
        return json(route, {
          data: [request(status, status === "approved" ? me : undefined)],
        });
      case "/support-access/sac_1/approve":
        status = "approved";
        return json(route, request("approved", me));
      default:
        return json(
          route,
          {
            error: {
              code: "NOT_FOUND",
              message: "Not found.",
              request_id: "r",
            },
          },
          404,
        );
    }
  });

  // Without a staff session, the console sends people to the passkey sign-in.
  await page.goto("/admin");
  await expect(page).toHaveURL(/\/admin\/login$/);
  await expect(
    page.getByRole("button", { name: "Sign in with passkey" }),
  ).toBeVisible();

  signedIn = true;
  await page.goto("/admin");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Overview");
  await expect(
    page.getByRole("link", { name: /^Onboarding reviews: 3/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /^Support requests: 1/ }),
  ).toBeVisible();
  const table = page.getByRole("table", { name: "Security across tenants" });
  await expect(table).toContainText("Acme Payments");
  await expect(table).toContainText("7");

  // The directory finds a tenant by name, and its page shows state, signals and staff activity.
  await page.getByRole("link", { name: "Tenants", exact: true }).click();
  const tenants = page.getByRole("table", { name: "Tenants" });
  await expect(tenants).toContainText("Acme Payments");
  await expect(tenants).toContainText("Locked");
  await page.getByLabel("Search").fill("nothing like it");
  await expect(page.getByText("No tenant matches")).toBeVisible();
  await page.getByLabel("Search").fill("acme");
  await page.getByRole("link", { name: "Acme Payments" }).click();
  await expect(page.getByRole("heading", { level: 1 })).toHaveText(
    "Acme Payments",
  );
  await expect(page.getByText("Submitted")).toBeVisible();
  await expect(
    page.getByRole("table", { name: "Staff activity on this tenant" }),
  ).toContainText("support_access.requested");

  await page.getByRole("link", { name: "Support access" }).click();
  const requests = page.getByRole("table", { name: "Support access requests" });
  await expect(requests).toContainText("Customer ticket 4812");
  await requests.getByRole("button", { name: "Approve" }).click();
  await expect(requests).toContainText("Approved");
  await expect(
    requests.getByRole("button", { name: "End access" }),
  ).toBeVisible();
  expect(problems).toEqual([]);
});
