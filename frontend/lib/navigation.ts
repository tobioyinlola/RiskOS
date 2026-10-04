import type { Route } from "next";

import type { Schemas } from "@/lib/api/client";

type Scope = Schemas["Scope"];

/**
 * A console section. The list is the single source for the navigation. A section is shown only
 * to users holding one of its permissions (the API enforces them regardless).
 */
export interface NavSection {
  href: Route;
  label: string;
  /** "Main" items are listed first without a heading; the others under their label. */
  group: "Main" | "Workflows" | "Compliance" | "Organization";
  icon:
    | "gauge"
    | "radio"
    | "receipt"
    | "fingerprint"
    | "chart"
    | "ban"
    | "activity"
    | "alert"
    | "folder"
    | "shield"
    | "sliders"
    | "code"
    | "users"
    | "scroll"
    | "scales"
    | "key";
  anyOf: readonly Scope[];
  /** Other paths that belong to the section (a preset rule's page belongs to Rules). */
  also?: readonly string[];
}

export const navSections = [
  {
    href: "/",
    label: "Overview",
    group: "Main",
    icon: "gauge",
    anyOf: ["events:read", "alerts:read"],
  },
  {
    href: "/live",
    label: "Live monitor",
    group: "Main",
    icon: "radio",
    anyOf: ["events:read"],
  },
  {
    href: "/alerts",
    label: "Alerts",
    group: "Main",
    icon: "alert",
    anyOf: ["alerts:read"],
  },
  {
    href: "/cases",
    label: "Cases",
    group: "Main",
    icon: "folder",
    anyOf: ["cases:read"],
  },
  {
    href: "/events",
    label: "Events",
    group: "Main",
    icon: "activity",
    anyOf: ["events:read"],
  },
  {
    href: "/transactions",
    label: "Transactions",
    group: "Main",
    icon: "receipt",
    anyOf: ["events:read"],
  },
  {
    href: "/entities",
    label: "Entities",
    group: "Main",
    icon: "fingerprint",
    anyOf: ["events:read"],
  },
  {
    href: "/analytics",
    label: "Analytics",
    group: "Main",
    icon: "chart",
    anyOf: ["events:read"],
  },
  {
    href: "/rules",
    label: "Rules",
    group: "Workflows",
    icon: "shield",
    anyOf: ["rules:read", "compliance:read"],
    also: ["/compliance/rules"],
  },
  {
    href: "/deny-lists",
    label: "Deny lists",
    group: "Workflows",
    icon: "ban",
    anyOf: ["rules:read", "risk:configure"],
  },
  {
    href: "/settings",
    label: "Risk settings",
    group: "Workflows",
    icon: "sliders",
    anyOf: ["risk:configure"],
  },
  {
    href: "/developer",
    label: "Developer",
    group: "Workflows",
    icon: "code",
    anyOf: ["api_keys:read", "webhooks:read"],
  },
  {
    href: "/compliance/work",
    label: "Work queue",
    group: "Compliance",
    icon: "folder",
    anyOf: ["compliance:read"],
  },
  {
    href: "/team",
    label: "Team",
    group: "Organization",
    icon: "users",
    anyOf: ["team:read"],
  },
  {
    href: "/sso",
    label: "Single sign-on",
    group: "Organization",
    icon: "key",
    anyOf: ["team:read"],
  },
  {
    href: "/audit-logs",
    label: "Audit log",
    group: "Organization",
    icon: "scroll",
    anyOf: ["audit_logs:read"],
  },
] as const satisfies readonly NavSection[];

export type SectionHref = (typeof navSections)[number]["href"];

export function visibleSections(
  can: (p: Scope) => boolean,
): readonly NavSection[] {
  return navSections.filter((s) => s.anyOf.some((p) => can(p)));
}

/** Whether `pathname` is inside the section at `href` (the root only matches exactly). */
export function isActiveSection(href: string, pathname: string): boolean {
  if (href === "/") {
    return pathname === "/";
  }
  // The most specific section wins: /compliance/work is not inside /compliance.
  const deeper = navSections.some(
    (s) =>
      s.href !== href &&
      s.href.startsWith(`${href}/`) &&
      (pathname === s.href || pathname.startsWith(`${s.href}/`)),
  );
  const inside = (p: string) => pathname === p || pathname.startsWith(`${p}/`);
  const also =
    (navSections as readonly NavSection[]).find((s) => s.href === href)?.also ??
    [];
  return (!deeper && inside(href)) || also.some(inside);
}
