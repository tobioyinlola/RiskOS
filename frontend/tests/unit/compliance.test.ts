import { describe, expect, it } from "vitest";

import type { Schemas } from "@/lib/api/client";
import {
  defaultLabel,
  formatDefault,
  formatDuration,
  formatRange,
  matchesFilter,
  NO_FILTER,
} from "@/lib/compliance";

type Parameter = Schemas["ComplianceParameter"];
type Rule = Schemas["ComplianceRule"];

const param = (p: Partial<Parameter>): Parameter => ({
  name: "p",
  label: "P",
  type: "count",
  default: 1,
  min: 0,
  max: 10,
  ...p,
});

describe("compliance display", () => {
  it("formats durations in their largest whole unit", () => {
    expect(formatDuration(600)).toBe("10 minutes");
    expect(formatDuration(86_400)).toBe("1 day");
    expect(formatDuration(7_200)).toBe("2 hours");
    expect(formatDuration(45)).toBe("45 seconds");
  });

  it("formats parameter values in their units", () => {
    expect(
      formatDefault(
        param({ type: "money", currency: "NGN", default: 2_000_000 }),
      ),
    ).toBe("20,000.00 NGN");
    expect(formatDefault(param({ type: "points", default: 15 }))).toBe(
      "15 points",
    );
    expect(formatDefault(param({ type: "hour", default: 22 }))).toBe(
      "22:00 Lagos time",
    );
    expect(formatDefault(param({ type: "flag", default: 0 }))).toBe("Off");
    expect(
      formatRange(param({ type: "duration", min: 86_400, max: 604_800 })),
    ).toBe("1 day to 7 days");
  });

  it("says when the institution must set a value, never inventing one", () => {
    expect(
      formatDefault(param({ type: "money", currency: "NGN", default: null })),
    ).toBe("Set by your institution");
  });

  it("labels defaults", () => {
    expect(defaultLabel({ enabled: false, mode: "shadow" })).toBe("Off");
    expect(defaultLabel({ enabled: true, mode: "live" })).toBe("Live");
    expect(defaultLabel({ enabled: true, mode: "shadow" })).toBe("Shadow");
  });

  it("filters by words, kind, regulator, verification and applicability", () => {
    const rule = {
      id: "REG-13",
      name: "Person of interest",
      description: "The customer appears on the NIBSS list",
      kind: "control",
      group: "mandatory_controls",
      regulators: ["NIBSS", "CBN"],
      verification: "conflicting",
      applicable: false,
    } as Rule;
    expect(matchesFilter(rule, NO_FILTER)).toBe(true);
    expect(matchesFilter(rule, { ...NO_FILTER, query: "reg-13" })).toBe(true);
    expect(matchesFilter(rule, { ...NO_FILTER, query: "nibss list" })).toBe(
      true,
    );
    expect(matchesFilter(rule, { ...NO_FILTER, query: "crypto" })).toBe(false);
    expect(matchesFilter(rule, { ...NO_FILTER, kind: "signal" })).toBe(false);
    expect(matchesFilter(rule, { ...NO_FILTER, regulator: "CBN" })).toBe(true);
    expect(
      matchesFilter(rule, { ...NO_FILTER, verification: "confirmed" }),
    ).toBe(false);
    expect(matchesFilter(rule, { ...NO_FILTER, applicableOnly: true })).toBe(
      false,
    );
  });
});
