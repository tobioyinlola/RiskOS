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
  group: "Monitor" | "Configure" | "Organization";
  icon:
    | "gauge"
    | "activity"
    | "alert"
    | "folder"
    | "shield"
    | "sliders"
    | "code"
    | "users"
    | "scroll";
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
    href: "/events",
    label: "Events",
    group: "Monitor",
    icon: "activity",
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
    href: "/rules",
    label: "Rules",
    group: "Configure",
    icon: "shield",
    anyOf: ["rules:read"],
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
