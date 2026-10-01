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
  group: "Monitor" | "Investigate" | "Configure" | "Organization";
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
    | "key";
  anyOf: readonly Scope[];
}

export const navSections = [
  {
    href: "/",
    label: "Overview",
    group: "Monitor",
    icon: "gauge",
    anyOf: ["events:read", "alerts:read"],
  },
  {
    href: "/live",
    label: "Live monitor",
    group: "Monitor",
    icon: "radio",
    anyOf: ["events:read"],
  },
  {
    href: "/alerts",
    label: "Alerts",
    group: "Monitor",
    icon: "alert",
    anyOf: ["alerts:read"],
  },
  {
    href: "/cases",
    label: "Cases",
    group: "Monitor",
    icon: "folder",
    anyOf: ["cases:read"],
  },
  {
    href: "/events",
    label: "Events",
    group: "Investigate",
    icon: "activity",
    anyOf: ["events:read"],
  },
  {
    href: "/transactions",
    label: "Transactions",
    group: "Investigate",
    icon: "receipt",
    anyOf: ["events:read"],
  },
  {
    href: "/entities",
    label: "Entities",
    group: "Investigate",
    icon: "fingerprint",
    anyOf: ["events:read"],
  },
  {
    href: "/analytics",
    label: "Analytics",
    group: "Investigate",
    icon: "chart",
    anyOf: ["events:read"],
  },
  {
    href: "/rules",
    label: "Rules",
    group: "Configure",
    icon: "shield",
    anyOf: ["rules:read"],
  },
  {
    href: "/deny-lists",
    label: "Deny lists",
    group: "Configure",
    icon: "ban",
    anyOf: ["rules:read", "risk:configure"],
  },
  {
    href: "/settings",
    label: "Risk settings",
    group: "Configure",
    icon: "sliders",
    anyOf: ["risk:configure"],
  },
  {
    href: "/developer",
    label: "Developer",
    group: "Configure",
    icon: "code",
    anyOf: ["api_keys:read", "webhooks:read"],
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
  return pathname === href || pathname.startsWith(`${href}/`);
}
