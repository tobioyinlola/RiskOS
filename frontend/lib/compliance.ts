/**
 * Display of the compliance rules library (ADR-0006). Wording, thresholds and bounds all come
 * from the pack the API serves; these helpers only label and format them.
 */
import type { Schemas } from "@/lib/api/client";
import { currencyDigits, formatAmount, formatCount } from "@/lib/format";
import type { Tone } from "@/components/ui/pill";

type Rule = Schemas["ComplianceRule"];
type Parameter = Schemas["ComplianceParameter"];

export const KIND_LABEL: Record<Schemas["ComplianceRuleKind"], string> = {
  control: "Regulatory control",
  signal: "Risk-scoring rule",
  governance: "Governance",
};

export const VERIFICATION: Record<
  Schemas["ComplianceVerification"],
  { label: string; tone: Tone }
> = {
  confirmed: { label: "Confirmed", tone: "success" },
  secondary_source_only: { label: "Secondary source", tone: "neutral" },
  conflicting: { label: "Sources conflict", tone: "warning" },
  awaiting_regulator: { label: "Awaiting regulator", tone: "warning" },
};

/** How a rule starts for a workspace: off, live or in shadow. */
export function defaultLabel(d: Rule["defaults"]): string {
  if (!d.enabled) return "Off";
  return d.mode === "live" ? "Live" : "Shadow";
}

/** The headings of the library's plain-English columns, which differ by kind. */
export function wordingHeadings(kind: Rule["kind"]): {
  description: string;
  second?: string;
} {
  switch (kind) {
    case "control":
      return {
        description: "When it applies",
        second: "What the system does",
      };
    case "signal":
      return { description: "What we look for", second: "Points" };
    default:
      return { description: "What it requires" };
  }
}

const plural = (n: number, unit: string) =>
  `${formatCount(n)} ${unit}${n === 1 ? "" : "s"}`;

/** A duration in seconds in its largest whole unit: "10 minutes", "24 hours", "90 days". */
export function formatDuration(seconds: number): string {
  if (seconds > 0 && seconds % 86_400 === 0)
    return plural(seconds / 86_400, "day");
  if (seconds > 0 && seconds % 3_600 === 0)
    return plural(seconds / 3_600, "hour");
  if (seconds > 0 && seconds % 60 === 0) return plural(seconds / 60, "minute");
  return plural(seconds, "second");
}

/** A parameter value in its unit. */
export function formatParameterValue(p: Parameter, v: number): string {
  switch (p.type) {
    case "money":
      return p.currency
        ? formatAmount(v, p.currency)
        : `${formatCount(v)} (minor units)`;
    case "duration":
      return formatDuration(v);
    case "count":
      return formatCount(v);
    case "ratio":
      return `${v}×`;
    case "points":
      return plural(v, "point");
    case "percent":
      return `${v}%`;
    case "speed":
      return `${formatCount(v)} km/h`;
    case "hour":
      return Number.isInteger(v)
        ? `${String(v).padStart(2, "0")}:00 local time`
        : `${v} h`;
    case "score":
      return `${v} / 100`;
    case "flag":
      return v ? "On" : "Off";
  }
}

/** A parameter's default; null means the institution must set it. */
export function formatDefault(p: Parameter): string {
  return p.default === null
    ? "Set by your institution"
    : formatParameterValue(p, p.default);
}

/** The allowed range of a parameter. For regulatory limits the maximum is the legal one. */
export function formatRange(p: Parameter): string {
  if (p.type === "flag") return "On or off";
  return `${formatParameterValue(p, p.min)} to ${formatParameterValue(p, p.max)}`;
}

export type RuleFilter = {
  query: string;
  kind: Rule["kind"] | "";
  group: string;
  regulator: string;
  verification: Rule["verification"] | "";
  applicableOnly: boolean;
};

export const NO_FILTER: RuleFilter = {
  query: "",
  kind: "",
  group: "",
  regulator: "",
  verification: "",
  applicableOnly: false,
};

/** Whether a rule matches the library's filters (ID, name or description for the words). */
export function matchesFilter(r: Rule, f: RuleFilter): boolean {
  if (f.kind && r.kind !== f.kind) return false;
  if (f.group && r.group !== f.group) return false;
  if (f.regulator && !r.regulators.includes(f.regulator)) return false;
  if (f.verification && r.verification !== f.verification) return false;
  if (f.applicableOnly && r.applicable !== true) return false;
  const q = f.query.trim().toLowerCase();
  if (!q) return true;
  return `${r.id}\n${r.name}\n${r.description}`.toLowerCase().includes(q);
}

export type Mode = Schemas["PackRuleMode"];

export const MODE_LABEL: Record<Mode, string> = {
  live: "Live",
  shadow: "Shadow",
  off: "Off",
};

export const MODE_HELP: Record<Mode, string> = {
  live: "It acts: controls set the decision, signals add points.",
  shadow: "It is evaluated and recorded, and changes nothing.",
  off: "It is not evaluated.",
};

/** A unit a setting is typed in: the stored value is the typed number times factor. */
export type InputUnit = { id: string; label: string; factor: number };

/** The units a setting can be typed in: naira (not kobo), minutes to days (not seconds). */
export function inputUnits(p: Parameter): InputUnit[] {
  switch (p.type) {
    case "money": {
      const currency = p.currency ?? "NGN";
      return [
        {
          id: currency,
          label: currency,
          factor: 10 ** currencyDigits(currency),
        },
      ];
    }
    case "duration":
      return [
        { id: "minutes", label: "minutes", factor: 60 },
        { id: "hours", label: "hours", factor: 3_600 },
        { id: "days", label: "days", factor: 86_400 },
      ];
    case "percent":
      return [{ id: "percent", label: "%", factor: 1 }];
    case "points":
      return [{ id: "points", label: "points", factor: 1 }];
    default:
      return [{ id: "value", label: "", factor: 1 }];
  }
}

/** The largest unit a value is a whole number of (for durations), else the first. */
export function bestUnit(p: Parameter, value: number | null): InputUnit {
  const units = inputUnits(p);
  if (value === null) return units[0]!;
  return [...units].reverse().find((u) => value % u.factor === 0) ?? units[0]!;
}

/** A stored value as typed in unit. */
export function toInput(value: number | null, unit: InputUnit): string {
  if (value === null) return "";
  return String(Math.round((value / unit.factor) * 1e6) / 1e6);
}

/** A typed number back to the stored value; null when it is not a number. */
export function fromInput(
  p: Parameter,
  text: string,
  unit: InputUnit,
): number | null {
  if (text.trim() === "") return null;
  const n = Number(text);
  if (!Number.isFinite(n)) return null;
  const v = n * unit.factor;
  const whole = ["money", "count", "duration", "hour", "points", "flag"];
  return whole.includes(p.type) ? Math.round(v) : Math.round(v * 1e6) / 1e6;
}

/** What each kind of compliance task is, in plain words. */
export const TASK_LABEL: Record<string, string> = {
  "report.STR": "Suspicious transaction report",
  "report.CTR": "Currency transaction report",
  "report.FTR": "Foreign transaction report",
  "report.PEP_monthly": "Monthly PEP transaction report",
  "report.sanctions": "Sanctions match report",
  "report.industry_fraud": "Industry fraud report",
  "case.open": "Open a case",
  edd: "Enhanced due diligence",
  source_of_funds: "Source of funds",
  source_of_wealth: "Source of wealth",
  senior_approval: "Senior management approval",
  freeze_account: "Freeze the account",
  close_account: "Close the account",
};

export const isReport = (kind: string) => kind.startsWith("report.");

/** "due in 3h 20m" or "overdue by 2d 4h", against now. */
export function formatDue(dueAt: string, now: Date = new Date()): string {
  const ms = new Date(dueAt).getTime() - now.getTime();
  const minutes = Math.floor(Math.abs(ms) / 60_000);
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const mins = minutes % 60;
  const span = days
    ? `${days}d ${hours}h`
    : hours
      ? `${hours}h ${mins}m`
      : `${mins}m`;
  return ms >= 0 ? `due in ${span}` : `overdue by ${span}`;
}

/** Where a country library's rule comes from (ADR-0007), with what that means for its numbers. */
export const PROVENANCE: Record<
  NonNullable<Rule["provenance"]>,
  { label: string; tone: Tone; meaning: string }
> = {
  regulatory: {
    label: "Regulatory",
    tone: "danger",
    meaning: "Follows a cited law, regulation or directive.",
  },
  industry_practice: {
    label: "Industry practice",
    tone: "info",
    meaning:
      "A published typology or practice. No law sets its numbers: they are starting values to tune.",
  },
  my_design: {
    label: "Library design",
    tone: "neutral",
    meaning:
      "The library author's proposal. Starting values, not regulation: calibrate them in monitoring first.",
  },
};

/** Why a rule ships off. */
export const OFF_REASON: Record<NonNullable<Rule["off_reason"]>, string> = {
  threshold_to_be_confirmed: "Off until a threshold is confirmed",
  verify_first: "Off until the legal basis is verified",
};

/** What a clock's duration rests on. */
export const CLOCK_KIND: Record<
  Schemas["ComplianceClockKind"],
  { label: string; tone: Tone }
> = {
  legal: { label: "Legal clock", tone: "danger" },
  internal_policy: { label: "Internal policy", tone: "info" },
  design_placeholder: { label: "Design placeholder", tone: "warning" },
  not_found: { label: "Not found", tone: "warning" },
};

const LINEAGE: Record<Schemas["ComplianceLineage"]["kind"], string> = {
  param: "A country version of",
  analogue: "An analogue of",
  cross_reference: "Cross-references",
  stricter: "Stricter than",
  new_local: "New for this country",
};

/**
 * How a rule relates to the Nigeria library, in words: inherited unchanged, a country version
 * (with the settings it changes), or new.
 */
export function lineageText(r: Rule): string {
  const changes = r.changes?.length
    ? `, with its own ${r.changes.join(", ")}`
    : "";
  if (r.inherited_from)
    return `Inherited from the Nigeria library's ${r.id}${changes}.`;
  if (!r.lineage) return "";
  const of = r.lineage.of.join(" and ");
  if (r.lineage.kind === "new_local")
    return of
      ? `New for this country (related: ${of}).`
      : "New for this country.";
  return `${LINEAGE[r.lineage.kind]} ${of ? `Nigeria's ${of}` : "a Nigerian practice"}${changes}.`;
}

/** A country's flag as emoji, from its ISO 3166-1 alpha-2 code. */
export function flag(code: string): string {
  return [...code.toUpperCase()]
    .map((c) => String.fromCodePoint(0x1f1e6 + c.charCodeAt(0) - 65))
    .join("");
}
