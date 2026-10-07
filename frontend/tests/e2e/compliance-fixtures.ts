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
    kind: "legal",
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
    account_ref: null,
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
    jurisdiction: "NG",
    event_jurisdiction: null as string | null,
    authority: (kind === "report.STR" ? "NFIU" : null) as string | null,
    ...extra,
  };
}

/**
 * A slice of the Rwanda pack (backend/internal/rules/packs/rw), wording from its text.json, for
 * browsing another country. The full pack is checked against the library by the backend's tests.
 */
export const RW_PACK = {
  id: "rw",
  jurisdiction: "RW",
  name: "Rwanda",
  version: "2026.10.1",
  latest_version: "2026.10.1",
  checksum: "b".repeat(64),
  loaded_at: NOW,
  currency: "RWF",
  timezone: "Africa/Kigali",
  groups: [
    { id: "mandatory_controls", label: "Mandatory controls", rules: 1 },
    { id: "typology", label: "Typology and monitoring", rules: 1 },
  ],
  licence_types: [
    { id: "bank", label: "Bank" },
    { id: "emi", label: "E-money issuer" },
  ],
  capabilities: [
    { id: "bnr_regulated", label: "Regulated by the National Bank of Rwanda" },
  ],
  sources: [
    {
      id: "RS3",
      title: "Law No. 001/2025 (text)",
      links: [
        {
          title: "RegAlert text of Law 001/2025",
          url: "https://regalert.today/document/195f62e1-4ddf-4323-86ae-c8a19628da31/text",
        },
      ],
    },
    {
      id: "RS6",
      title: "FATF second-round mutual evaluation, July 2024",
      links: [],
    },
    { id: "RS23", title: "Digital fraud in Rwanda", links: [] },
  ],
  counts: { controls: 25, signals: 54, governance: 9 },
  penalties: [
    {
      provision:
        "Targeted financial sanctions administrative fine (FIC Reg 001/FIC/2025 Arts 26-27)",
      penalty: "RWF 1,000,000 to RWF 10,000,000",
      inference: false,
      sources: ["RS3"],
    },
  ],
};

const RW_REG_04 = {
  id: "RW-REG-04",
  name: "Suspicious transaction report (STR)",
  description:
    "A person forms a suspicion that funds are linked to crime. The report goes to the FIC, without telling the customer. The numeric deadline in Law 001/2025 Art 32 was not found, so the clock is a tenant setting with a design default.",
  action: "Report to the FIC; deadline is a placeholder",
  group: "mandatory_controls",
  kind: "control",
  status: "regulatory",
  regulators: ["FIC", "law"],
  applies_to: [],
  requires: [],
  sources: ["RS3", "RS6"],
  effective: {
    text: "Law No. 001/2025 of 22 January 2025 on money laundering, terrorism and proliferation financing; in force; reporting duty in Art 32, non-disclosure in Art 34 (per map)",
  },
  verification: "secondary_source_only",
  caveat:
    "The duty to report is in force; its deadline was not found in any readable source (C1). The 24-hour clock is a design placeholder, not a statutory number.",
  defaults: { enabled: true, mode: "live" },
  inputs: ["workflow"],
  events: ["suspicion_recorded"],
  outcome: { tasks: ["report.STR"] },
  parameters: [
    {
      name: "str_due",
      label:
        "Time to file after suspicion is first recorded (design placeholder)",
      type: "duration",
      default: 86400,
      min: 3600,
      max: 259200,
    },
  ],
  jurisdiction: "RW",
  provenance: "regulatory",
  lineage: { kind: "param", of: ["REG-14"] },
  basis:
    "[Regulatory]. Param of REG-14 (deadline placeholder). Default: On. Basis: RS3 (Art 32, Art 34 per map); RS6",
};

const RW_TM_05 = {
  id: "RW-TM-05",
  name: "New device, new payee, then transfer",
  description:
    "Within 30 minutes: a new device is linked, a new payee is added, and money is sent above the minimum.",
  points: "30 points, review",
  group: "typology",
  kind: "signal",
  status: "recommended",
  regulators: ["BNR"],
  applies_to: [],
  requires: [],
  sources: ["RS23"],
  effective: {
    text: "Not regulation: a recommended starting rule from the Ghana and Rwanda library of 4 October 2026",
  },
  verification: "secondary_source_only",
  defaults: { enabled: true, mode: "shadow" },
  inputs: ["events", "history"],
  events: ["transfer_outbound"],
  outcome: { severity: "high", tasks: [] },
  parameters: [
    {
      name: "floor",
      label: "Smallest amount that counts (suggested RWF 200,000)",
      type: "money",
      currency: "RWF",
      default: 200000,
      min: 0,
      max: 1000000000,
    },
  ],
  jurisdiction: "RW",
  provenance: "industry_practice",
  lineage: { kind: "param", of: ["ATO-01"] },
  template: "ATO-01",
  changes: ["floor"],
  basis:
    "[Industry practice]. Param of ATO-01. Default: On. Basis: RS23 (account takeover 24% of losses)",
};

export const RW_RULES = [RW_REG_04, RW_TM_05];

export const RW_CLOCKS = [
  {
    id: "str_filing",
    duration: "24 hours (design placeholder)",
    hours: 24,
    kind: "design_placeholder",
    authority: "FIC",
    duty: "File a suspicious transaction report; Law 001/2025 Art 32 sets the duty but no deadline was found",
    applies_to: "Reporting entities",
    requirement:
      "Timer from the earliest recorded suspicion, labelled a design default until the Gazette text is read",
    sources: ["RS3", "RS6"],
    rules: ["RW-REG-04"],
  },
  {
    id: "bnr_incident",
    duration: "Not found",
    hours: null,
    kind: "not_found",
    authority: "BNR",
    duty: "Report a cyber incident to the central bank (Regulation 50/2022 Art 21); the hour count was not found",
    applies_to: "BNR-regulated institutions",
    requirement:
      "Shown as not found until a person enters the supervisor's clock",
    sources: [],
    rules: [],
  },
];

export const RW_GAPS = [
  {
    id: "C1",
    kind: "conflict",
    title: "Rwanda STR numeric deadline",
    detail:
      "Not found in any readable source; 2024 MER states no statutory timeframe",
    treatment:
      'RW-REG-04 ships with a 24-hour design placeholder, labelled "threshold to be confirmed"',
    rules: ["RW-REG-04"],
    sources: ["RS3", "RS6"],
  },
];

export const RW_GLOSSARY = [
  {
    term: "NIDA",
    expansion: "National Identification Agency",
    definition:
      "The agency that issues Rwandan national IDs and verifies them.",
  },
];

export const CLOCK_MATRIX = [
  {
    item: "STR clock",
    nigeria:
      "Screen alerts within 72 hours; file within 24 hours of confirmed suspicion (REG-14); older summaries say 24 hours from the transaction",
    ghana:
      "24 hours from forming suspicion (Act 1044 s.38); BoG guideline says from confirmation (GH-REG-06)",
    rwanda: "Duty in Art 32; numeric deadline not found (RW-REG-04)",
    working_default:
      'Start a 24-hour clock at the earliest recorded suspicion in all three countries; label Rwanda "design default" until the Gazette text is read; internal SLA of 4 hours triage and 12 hours decision',
  },
];

export const JURISDICTIONS = [
  {
    code: "NG",
    name: "Nigeria",
    currency: "NGN",
    minor_unit: 2,
    timezone: "Africa/Lagos",
    regulators: ["CBN", "NFIU"],
    pack: "ng",
    activated: true,
  },
  {
    code: "GH",
    name: "Ghana",
    currency: "GHS",
    minor_unit: 2,
    timezone: "Africa/Accra",
    regulators: ["BoG", "FIC", "SEC", "NCA", "DPC"],
    pack: "gh",
    activated: false,
  },
  {
    code: "RW",
    name: "Rwanda",
    currency: "RWF",
    minor_unit: 0,
    timezone: "Africa/Kigali",
    regulators: ["FIC", "BNR", "CMA", "NCSA", "RURA"],
    pack: "rw",
    activated: false,
  },
];

export const ACTIVATIONS = [
  {
    id: "jac_ng",
    jurisdiction: "NG",
    booking_entity: "default",
    status: "live",
    fallback: true,
    pack: "ng@2026.10.0",
    licence_type: "bank",
    capabilities: [],
    updated_at: NOW,
  },
];

/** The rules running for the workspace's activations (GET /compliance/active-rules). */
export const ACTIVE_RULES = [
  {
    rule: "REG-01",
    name: "New device transfer cap",
    kind: "control",
    jurisdiction: "NG",
    pack: "ng@2026.10.0",
    booking_entity: "default",
    mode: "live",
    events: ["transfer_outbound"],
    facts: ["txn.amount"],
    refs: [],
    unset_settings: [],
  },
  {
    rule: "RW-REG-05",
    name: "Cash transaction report",
    kind: "control",
    jurisdiction: "RW",
    pack: "rw@2026.10.2",
    booking_entity: "rw-bank",
    mode: "shadow",
    events: ["cash_deposit"],
    facts: ["txn.amount"],
    refs: [],
    unset_settings: ["ctr_due"],
  },
];
