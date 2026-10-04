import type { Page, Route } from "@playwright/test";

import {
  applies,
  CLOCKS,
  ENGINEERING,
  GAPS,
  GLOSSARY,
  PACK,
  RULES,
  task,
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
  "compliance:configure",
  "compliance:sensitive",
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

export interface MockRuleVersion {
  version: number;
  mode: string;
  settings: Record<string, number>;
  reason: string;
  created_at: string;
  created_by: string;
  change_id: string | null;
  restored_from: number | null;
}

export interface MockRuleConfig {
  versions: MockRuleVersion[];
}

export interface MockRuleChange {
  id: string;
  env: string;
  rule: string;
  base_version: number;
  mode: string;
  settings: Record<string, number>;
  reason: string;
  restored_from: number | null;
  status: "pending" | "approved" | "rejected";
  proposed_by: string;
  proposed_at: string;
  decided_by: string | null;
  decided_at: string | null;
  decision_note: string | null;
  applied_version: number | null;
}

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
  /** Pack rule configuration by "<environment>/<rule>", and changes awaiting approval. */
  ruleConfigs: Record<string, MockRuleConfig>;
  ruleChanges: MockRuleChange[];
  /** Compliance tasks (ReturnType<typeof task>). */
  tasks: ReturnType<typeof task>[];
  /** Alerts past a deadline of the pack, and confirmed fraud not yet reported. */
  escalations: Record<string, unknown>[];
  unreportedFraud: { case_id: string; title: string; resolved_at: string }[];
  /** Exchange rates by currency. */
  fxRates: Record<string, number>;
  /**
   * The workspace's compliance profile: every workspace starts with the automatic one (licence
   * "other", unconfirmed, risk signals off) until a person saves it.
   */
  complianceProfile: {
    jurisdiction: string;
    licence_type: string;
    capabilities: string[];
    confirmed: boolean;
    signalsOff: boolean;
  };
  /** The workspace's own rules (POST /rules), newest last. */
  rules: Record<string, unknown>[];
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

/** The rule fields the builder offers (GET /risk/catalog), as the engine describes them. */
const CATALOG_FIELDS = [
  {
    name: "event.type",
    type: "string",
    operators: ["eq", "ne", "in", "not_in"],
    label: "Event type",
    group: "Event",
    events: [],
    values: [
      { value: "transaction", label: "Transaction" },
      { value: "login", label: "Login" },
      { value: "beneficiary_added", label: "Beneficiary added" },
    ],
  },
  {
    name: "transaction.amount",
    type: "number",
    operators: ["eq", "ne", "gt", "gte", "lt", "lte", "in", "not_in"],
    label: "Amount",
    group: "Transaction",
    unit: "money",
    events: ["transaction"],
  },
  {
    name: "transaction.channel",
    type: "string",
    operators: ["eq", "ne", "in", "not_in"],
    label: "Channel",
    group: "Transaction",
    events: ["transaction"],
    values: [
      { value: "card", label: "Card" },
      { value: "bank_transfer", label: "Bank transfer" },
      { value: "wallet", label: "Wallet" },
    ],
  },
  {
    name: "beneficiary.country",
    type: "string",
    operators: ["eq", "ne", "in", "not_in"],
    label: "Beneficiary's country",
    group: "Transaction",
    kind: "country",
    events: ["transaction", "beneficiary_added"],
  },
  {
    name: "feature.new_beneficiary",
    type: "boolean",
    operators: ["eq", "ne"],
    label: "New beneficiary for this customer",
    group: "Customer history",
    events: [],
  },
  {
    name: "feature.failed_logins_15m",
    type: "number",
    operators: ["eq", "ne", "gt", "gte", "lt", "lte", "in", "not_in"],
    label: "Failed logins in the last 15 minutes",
    group: "Recent activity",
    unit: "count",
    events: ["login"],
  },
];

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
      decisions: {
        allow: 9,
        step_up: 1,
        review: 2,
        block: 0,
        fee: 0,
        flag: 0,
        restrict: 0,
        hold: 0,
        block_session: 0,
        decline: 0,
      },
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
  decisions: {
    allow: 5,
    step_up: 0,
    review: 2,
    block: 0,
    fee: 0,
    flag: 0,
    restrict: 0,
    hold: 0,
    block_session: 0,
    decline: 0,
  },
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
    complianceProfile: {
      jurisdiction: "NG",
      licence_type: "other",
      capabilities: [],
      confirmed: false,
      signalsOff: true,
    },
    rules: [],
    ruleConfigs: {},
    ruleChanges: [],
    fxRates: {},
    tasks: [],
    escalations: [],
    unreportedFraud: [],
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

  type FixtureRule = (typeof RULES)[number];
  const defaultMode = (r: FixtureRule) =>
    !r.defaults.enabled ||
    (r.kind === "signal" && state.complianceProfile.signalsOff)
      ? "off"
      : r.defaults.mode;
  const currentVersion = (env: string, r: FixtureRule) =>
    state.ruleConfigs[`${env}/${r.id}`]?.versions.at(-1);
  const configView = (env: string, r: FixtureRule) => {
    const v = currentVersion(env, r);
    return {
      rule: r.id,
      version: v?.version ?? 0,
      mode: v?.mode ?? defaultMode(r),
      settings: v?.settings ?? {},
      reason: v?.reason ?? "",
      updated_at: v?.created_at ?? null,
      updated_by: v?.created_by ?? null,
      pending:
        state.ruleChanges.find(
          (c) => c.env === env && c.rule === r.id && c.status === "pending",
        ) ?? null,
    };
  };
  const ruleView = (r: FixtureRule, detail: boolean, env: string) => ({
    ...r,
    mode: configView(env, r).mode,
    config_version: configView(env, r).version,
    parameters: r.parameters.map((p) => ({
      ...p,
      value: configView(env, r).settings[p.name] ?? p.default,
    })),
    applicable: applies(r, state.complianceProfile),
    ...(detail &&
    state.permissions.includes("compliance:engineering") &&
    ENGINEERING[r.id]
      ? { engineering_logic: ENGINEERING[r.id], definition: { id: r.id } }
      : {}),
  });

  const applyVersion = (
    env: string,
    r: FixtureRule,
    v: Omit<MockRuleVersion, "version" | "created_at">,
  ) => {
    const key = `${env}/${r.id}`;
    const versions = state.ruleConfigs[key]?.versions ?? [];
    const next = { ...v, version: versions.length + 1, created_at: NOW };
    state.ruleConfigs[key] = { versions: [...versions, next] };
    return next;
  };

  const compliance = (
    route: Route,
    path: string,
    method: string,
    body: Record<string, unknown> | null,
    env: string,
  ) => {
    if (!state.permissions.includes("compliance:read")) {
      return error(route, 403, "FORBIDDEN", "Not allowed.");
    }
    const sub = path.match(
      /^\/compliance\/rules\/([A-Z]{3}-[0-9]{2})(?:\/(config|history|backtest|rollback))?$/,
    );
    if (sub) {
      const r = RULES.find((x) => x.id === sub[1]);
      if (!r) return error(route, 404, "NOT_FOUND", "No such rule.");
      switch (sub[2]) {
        case undefined:
          return json(route, ruleView(r, true, env));
        case "history":
          return json(route, {
            data: [
              ...(state.ruleConfigs[`${env}/${r.id}`]?.versions ?? []),
            ].reverse(),
          });
        case "backtest":
          return json(route, {
            rule: r.id,
            from: NOW,
            to: NOW,
            replayed: 120,
            truncated: false,
            current: {
              triggered: 120,
              fired: 2,
              clear: 110,
              not_evaluated: 8,
              missing: [
                {
                  item: "history:count transfer_outbound by account",
                  events: 8,
                },
              ],
              fired_samples: [],
            },
            candidate: {
              triggered: 120,
              fired: 5,
              clear: 107,
              not_evaluated: 8,
              missing: [
                {
                  item: "history:count transfer_outbound by account",
                  events: 8,
                },
              ],
              fired_samples: [],
            },
          });
        case "config":
          if (method === "GET") return json(route, configView(env, r));
          break;
        default:
          break;
      }
      if (!state.permissions.includes("compliance:configure")) {
        return error(route, 403, "FORBIDDEN", "Not allowed.");
      }
      const cur = configView(env, r);
      const target =
        sub[2] === "rollback"
          ? state.ruleConfigs[`${env}/${r.id}`]?.versions.find(
              (v) => v.version === body?.version,
            )
          : undefined;
      const mode = String(target?.mode ?? body?.mode);
      const settings = (target?.settings ?? body?.settings ?? {}) as Record<
        string,
        number
      >;
      const rank = (m: string) => ["off", "shadow", "live"].indexOf(m);
      const changed = JSON.stringify(settings) !== JSON.stringify(cur.settings);
      if (
        env === "production" &&
        r.kind === "control" &&
        (changed || rank(mode) < rank(cur.mode))
      ) {
        const change: MockRuleChange = {
          id: `pch_${state.ruleChanges.length + 1}`,
          env,
          rule: r.id,
          base_version: cur.version,
          mode,
          settings,
          reason: String(body?.reason),
          restored_from: target?.version ?? null,
          status: "pending",
          proposed_by: "usr_admin",
          proposed_at: NOW,
          decided_by: null,
          decided_at: null,
          decision_note: null,
          applied_version: null,
        };
        state.ruleChanges.push(change);
        return json(route, change, 202);
      }
      applyVersion(env, r, {
        mode,
        settings,
        reason: String(body?.reason),
        created_by: "usr_admin",
        change_id: null,
        restored_from: target?.version ?? null,
      });
      return json(route, configView(env, r));
    }
    const decision = path.match(
      /^\/compliance\/changes\/([^/]+)\/(approve|reject)$/,
    );
    if (decision) {
      const c = state.ruleChanges.find((x) => x.id === decision[1]);
      if (!c || c.status !== "pending")
        return error(route, 409, "CONFLICT", "Decided.");
      if (decision[2] === "approve") {
        if (c.proposed_by === "usr_admin") {
          return error(
            route,
            403,
            "FORBIDDEN",
            "A change needs another person's approval.",
          );
        }
        const r = RULES.find((x) => x.id === c.rule)!;
        const v = applyVersion(c.env, r, {
          mode: c.mode,
          settings: c.settings,
          reason: c.reason,
          created_by: c.proposed_by,
          change_id: c.id,
          restored_from: c.restored_from,
        });
        Object.assign(c, { status: "approved", applied_version: v.version });
      } else {
        c.status = "rejected";
      }
      Object.assign(c, { decided_by: "usr_admin", decided_at: NOW });
      return json(route, c);
    }
    const sensitive = state.permissions.includes("compliance:sensitive");
    const taskAction = path.match(
      /^\/compliance\/tasks\/([^/]+)(?:\/(approve|file|close|report|export))?$/,
    );
    if (taskAction) {
      const t = state.tasks.find((x) => x.id === taskAction[1]);
      if (!t || (t.sensitive && !sensitive)) {
        return error(route, 404, "NOT_FOUND", "No such task.");
      }
      switch (taskAction[2]) {
        case "approve":
          if (!sensitive) return error(route, 403, "FORBIDDEN", "Not allowed.");
          Object.assign(t, {
            status: "approved",
            approved_by: "usr_admin",
            approved_at: NOW,
          });
          break;
        case "file":
          Object.assign(t, {
            status: "filed",
            filing_ref: String(body?.filing_ref),
          });
          break;
        case "close":
          Object.assign(t, {
            status: String(body?.status),
            close_note: body?.note ?? null,
          });
          break;
        case "report":
          Object.assign(t, { report: { ...t.report, ...body } });
          break;
        default:
          break;
      }
      return json(route, t);
    }
    if (path === "/compliance/fraud-reports") {
      const c = state.unreportedFraud.find((x) => x.case_id === body?.case_id);
      if (!c)
        return error(route, 400, "INVALID_REQUEST", "Not confirmed fraud.");
      state.unreportedFraud = state.unreportedFraud.filter((x) => x !== c);
      const t = task(
        `ctk_${state.tasks.length + 1}`,
        "report.industry_fraud",
        "GOV-12",
        {
          case_id: c.case_id,
          due_at: null,
          report: {
            category: "fraud",
            case_title: c.title,
            narrative: "Credentials phished.",
          },
        },
      );
      state.tasks.push(t);
      return json(route, t, 201);
    }
    if (path === "/compliance/suspicion") {
      const t = task(`ctk_${state.tasks.length + 1}`, "report.STR", "REG-14", {
        case_id: body?.case_id,
        report: { narrative: body?.note ?? "" },
      });
      state.tasks.push(t);
      return json(route, t, 201);
    }
    const fx = path.match(/^\/compliance\/fx-rates\/([A-Z]{3})$/);
    if (fx) {
      const code = fx[1]!;
      if (method === "DELETE") {
        delete state.fxRates[code];
        return route.fulfill({ status: 204 });
      }
      state.fxRates[code] = Number(body?.usd_per_unit);
      return json(route, {
        currency: code,
        usd_per_unit: state.fxRates[code],
        updated_at: NOW,
        updated_by: "usr_admin",
      });
    }
    switch (path) {
      case "/compliance/changes": {
        const status = new URL(route.request().url()).searchParams.get(
          "status",
        );
        return json(route, {
          data: state.ruleChanges.filter(
            (c) => c.env === env && (!status || c.status === status),
          ),
        });
      }
      case "/compliance/tasks": {
        const status = new URL(route.request().url()).searchParams.get(
          "status",
        );
        return json(route, {
          data: state.tasks.filter(
            (t) =>
              (sensitive || !t.sensitive) && (!status || t.status === status),
          ),
          escalations: state.escalations,
          unreported_fraud: state.unreportedFraud,
        });
      }
      case "/compliance/restricted-signals":
        return json(route, { data: [] });
      case "/compliance/fx-rates":
        return json(route, {
          data: Object.entries(state.fxRates).map(([currency, usd]) => ({
            currency,
            usd_per_unit: usd,
            updated_at: NOW,
            updated_by: "usr_admin",
          })),
        });
      case "/compliance/pack":
        return json(route, PACK);
      case "/compliance/rules":
        return json(route, { data: RULES.map((r) => ruleView(r, false, env)) });
      case "/compliance/clocks":
        return json(route, { data: CLOCKS });
      case "/compliance/gaps":
        return json(route, { data: GAPS });
      case "/compliance/glossary":
        return json(route, { data: GLOSSARY });
      case "/compliance/profile": {
        const view = () => ({
          jurisdiction: state.complianceProfile.jurisdiction,
          licence_type: state.complianceProfile.licence_type,
          capabilities: state.complianceProfile.capabilities,
          confirmed: state.complianceProfile.confirmed,
          pack_id: "ng",
          pack_version: PACK.version,
          updated_at: NOW,
          updated_by: state.complianceProfile.confirmed
            ? "usr_admin"
            : "system:default",
        });
        if (method === "PUT") {
          if (!state.permissions.includes("compliance:write")) {
            return error(route, 403, "FORBIDDEN", "Not allowed.");
          }
          state.complianceProfile = {
            ...state.complianceProfile,
            jurisdiction: String(body?.jurisdiction),
            licence_type: String(body?.licence_type),
            capabilities: (body?.capabilities as string[]) ?? [],
            confirmed: true,
          };
          return json(route, view());
        }
        return json(route, view());
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
      return compliance(
        route,
        path,
        method,
        body,
        req.headers()["riskos-environment"] ?? "test",
      );
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
    const own = path.match(/^\/rules\/(rul_[0-9]+)(\/versions)?$/);
    if (own) {
      const rule = state.rules.find((r) => r.id === own[1]);
      if (!rule) return error(route, 404, "NOT_FOUND", "No such rule.");
      if (own[2])
        return json(route, {
          data: [{ ...rule, created_by: "usr_admin" }],
          current_version: rule.version,
        });
      if (method === "PATCH") {
        Object.assign(rule, body, { version: Number(rule.version) + 1 });
      }
      return json(route, rule);
    }
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
          fields: CATALOG_FIELDS,
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
        if (method === "POST") {
          const rule = {
            id: `rul_${state.rules.length + 1}`,
            ...body,
            version: 1,
            restored_from: null,
            created_at: NOW,
            updated_at: NOW,
            pending_change_id: null,
          };
          state.rules.push(rule);
          return json(route, rule, 201);
        }
        return json(route, { data: state.rules });
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
