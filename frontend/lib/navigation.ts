import type { Route } from "next";

/**
 * A top-level section of the RiskOS console. The list mirrors the navigation defined in the
 * product specification (docs/spec, §10) and is the single source for the sidebar, page titles
 * and page metadata.
 */
export interface NavSection {
  href: Route;
  label: string;
  /** What the section will provide, taken from the specification. */
  description: string;
}

export const navSections = [
  {
    href: "/",
    label: "Overview",
    description:
      "Event, evaluation and transaction volumes, open alerts and cases, blocked activity, risk and decision distributions, top signals and recent risk activity.",
  },
  {
    href: "/risk-monitor",
    label: "Risk Monitor",
    description:
      "Live event stream with throughput, high-risk and critical counts, processing latency and filters by risk level, decision, event type, entity, location and time.",
  },
  {
    href: "/transactions",
    label: "Transactions",
    description:
      "Transactions with amount, currency, sender, recipient, beneficiary, risk and history.",
  },
  {
    href: "/events",
    label: "Events",
    description:
      "Events with raw and normalized payloads, processing timeline, risk evaluation and related activity.",
  },
  {
    href: "/users",
    label: "Users",
    description:
      "End-customer profiles with accounts, devices, transactions, logins, alerts, cases and risk history.",
  },
  {
    href: "/accounts",
    label: "Accounts",
    description:
      "Accounts and wallets with transactions, beneficiaries, devices, login history and risk history.",
  },
  {
    href: "/devices",
    label: "Devices",
    description:
      "Devices with associated users, accounts, IPs, countries and sessions, and unusual relationships.",
  },
  {
    href: "/alerts",
    label: "Alerts",
    description:
      "Alert queue with severity, entities, score, analyst and status; triage actions from assignment to resolution.",
  },
  {
    href: "/cases",
    label: "Cases",
    description:
      "Investigation workspaces with alerts, entities, evidence, notes, assignment and resolution.",
  },
  {
    href: "/rules",
    label: "Rules",
    description:
      "Structured, versioned risk rules with priority, severity, testing, audit trail and rollback.",
  },
  {
    href: "/analytics",
    label: "Analytics",
    description: "Risk trends and strategy performance over time.",
  },
  {
    href: "/developer",
    label: "Developer",
    description:
      "API keys, event explorer, webhooks, API documentation and request debugging.",
  },
  {
    href: "/team",
    label: "Team",
    description: "Team members, roles and access.",
  },
  {
    href: "/audit-logs",
    label: "Audit Logs",
    description:
      "The audit trail of material administrative and security actions.",
  },
  {
    href: "/settings",
    label: "Settings",
    description: "Organization, environment and configuration settings.",
  },
] as const satisfies readonly NavSection[];

export type SectionHref = (typeof navSections)[number]["href"];

export function getSection(href: SectionHref): NavSection {
  const section = navSections.find((s) => s.href === href);
  if (!section) {
    throw new Error(`unknown section ${href}`);
  }
  return section;
}

/** Whether `pathname` is inside the section at `href` (the root only matches exactly). */
export function isActiveSection(href: string, pathname: string): boolean {
  if (href === "/") {
    return pathname === "/";
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}
