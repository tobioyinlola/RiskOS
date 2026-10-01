import type { Page, Route } from "@playwright/test";

/**
 * A small in-memory stand-in for the RiskOS API, served through Playwright's request
 * interception, so the browser tests run without a backend. It checks what the browser sends
 * where the console's own logic is at stake (CSRF, environment, passkey encodings); the real
 * API's behaviour is covered by the backend's integration tests and the stack test.
 */

const NOW = "2026-09-30T12:00:00Z";
const b64url = (s: string) =>
  Buffer.from(s)
    .toString("base64")
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");

const ALL_PERMISSIONS = [
  "rules:approve",
  "api_keys:read",
  "api_keys:write",
  "audit_logs:read",
  "events:read",
  "rules:read",
  "rules:write",
  "risk:configure",
  "alerts:read",
  "alerts:write",
  "cases:read",
  "cases:write",
  "webhooks:read",
  "webhooks:write",
  "team:read",
  "team:write",
];

const THRESHOLDS = { low: 20, elevated: 50, high: 70, critical: 90 };

const ALERT = {
  id: "alt_01",
  kind: "risk",
  subject: { type: "user", ref: "usr_4812" },
  title: "High-risk transfer to a new beneficiary",
  severity: "high",
  status: "open",
  assignee: null,
  case_id: null,
  max_score: 81,
  evaluation_count: 2,
  first_seen_at: NOW,
  last_seen_at: NOW,
  created_at: NOW,
  updated_at: NOW,
};

export interface MockApi {
  /** Requests the console made, as "METHOD /path". */
  calls: string[];
  /** Problems with what the browser sent; a test should end with none. */
  problems: string[];
  signedIn: boolean;
  hasPasskey: boolean;
  changeStatus: "pending" | "approved";
}

const RULE_CHANGE_ID = "rch_01";

function ruleChange(status: "pending" | "approved") {
  return {
    id: RULE_CHANGE_ID,
    rule_id: "rul_01",
    code: "large_transfer",
    kind: "create",
    base_version: null,
    status,
    proposed: {
      name: "Large transfer",
      description: "",
      severity: "high",
      weight: 25,
      mode: "shadow",
      enabled: true,
      condition: { field: "transaction.amount", op: "gte", value: 1000000 },
      restored_from: null,
    },
    proposed_by: "usr_grace",
    proposed_at: NOW,
    decided_by: status === "approved" ? "usr_admin" : null,
    decided_at: status === "approved" ? NOW : null,
    reason: null,
    applied_version: status === "approved" ? 1 : null,
  };
}

export async function mockApi(page: Page): Promise<MockApi> {
  const state: MockApi = {
    calls: [],
    problems: [],
    signedIn: false,
    hasPasskey: false,
    changeStatus: "pending",
  };
  const csrf = "csrf-test-token";
  let challenge = "";

  const session = () => ({
    state: "active",
    user: {
      id: "usr_admin",
      email: "ada@example.com",
      name: "Ada Lovelace",
      role: "customer_admin",
    },
    tenant: { id: "ten_1", name: "Acme Payments" },
    permissions: ALL_PERMISSIONS,
    environments: ["test", "production"],
    csrf_token: csrf,
    expires_at: "2026-09-30T23:59:59Z",
    reauth_valid_until: null,
    mfa: {
      totp: false,
      passkeys: state.hasPasskey
        ? [{ id: "pk_1", name: "Passkey", created_at: NOW, last_used_at: null }]
        : [],
      recovery_codes_remaining: 10,
    },
  });

  const checkClientData = (credential: unknown, type: string) => {
    const c = credential as
      { id?: string; response?: Record<string, string> } | undefined;
    const raw = c?.response?.clientDataJSON;
    if (!c?.id || !raw) {
      state.problems.push(`${type}: credential is missing fields`);
      return;
    }
    const data = JSON.parse(Buffer.from(raw, "base64url").toString()) as Record<
      string,
      string
    >;
    if (data.type !== type)
      state.problems.push(`${type}: client data type is ${data.type}`);
    if (data.challenge !== challenge)
      state.problems.push(`${type}: challenge does not match`);
  };

  const json = (route: Route, body: unknown, status = 200) =>
    route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(body),
    });

  const error = (route: Route, status: number, code: string, message: string) =>
    json(route, { error: { code, message, request_id: "req_test" } }, status);

  await page.route("**/api/v1/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");
    const method = req.method();
    state.calls.push(`${method} ${path}`);
    const body = req.postDataJSON() as Record<string, unknown> | null;

    if (
      state.signedIn &&
      method !== "GET" &&
      req.headers()["x-csrf-token"] !== csrf
    ) {
      state.problems.push(`${method} ${path} without the CSRF token`);
    }

    // Sign-in and second factors.
    if (path === "/auth/session") {
      return state.signedIn
        ? json(route, session())
        : error(route, 401, "UNAUTHENTICATED", "Sign in.");
    }
    if (path === "/auth/login") {
      if (body?.password !== "correct horse battery staple") {
        return error(
          route,
          401,
          "INVALID_CREDENTIALS",
          "The email or password is incorrect.",
        );
      }
      return json(
        route,
        state.hasPasskey
          ? { state: "mfa_required", mfa_methods: ["passkey", "recovery_code"] }
          : { state: "enrollment_required" },
      );
    }
    if (path === "/auth/mfa/passkeys/register/options") {
      challenge = b64url(`register-${Date.now()}`);
      return json(route, {
        publicKey: {
          challenge,
          rp: { name: "RiskOS", id: url.hostname },
          user: {
            id: b64url("usr_admin"),
            name: "ada@example.com",
            displayName: "Ada Lovelace",
          },
          pubKeyCredParams: [
            { type: "public-key", alg: -7 },
            { type: "public-key", alg: -257 },
          ],
          authenticatorSelection: {
            residentKey: "preferred",
            userVerification: "required",
          },
          timeout: 60000,
          attestation: "none",
        },
      });
    }
    if (path === "/auth/mfa/passkeys/register") {
      checkClientData(body?.credential, "webauthn.create");
      state.hasPasskey = true;
      state.signedIn = true;
      return json(route, {
        state: "active",
        recovery_codes: Array.from(
          { length: 10 },
          (_, i) => `abcd-efgh-ijkl-000${i}`,
        ),
      });
    }
    if (path === "/auth/mfa/passkeys/login/options") {
      challenge = b64url(`login-${Date.now()}`);
      return json(route, {
        publicKey: {
          challenge,
          rpId: url.hostname,
          userVerification: "required",
          timeout: 60000,
        },
      });
    }
    if (path === "/auth/mfa/passkeys/login") {
      checkClientData(body?.credential, "webauthn.get");
      state.signedIn = true;
      return json(route, { state: "active" });
    }
    if (path === "/auth/logout") {
      state.signedIn = false;
      return route.fulfill({ status: 204 });
    }

    if (!state.signedIn)
      return error(route, 401, "UNAUTHENTICATED", "Sign in.");
    if (
      !["test", "production"].includes(
        req.headers()["riskos-environment"] ?? "",
      )
    ) {
      state.problems.push(`${method} ${path} without an environment`);
    }

    // Console data.
    switch (path) {
      case `/alerts/${ALERT.id}`:
        return json(route, {
          alert: ALERT,
          evaluations: [
            {
              evaluation_id: "eval_01",
              event_id: "evt_01",
              event_type: "transaction",
              risk_score: 81,
              risk_level: "high",
              decision: "step_up",
              signals: [
                {
                  code: "new_beneficiary",
                  label: "New beneficiary",
                  weight: 30,
                },
                {
                  code: "new_device",
                  label: "New device for this user",
                  weight: 15,
                },
              ],
              degraded_signals: [],
              evaluated_at: NOW,
            },
          ],
          activity: [
            {
              id: "act_01",
              action: "created",
              actor: "system",
              detail: {},
              created_at: NOW,
            },
          ],
        });
      case "/risk/policy":
        return json(route, {
          engine_version: "risk-engine-2.0.0",
          thresholds: THRESHOLDS,
          custom_calibration: false,
          alert_min_decision: "review",
          bands: [
            {
              band: "minimal",
              min_score: 0,
              max_score: 19,
              default_decision: "allow",
            },
            {
              band: "low",
              min_score: 20,
              max_score: 49,
              default_decision: "allow",
            },
            {
              band: "elevated",
              min_score: 50,
              max_score: 69,
              default_decision: "review",
            },
            {
              band: "high",
              min_score: 70,
              max_score: 89,
              default_decision: "step_up",
            },
            {
              band: "critical",
              min_score: 90,
              max_score: 100,
              default_decision: "review",
            },
          ],
        });
      case "/risk/catalog":
        return json(route, {
          engine_version: "risk-engine-2.0.0",
          signals: [
            {
              code: "new_device",
              label: "New device for this user",
              default_weight: 15,
              weight: 15,
              source: "history",
              strengthenable: true,
            },
          ],
          fields: [
            {
              name: "amount",
              type: "number",
              operators: ["gt", "gte", "lt", "lte", "eq"],
            },
          ],
        });
      case "/risk/settings":
        return json(route, {
          weights: {},
          high_value_default: 1000000,
          high_value_by_currency: {},
          fail_policies: {},
          thresholds: THRESHOLDS,
          alert_min_decision: "review",
        });
      case "/alerts":
        return json(route, { data: [ALERT], next_cursor: null });
      case "/events":
      case "/cases":
      case "/audit-logs":
      case "/api-keys":
        return json(route, { data: [], next_cursor: null });
      case "/rule-changes":
        // Production has one proposal awaiting approval, made by someone else.
        return json(route, {
          data:
            req.headers()["riskos-environment"] === "production" &&
            state.changeStatus === "pending"
              ? [ruleChange(state.changeStatus)]
              : [],
        });
      case `/rule-changes/${RULE_CHANGE_ID}`:
        return json(route, ruleChange(state.changeStatus));
      case `/rule-changes/${RULE_CHANGE_ID}/approve`:
        state.changeStatus = "approved";
        return json(route, ruleChange("approved"));
      case "/rules":
      case "/webhooks":
      case "/team/invites":
        return json(route, { data: [] });
      case "/team/members":
        return json(route, {
          data: [
            {
              id: "usr_admin",
              email: "ada@example.com",
              name: "Ada Lovelace",
              role: "customer_admin",
              status: "active",
              mfa_enabled: true,
              last_login_at: NOW,
              created_at: NOW,
            },
          ],
        });
      default:
        state.problems.push(`unmocked ${method} ${path}`);
        return error(route, 404, "NOT_FOUND", "Not mocked.");
    }
  });
  return state;
}
