import { describe, expect, it } from "vitest";

import {
  bandFor,
  bandInfo,
  BANDS,
  DEFAULT_THRESHOLDS,
  readout,
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

  it("encodes each band with a bar count", () => {
    expect(BANDS.map((b) => b.bars)).toEqual([1, 2, 3, 4, 5]);
    expect(bandInfo("elevated").label).toBe("Elevated");
  });

  it("reads out three digits with the leading zeros apart", () => {
    expect(readout(87)).toEqual({ lead: "0", value: "87" });
    expect(readout(5)).toEqual({ lead: "00", value: "5" });
    expect(readout(0)).toEqual({ lead: "00", value: "0" });
    expect(readout(100)).toEqual({ lead: "", value: "100" });
  });
});
