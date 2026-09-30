import { describe, expect, it } from "vitest";

import { getSection, isActiveSection, navSections } from "@/lib/navigation";

describe("navigation", () => {
  it("matches the specification's navigation, in order (spec §10)", () => {
    expect(navSections.map((s) => s.label)).toEqual([
      "Overview",
      "Risk Monitor",
      "Transactions",
      "Events",
      "Users",
      "Accounts",
      "Devices",
      "Alerts",
      "Cases",
      "Rules",
      "Analytics",
      "Developer",
      "Team",
      "Audit Logs",
      "Settings",
    ]);
  });

  it("has unique hrefs and a description for every section", () => {
    const hrefs = navSections.map((s) => s.href);
    expect(new Set(hrefs).size).toBe(hrefs.length);
    for (const s of navSections) {
      expect(s.description.length).toBeGreaterThan(10);
    }
  });

  it("looks up sections by href", () => {
    expect(getSection("/alerts").label).toBe("Alerts");
  });

  it.each([
    ["/", "/", true],
    ["/", "/alerts", false],
    ["/alerts", "/alerts", true],
    ["/alerts", "/alerts/alt_123", true],
    ["/alerts", "/alerts-archive", false],
    ["/cases", "/alerts", false],
  ])("isActiveSection(%s, %s) = %s", (href, pathname, want) => {
    expect(isActiveSection(href, pathname)).toBe(want);
  });
});
