import type { Page, Route } from "@playwright/test";

import {
  applies,
  CLOCKS,
  ENGINEERING,
  GAPS,
  GLOSSARY,
  PACK,
  RULES,
} from "./compliance-fixtures";

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
  "compliance:read",
  "compliance:engineering",
  "compliance:write",
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
  /** Events the console can list, newest first; tests push to it to simulate arrivals. */
  events: ReturnType<typeof eventSummary>[];
  /** The deny list; the console adds and removes entries. */
  deny: Record<string, unknown>[];
  /** Whether the workspace's production environment is open (self-serve ones start locked). */
  productionEnabled: boolean;
  /** The workspace's compliance onboarding (null: created by an operator, none). */
  onboarding: { status: string; answers: Record<string, unknown> } | null;
  /** The signed-in user's permissions; tests narrow them to act as another role. */
  permissions: string[];
  /** The workspace's compliance profile (null until set). */
  complianceProfile: {
    jurisdiction: string;
    licence_type: string;
    capabilities: string[];
  } | null;
}

/** The answers onboarding needs before it can be submitted. */
const REQUIRED = [
  "legal_name",
  "country",
  "registration_number",
  "website",
  "industry",
  "regulated",
  "aml_program",
  "compliance_officer_name",
  "compliance_officer_email",
  "monitored_events",
  "monthly_volume",
  "regions_served",
  "go_live",
  "data_protection",
  "data_residency",
  "accept_terms",
  "accept_dpa",
];

function missing(answers: Record<string, unknown>): string[] {
  return REQUIRED.filter((k) => {
    const v = answers[k];
    return (
      v === undefined ||
      v === "" ||
      v === false ||
      (Array.isArray(v) && v.length === 0)
    );
  }).filter((k) => !(k === "regulated" && answers.regulated === false));
}

let eventSeq = 0;

/** A transaction event summary as GET /api/v1/events returns it. */
export function eventSummary(score: number, user = "usr_4812") {
  eventSeq += 1;
  const level =
    score >= 90
      ? "critical"
      : score >= 70
        ? "high"
        : score >= 50
          ? "elevated"
          : score >= 20
            ? "low"
            : "minimal";
  return {
    id: `evt_${String(eventSeq).padStart(4, "0")}`,
    event_key: `txn-${eventSeq}`,
    type: "transaction",
    occurred_at: new Date().toISOString(),
    received_at: new Date().toISOString(),
    risk_score: score,
    risk_level: level,
    decision: score >= 50 ? "review" : "allow",
    actor: { user_id: user, account_id: "acc_1", device_id: "dev_1" },
    country: "NG",
    transaction: {
      amount: 450000,
      currency: "NGN",
      direction: "debit",
      beneficiary_id: "ben_9",
      channel: "bank_transfer",
    },
  };
}

function hourAgo(h: number): string {
  return (
    new Date(Date.now() - h * 3600_000).toISOString().slice(0, 13) +
    ":00:00.000Z"
  );
}

const ANALYTICS = {
  window: "24h",
  bucket: "hour",
  since: hourAgo(24),
  series: [
    {
      at: hourAgo(1),
      events: 12,
      mean_score: 31,
      decisions: { allow: 9, step_up: 1, review: 2, block: 0 },
      bands: { minimal: 5, low: 4, elevated: 1, high: 2, critical: 0 },
    },
  ],
  top_users: [
    {
      user_id: "usr_4812",
      max_score: 81,
      events: 7,
      flagged: 2,
      last_seen: NOW,
    },
  ],
  top_signals: [
    { code: "new_device", label: "New device for this user", hits: 4 },
  ],
};

const PROFILE = {
  kind: "user",
  ref: "usr_4812",
  event_count: 7,
  first_seen: NOW,
  last_seen: NOW,
  max_score: 81,
  mean_score: 34,
  decisions: { allow: 5, step_up: 0, review: 2, block: 0 },
  linked: {
    users: [{ ref: "usr_4812", events: 7, last_seen: NOW }],
    accounts: [{ ref: "acc_1", events: 7, last_seen: NOW }],
    devices: [
      { ref: "dev_1", events: 5, last_seen: NOW },
      { ref: "dev_2", events: 2, last_seen: NOW },
    ],
    countries: [{ ref: "NG", events: 7, last_seen: NOW }],
    ips: [{ ref: "203.0.113.7", events: 7, last_seen: NOW }],
  },
};

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
    events: [],
    deny: [],
    productionEnabled: true,
    onboarding: null,
    permissions: ALL_PERMISSIONS,
    complianceProfile: null,
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
    tenant: {
      id: "ten_1",
      name: "Acme Payments",
      production_enabled: state.productionEnabled,
    },
    permissions: state.permissions,
    environments: state.productionEnabled ? ["test", "production"] : ["test"],
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

  const ruleView = (r: (typeof RULES)[number], detail: boolean) => ({
    ...r,
    applicable: applies(r, state.complianceProfile),
    ...(detail &&
    state.permissions.includes("compliance:engineering") &&
    ENGINEERING[r.id]
      ? { engineering_logic: ENGINEERING[r.id], definition: { id: r.id } }
      : {}),
  });

  const compliance = (
    route: Route,
    path: string,
    method: string,
    body: Record<string, unknown> | null,
  ) => {
    if (!state.permissions.includes("compliance:read")) {
      return error(route, 403, "FORBIDDEN", "Not allowed.");
    }
    const ruleId = path.match(/^\/compliance\/rules\/(.+)$/)?.[1];
    if (ruleId) {
      const r = RULES.find((x) => x.id === ruleId);
      return r
        ? json(route, ruleView(r, true))
        : error(route, 404, "NOT_FOUND", "No such rule.");
    }
    switch (path) {
      case "/compliance/pack":
        return json(route, PACK);
      case "/compliance/rules":
        return json(route, { data: RULES.map((r) => ruleView(r, false)) });
      case "/compliance/clocks":
        return json(route, { data: CLOCKS });
      case "/compliance/gaps":
        return json(route, { data: GAPS });
      case "/compliance/glossary":
        return json(route, { data: GLOSSARY });
      case "/compliance/profile": {
        const view = () => ({
          ...state.complianceProfile,
          pack_id: "ng",
          pack_version: PACK.version,
          updated_at: NOW,
          updated_by: "usr_admin",
        });
        if (method === "PUT") {
          if (!state.permissions.includes("compliance:write")) {
            return error(route, 403, "FORBIDDEN", "Not allowed.");
          }
          state.complianceProfile = {
            jurisdiction: String(body?.jurisdiction),
            licence_type: String(body?.licence_type),
            capabilities: (body?.capabilities as string[]) ?? [],
          };
          return json(route, view());
        }
        return state.complianceProfile
          ? json(route, view())
          : error(route, 404, "NOT_FOUND", "No compliance profile.");
      }
      default:
        state.problems.push(`unmocked ${method} ${path}`);
        return error(route, 404, "NOT_FOUND", "Not mocked.");
    }
  };

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

    if (path.startsWith("/compliance/")) {
      return compliance(route, path, method, body);
    }

    if (path.startsWith("/deny-list/") && method === "DELETE") {
      const id = path.slice("/deny-list/".length);
      state.deny = state.deny.filter((e) => e.id !== id);
      return route.fulfill({ status: 204 });
    }

    // Single sign-on: the "provider" sends the browser straight back with a code.
    if (path === "/auth/sso/start") {
      return json(route, {
        redirect_url: `${url.origin}/sso/callback?code=code-1&state=state-1`,
      });
    }
    if (path === "/auth/sso/callback") {
      if (body?.code !== "code-1" || body?.state !== "state-1") {
        state.problems.push(
          "SSO callback without the provider's code and state",
        );
      }
      state.signedIn = true;
      return json(route, { state: "active", mfa_methods: [] });
    }

    // Self-serve sign-up: the emailed code is 123456; Google sends the browser straight back.
    if (path === "/auth/signup/options") {
      return json(route, { email: true, google: true });
    }
    if (path === "/auth/signup") {
      if (
        !body?.name ||
        !body?.email ||
        !body?.business_name ||
        !body?.password
      ) {
        state.problems.push("sign-up without every field");
      }
      return json(
        route,
        {
          email: body?.email,
          resend_after_seconds: 60,
          expires_at: "2026-10-01T23:00:00Z",
        },
        202,
      );
    }
    if (path === "/auth/signup/resend") {
      return json(route, {
        email: "new@acme.example",
        resend_after_seconds: 60,
        expires_at: "2026-10-01T23:00:00Z",
      });
    }
    if (path === "/auth/signup/verify") {
      if (body?.code !== "123456") {
        return error(
          route,
          400,
          "CODE_INVALID",
          "That code is incorrect. Check the latest email and try again.",
        );
      }
      state.productionEnabled = false;
      state.onboarding = { status: "draft", answers: {} };
      return json(route, { state: "enrollment_required" });
    }
    if (path === "/auth/google/start") {
      return json(route, {
        redirect_url: `${url.origin}/signup/google?code=g-1&state=gs-1`,
      });
    }
    if (path === "/auth/google/callback") {
      if (body?.code !== "g-1" || body?.state !== "gs-1") {
        state.problems.push("Google callback without Google's code and state");
      }
      return json(route, {
        outcome: "needs_business",
        email: "grace@acme.example",
        name: "Grace Hopper",
      });
    }
    if (path === "/auth/signup/complete") {
      state.productionEnabled = false;
      state.onboarding = { status: "draft", answers: {} };
      return json(route, { state: "enrollment_required" });
    }

    // Compliance onboarding.
    if (path === "/onboarding" || path === "/onboarding/submit") {
      if (!state.onboarding)
        return error(route, 404, "NOT_FOUND", "Not found.");
      if (method === "PUT") state.onboarding.answers = body ?? {};
      if (path === "/onboarding/submit") {
        if (missing(state.onboarding.answers).length) {
          return error(
            route,
            400,
            "INVALID_REQUEST",
            "Answer everything first.",
          );
        }
        state.onboarding.status = "submitted";
      }
      return json(route, {
        status: state.onboarding.status,
        answers: state.onboarding.answers,
        missing: missing(state.onboarding.answers),
        production_enabled: state.productionEnabled,
        submitted_at: state.onboarding.status === "submitted" ? NOW : null,
        reviewed_at: null,
        review_note: null,
        updated_at: NOW,
      });
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
          auto_block: {
            mode: "off",
            shadow_since: null,
            shadow_days_required: 14,
            eligible_at: null,
            would_block_last_14_days: 0,
          },
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
          auto_block: { mode: "off" },
        });
      case "/alerts":
        return json(route, { data: [ALERT], next_cursor: null });
      case "/events": {
        const after = url.searchParams.get("after");
        const user = url.searchParams.get("user");
        let data = state.events;
        if (after) {
          const i = data.findIndex((e) => e.id === after);
          data = i === -1 ? data : data.slice(0, i);
        }
        if (user) data = data.filter((e) => e.actor.user_id === user);
        return json(route, { data, next_cursor: null });
      }
      case "/analytics":
        return json(route, ANALYTICS);
      case "/sso":
        return json(route, {
          redirect_uri: `${url.origin}/sso/callback`,
          connection: null,
        });
      case "/deny-list":
        if (method === "POST") {
          const entry = {
            id: `dny_${state.deny.length + 1}`,
            kind: body?.kind,
            value: body?.value,
            reason: body?.reason ?? "",
            expires_at: body?.expires_at ?? null,
            created_at: NOW,
            created_by: "usr_admin",
          };
          state.deny.unshift(entry);
          return json(route, entry, 201);
        }
        return json(route, {
          data: state.deny,
          total: state.deny.length,
          limit: 10000,
        });
      case "/entities/user/usr_4812":
        return json(route, PROFILE);
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
