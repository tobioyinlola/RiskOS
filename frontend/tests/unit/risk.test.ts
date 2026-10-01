import { describe, expect, it } from "vitest";

import {
  bandFor,
  bandInfo,
  bandRanges,
  BANDS,
  DEFAULT_THRESHOLDS,
} from "@/lib/risk";

describe("risk bands", () => {
  it("follows the design system's scale by default", () => {
    expect(DEFAULT_THRESHOLDS).toEqual({
      low: 20,
      elevated: 50,
      high: 70,
      critical: 90,
    });
    const cases: [number, string][] = [
      [0, "minimal"],
      [19, "minimal"],
      [20, "low"],
      [49, "low"],
      [50, "elevated"],
      [69, "elevated"],
      [70, "high"],
      [89, "high"],
      [90, "critical"],
      [100, "critical"],
    ];
    for (const [score, band] of cases) expect(bandFor(score)).toBe(band);
  });

  it("uses a tenant's calibration", () => {
    expect(bandFor(84, { low: 10, elevated: 30, high: 60, critical: 80 })).toBe(
      "critical",
    );
  });

  it("gives each band a glyph so it reads without color", () => {
    expect(BANDS.map((b) => b.shape)).toEqual([
      "circle",
      "circle",
      "triangle",
      "triangle",
      "octagon",
    ]);
    expect(bandInfo("elevated").label).toBe("Elevated");
  });

  it("splits 0-100 at the thresholds", () => {
    expect(bandRanges().map((r) => [r.from, r.to])).toEqual([
      [0, 19],
      [20, 49],
      [50, 69],
      [70, 89],
      [90, 100],
    ]);
  });
});
