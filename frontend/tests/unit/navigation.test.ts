import { describe, expect, it } from "vitest";

import {
  isActiveSection,
  navSections,
  visibleSections,
} from "@/lib/navigation";

describe("navigation", () => {
  it("lists the built console sections in their groups", () => {
    expect(navSections.map((s) => s.label)).toEqual([
      "Overview",
      "Live monitor",
      "Alerts",
      "Cases",
      "Events",
      "Transactions",
      "Entities",
      "Analytics",
      "Rules",
      "Deny lists",
      "Risk settings",
      "Developer",
      "Rules library",
      "Work queue",
      "Team",
      "Single sign-on",
      "Audit log",
    ]);
  });

  it("shows a section only to users holding one of its permissions", () => {
    const analyst = new Set([
      "events:read",
      "rules:read",
      "alerts:read",
      "alerts:write",
      "cases:read",
      "cases:write",
    ]);
    const labels = visibleSections((p) => analyst.has(p)).map((s) => s.label);
    expect(labels).toEqual([
      "Overview",
      "Live monitor",
      "Alerts",
      "Cases",
      "Events",
      "Transactions",
      "Entities",
      "Analytics",
      "Rules",
      "Deny lists",
    ]);
    expect(visibleSections(() => false)).toEqual([]);
  });

  it("matches the root exactly and sections by prefix", () => {
    expect(isActiveSection("/", "/")).toBe(true);
    expect(isActiveSection("/", "/alerts")).toBe(false);
    expect(isActiveSection("/cases", "/cases/cas_1")).toBe(true);
    expect(isActiveSection("/cases", "/casesx")).toBe(false);
    expect(isActiveSection("/compliance", "/compliance/rules/REG-01")).toBe(
      true,
    );
    expect(isActiveSection("/compliance", "/compliance/work")).toBe(false);
    expect(isActiveSection("/compliance/work", "/compliance/work")).toBe(true);
  });
});
