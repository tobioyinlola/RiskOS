/**
 * A slice of the Nigeria rule pack for the browser tests, copied from the backend seed
 * (backend/internal/rules/packs/ng) and the library's wording; REG-13's events are abridged. The API's own behaviour, and the
 * full pack, are covered by the backend's integration tests.
 */

const NOW = "2026-09-30T12:00:00Z";

export const PACK = {
  id: "ng",
  jurisdiction: "NG",
  name: "Nigeria",
  version: "2026.10.0",
  latest_version: "2026.10.0",
  checksum: "a".repeat(64),
  loaded_at: NOW,
  groups: [
    { id: "mandatory_controls", label: "Mandatory controls", rules: 2 },
    { id: "velocity", label: "Velocity and structuring", rules: 1 },
  ],
  licence_types: [
    { id: "bank", label: "Commercial bank" },
    { id: "mfb", label: "Microfinance bank" },
    { id: "psp", label: "Payment service provider" },
  ],
  capabilities: [
    { id: "instant_payments", label: "Offers instant payments" },
    { id: "cash_fee", label: "Applies the weekly cash withdrawal charge" },
  ],
  sources: [
    {
      id: "S2",
      title: "CBN instant payment guidelines",
      links: [
        {
          title: "Afriwise",
          url: "https://www.afriwise.com/blog/device-binding-and-transaction-caps-the-cbns-instant-payment-guidelines-take-effect",
        },
      ],
    },
    { id: "S7", title: "NIBSS fraud landscape reports", links: [] },
  ],
  counts: { controls: 19, signals: 52, governance: 12 },
};

const REG_01 = {
  id: "REG-01",
  name: "New-device limit",
  description:
    "A customer sends more than ₦20,000 within 24 hours of linking a new phone or app, or within 24 hours of opening the account",
  action: "Declines the transfer",
  group: "mandatory_controls",
  kind: "control",
  status: "regulatory",
  regulators: ["CBN"],
  applies_to: ["bank", "mfb", "psp"],
  requires: ["instant_payments"],
  sources: ["S2"],
  effective: {
    date: "2026-07-01",
    text: "In force 1 July 2026 (CBN instant payment guidelines of 12 March 2026)",
  },
  verification: "secondary_source_only",
  caveat:
    "Read from law-firm and press summaries of the CBN circular, not the circular itself: the CBN website blocked downloads. Confirm the amounts, scope and effective date against the original before relying on them.",
  defaults: { enabled: true, mode: "live" },
  inputs: ["events", "device_binding", "customer_profile"],
  events: ["transfer_outbound"],
  outcome: { decision: "decline", tasks: [] },
  parameters: [
    {
      name: "new_device_cap",
      label:
        "Most a customer can send within 24 hours of a new device or a new account",
      type: "money",
      currency: "NGN",
      default: 2000000,
      min: 0,
      max: 2000000,
    },
    {
      name: "new_device_window",
      label: "How long a new device or account counts as new",
      type: "duration",
      default: 86400,
      min: 86400,
      max: 604800,
    },
  ],
};

const REG_13 = {
  id: "REG-13",
  name: "Person of interest",
  description:
    "The customer or the other party appears on the NIBSS Person of Interest list",
  action: "Sends for review and opens a case",
  group: "mandatory_controls",
  kind: "control",
  status: "regulatory",
  regulators: ["NIBSS", "CBN"],
  applies_to: [],
  requires: [],
  sources: ["S7"],
  effective: {
    text: "NIBSS and CBN Person of Interest portal (list size reported as of January 2026)",
  },
  verification: "conflicting",
  caveat:
    "One report says the Person of Interest list has 3,417 entries and another says 13,417. The count does not change the rule; check it against the portal.",
  defaults: { enabled: true, mode: "live" },
  inputs: ["events", "poi_list"],
  events: ["transfer_outbound", "transfer_inbound", "customer_onboarded"],
  outcome: { decision: "review", tasks: ["case.open"] },
  parameters: [],
};

const VEL_01 = {
  id: "VEL-01",
  name: "Burst of transfers",
  description: "Five or more transfers out in 10 minutes",
  points: "15",
  group: "velocity",
  kind: "signal",
  status: "recommended",
  regulators: ["NIBSS"],
  applies_to: [],
  requires: [],
  sources: ["S7"],
  effective: {
    text: "Not regulation: a recommended starting rule from the library of 2 October 2026",
  },
  verification: "secondary_source_only",
  defaults: { enabled: true, mode: "shadow" },
  inputs: ["events", "history"],
  events: ["transfer_outbound"],
  outcome: { severity: "medium", tasks: [] },
  parameters: [
    {
      name: "weight",
      label: "Points added to the risk score",
      type: "points",
      default: 15,
      min: 0,
      max: 50,
    },
    {
      name: "transfers",
      label: "Transfers out that count as a burst",
      type: "count",
      default: 5,
      min: 2,
      max: 100,
    },
    {
      name: "window",
      label: "Time window",
      type: "duration",
      default: 600,
      min: 60,
      max: 3600,
    },
  ],
};

export const RULES = [REG_01, REG_13, VEL_01];

/** The section 7B block, returned only to callers with compliance:engineering. */
export const ENGINEERING: Record<string, string> = {
  "REG-01": `RULE REG-01 | control=hard | action=decline
WHEN  txn.type = transfer.outbound
SCOPE tenant.type IN (bank, mfb, psp) AND tenant.instant_payments = true
IF    txn.amount > tenant.cfg.new_device_cap            # default NGN 20_000
AND   ANY OF ( device.bound_age < 24h , account.age < 24h )
THEN  decision=decline  reason=new_device_or_account_cap`,
};

export const CLOCKS = [
  {
    id: "cbn_cyber_incident",
    duration: "24 hours",
    hours: 24,
    duty: "Report cyber incidents to the CBN",
    applies_to:
      "Banks and payment service banks under the CBN 2024 risk-based cybersecurity framework (S5)",
    requirement:
      "Incident record with a countdown from detection and an export for the CBN",
    sources: ["S5"],
    rules: [],
  },
];

export const GAPS = [
  {
    id: "gap-01",
    title: "CBN circulars not read directly",
    detail:
      "The CBN site returned access errors, so REG-01 to REG-06 rest on law-firm and press summaries. Confirm amounts, scope and effective dates in the originals.",
    rules: ["REG-01"],
    sources: ["S2"],
  },
];

export const GLOSSARY = [
  {
    term: "BVN",
    expansion: "Bank Verification Number",
    definition:
      "An 11-digit number that identifies one person across every Nigerian bank, linked to their fingerprints and photo.",
  },
];

/** Whether a rule applies to a profile, as the API decides it. */
export function applies(
  rule: { applies_to: string[]; requires: string[] },
  profile: { licence_type: string; capabilities: string[] } | null,
): boolean | null {
  if (!profile) return null;
  return (
    (rule.applies_to.length === 0 ||
      rule.applies_to.includes(profile.licence_type)) &&
    rule.requires.every((c) => profile.capabilities.includes(c))
  );
}

/** A compliance task as GET /api/v1/compliance/tasks returns it. */
export function task(
  id: string,
  kind: string,
  rule: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id,
    kind,
    rule,
    status: "open",
    sensitive: kind === "report.STR",
    due_at: new Date(Date.now() + 5 * 3_600_000).toISOString(),
    overdue: false,
    subject_ref: "usr_4812",
    evaluation_id: null,
    event_id: null,
    case_id: null,
    report: {},
    created_at: NOW,
    created_by: "system:compliance",
    updated_at: NOW,
    approved_by: null,
    approved_at: null,
    closed_by: null,
    closed_at: null,
    close_note: null,
    filing_ref: null,
    ...extra,
  };
}
