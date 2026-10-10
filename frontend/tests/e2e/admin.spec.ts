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
  let limitsOverride: Record<string, unknown> | undefined;
  let status: "pending" | "approved" = "pending";
  const statusRequests: Record<string, unknown>[] = [
    {
      id: "tsr_other",
      tenant_id: "ten_2",
      tenant_name: "Beta Remit",
      action: "suspend",
      reason: "Fraud ring suspected: ticket SEC-7781",
      status: "pending",
      requested_by: other,
      created_at: NOW,
    },
  ];
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
          status_requests_pending: 1,
        });
      case "/tenant-status-requests":
        return json(route, { data: statusRequests });
      case "/tenants/ten_1/limits": {
        if (req.method() === "PUT") {
          const body = req.postDataJSON() as {
            per_key: unknown;
            per_tenant: unknown;
            reason: string;
          };
          limitsOverride = {
            per_key: body.per_key,
            per_tenant: body.per_tenant,
            reason: body.reason,
            updated_by: me,
            updated_at: NOW,
          };
        }
        return json(route, {
          defaults: {
            per_key: { per_second: 50, burst: 100 },
            per_tenant: { per_second: 500, burst: 1000 },
          },
          ...(limitsOverride ? { override: limitsOverride } : {}),
        });
      }
      case "/tenants/ten_1/status-requests": {
        const body = req.postDataJSON() as { action: string; reason: string };
        const created = {
          id: "tsr_mine",
          tenant_id: "ten_1",
          tenant_name: "Acme Payments",
          action: body.action,
          reason: body.reason,
          status: "pending",
          requested_by: me,
          created_at: NOW,
        };
        statusRequests.unshift(created);
        return json(route, created, 201);
      }
      case "/tenant-status-requests/tsr_other/approve": {
        const r = statusRequests.find((x) => x.id === "tsr_other");
        Object.assign(r ?? {}, {
          status: "approved",
          decided_by: me,
          decided_at: NOW,
        });
        return json(route, r);
      }
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
  // Operations administrators are not offered the staff roster.
  await expect(
    page.getByRole("link", { name: "Staff", exact: true }),
  ).toHaveCount(0);
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

  // Asking for a suspension waits for someone else; deciding another's request is one click.
  await page
    .getByLabel("Reason", { exact: true })
    .fill("Fraud ring suspected: ticket SEC-7790");
  await page.getByRole("button", { name: "Request suspension" }).click();
  await expect(
    page.getByText("A suspension requested by Ops A is waiting"),
  ).toBeVisible();

  // Request limits start as the platform's; saving an override says so.
  await expect(page.getByText("Platform limits")).toBeVisible();
  await page.getByLabel("Per key: burst").fill("20");
  await page.getByLabel("Limits reason").fill("Abusive polling: ticket OPS-77");
  await page.getByRole("button", { name: "Save limits" }).click();
  await expect(page.getByText("Override in force")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Remove override" }),
  ).toBeVisible();

  await page.getByRole("link", { name: "Status requests" }).click();
  const decisions = page.getByRole("table", { name: "Tenant status requests" });
  await expect(decisions).toContainText("Beta Remit");
  await expect(decisions).toContainText("Awaiting another staff member");
  await decisions.getByRole("button", { name: "Approve" }).click();
  await expect(decisions).toContainText("Approved");

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

test("staff console: the roster is for security administrators, and disabling is one click", async ({
  page,
}) => {
  const accounts: Record<string, unknown>[] = [
    {
      id: "stf_me",
      email: "sec-b@riskos.example",
      name: "Sec B",
      role: "security_admin",
      status: "active",
      passkeys: 2,
      last_sign_in: NOW,
      enrollment_pending: false,
      created_at: NOW,
    },
    {
      id: "stf_ops",
      email: "ops-a@riskos.example",
      name: "Ops A",
      role: "operations_admin",
      status: "active",
      passkeys: 1,
      last_sign_in: null,
      enrollment_pending: false,
      created_at: NOW,
    },
  ];
  const json = (route: Route, body: unknown, code = 200) =>
    route.fulfill({
      status: code,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  await page.route("**/api/admin/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname.replace(
      "/api/admin/v1",
      "",
    );
    switch (path) {
      case "/auth/session":
        return json(route, {
          staff: {
            id: "stf_me",
            name: "Sec B",
            email: "sec-b@riskos.example",
            role: "security_admin",
          },
          csrf_token: CSRF,
          expires_at: "2026-10-01T14:00:00Z",
          idle_expires_at: "2026-10-01T10:15:00Z",
        });
      case "/staff":
        return json(route, { data: accounts });
      case "/staff/stf_ops/disable":
        Object.assign(accounts[1] ?? {}, { status: "disabled" });
        return json(route, accounts[1]);
      default:
        return json(route, { data: [] });
    }
  });

  await page.goto("/admin/staff");
  await page
    .getByRole("link", { name: "Staff", exact: true })
    .first()
    .waitFor();
  const table = page.getByRole("table", { name: "Staff accounts" });
  await expect(table).toContainText("Sec B (you)");
  await expect(table).toContainText("Never");
  await expect(table.getByText("Your account")).toBeVisible();
  await table.getByRole("button", { name: "Disable" }).click();
  await expect(table).toContainText("Disabled");
  await expect(table.getByRole("button", { name: "Enable" })).toBeVisible();
});

test("staff console: break-glass suspension is flagged and waits for someone else's review", async ({
  page,
}) => {
  const requests: Record<string, unknown>[] = [
    {
      id: "tsr_em_other",
      tenant_id: "ten_2",
      tenant_name: "Beta Remit",
      action: "suspend",
      reason: "Credential stuffing in progress: INC-204",
      status: "approved",
      requested_by: other,
      decided_by: other,
      decided_at: NOW,
      created_at: NOW,
      emergency: true,
    },
  ];
  const json = (route: Route, body: unknown, code = 200) =>
    route.fulfill({
      status: code,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  await page.route("**/api/admin/v1/**", async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname.replace("/api/admin/v1", "");
    switch (path) {
      case "/auth/session":
        return json(route, {
          staff: {
            ...me,
            email: "sec-me@riskos.example",
            role: "security_admin",
          },
          csrf_token: CSRF,
          expires_at: "2026-10-01T14:00:00Z",
          idle_expires_at: "2026-10-01T10:15:00Z",
        });
      case "/tenants/ten_1":
        return json(route, {
          tenant,
          since: "2026-09-30T10:00:00Z",
          security: {
            tenant_id: "ten_1",
            tenant_name: "Acme Payments",
            tenant_status: "active",
            degraded_evaluations: 0,
            open_security_alerts: 0,
            failed_logins: 0,
            disabled_webhooks: 0,
          },
          recent_staff_actions: [],
        });
      case "/tenant-status-requests":
        return json(route, { data: requests });
      case "/tenants/ten_1/limits":
        return json(route, {
          defaults: {
            per_key: { per_second: 50, burst: 100 },
            per_tenant: { per_second: 500, burst: 1000 },
          },
        });
      case "/tenants/ten_1/emergency-suspension": {
        const body = req.postDataJSON() as { reason: string };
        const created = {
          id: "tsr_em_mine",
          tenant_id: "ten_1",
          tenant_name: "Acme Payments",
          action: "suspend",
          reason: body.reason,
          status: "approved",
          requested_by: me,
          decided_by: me,
          decided_at: NOW,
          created_at: NOW,
          emergency: true,
        };
        requests.unshift(created);
        return json(route, created, 201);
      }
      case "/tenant-status-requests/tsr_em_other/review": {
        Object.assign(requests.find((r) => r.id === "tsr_em_other") ?? {}, {
          reviewed_by: me,
          reviewed_at: NOW,
        });
        return json(
          route,
          requests.find((r) => r.id === "tsr_em_other"),
        );
      }
      default:
        return json(route, { data: [] });
    }
  });

  // Another staff member's emergency suspension waits for this person's review.
  await page.goto("/admin/status-requests");
  const table = page.getByRole("table", { name: "Tenant status requests" });
  await expect(table).toContainText("Emergency · awaiting review");
  await table.getByRole("button", { name: "Mark reviewed" }).click();
  await expect(table).toContainText("reviewed by Ops A");

  // Suspending in an emergency needs a reason and an explicit confirmation, then is flagged.
  await page.goto("/admin/tenants/ten_1");
  await page
    .getByLabel("Reason", { exact: true })
    .last()
    .fill("Credential stuffing in progress: INC-205");
  await page.getByRole("checkbox").check();
  await page.getByRole("button", { name: "Suspend now" }).click();
  await page.goto("/admin/status-requests");
  await expect(table).toContainText("Acme Payments");
  await expect(table).toContainText("Awaiting review by another staff member");
});
