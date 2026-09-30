/**
 * Risk bands (docs/design-system.md, Risk scale; RISK_ENGINE.md). The engine returns the band;
 * the console reads the environment's thresholds from GET /api/v1/risk/policy to draw rulers.
 */
export type RiskBand = "minimal" | "low" | "elevated" | "high" | "critical";

export const BANDS: {
  band: RiskBand;
  label: string;
  bars: 1 | 2 | 3 | 4 | 5;
}[] = [
  { band: "minimal", label: "Minimal", bars: 1 },
  { band: "low", label: "Low", bars: 2 },
  { band: "elevated", label: "Elevated", bars: 3 },
  { band: "high", label: "High", bars: 4 },
  { band: "critical", label: "Critical", bars: 5 },
];

export interface Thresholds {
  low: number;
  elevated: number;
  high: number;
  critical: number;
}

export const DEFAULT_THRESHOLDS: Thresholds = {
  low: 20,
  elevated: 50,
  high: 70,
  critical: 90,
};

export function bandFor(
  score: number,
  t: Thresholds = DEFAULT_THRESHOLDS,
): RiskBand {
  if (score >= t.critical) return "critical";
  if (score >= t.high) return "high";
  if (score >= t.elevated) return "elevated";
  if (score >= t.low) return "low";
  return "minimal";
}

export function bandInfo(band: RiskBand) {
  return BANDS.find((b) => b.band === band) ?? BANDS[0]!;
}

/** Readout: always 3 digits; render the leading zeros in ink-3. */
export function readout(score: number) {
  const s = String(Math.max(0, Math.min(999, Math.round(score)))).padStart(
    3,
    "0",
  );
  const lead = s.match(/^0+(?=\d)/)?.[0] ?? "";
  return { lead, value: s.slice(lead.length) };
}

/** Decisions are verbs (docs/design-system.md, Voice). */
export type Decision = "allow" | "review" | "step_up" | "block";

export const DECISION_LABEL: Record<Decision, string> = {
  allow: "Allow",
  review: "Review",
  step_up: "Step-up",
  block: "Block",
};
