/**
 * Risk bands (docs/design-system.md, Risk scale; RISK_ENGINE.md). The engine returns the band;
 * the console reads the environment's thresholds and decisions from GET /api/v1/risk/policy.
 */
export type RiskBand = "minimal" | "low" | "elevated" | "high" | "critical";

/** The glyph that lets a band read without color. */
export type BandShape = "circle" | "triangle" | "octagon";

export const BANDS: { band: RiskBand; label: string; shape: BandShape }[] = [
  { band: "minimal", label: "Minimal", shape: "circle" },
  { band: "low", label: "Low", shape: "circle" },
  { band: "elevated", label: "Elevated", shape: "triangle" },
  { band: "high", label: "High", shape: "triangle" },
  { band: "critical", label: "Critical", shape: "octagon" },
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

/** The score range of each band under thresholds t, in band order. */
export function bandRanges(t: Thresholds = DEFAULT_THRESHOLDS) {
  const edges = [0, t.low, t.elevated, t.high, t.critical, 101];
  return BANDS.map((b, i) => ({
    ...b,
    from: edges[i]!,
    to: edges[i + 1]! - 1,
  }));
}

/** Decisions are verbs (docs/design-system.md, Voice and content). */
export type Decision = "allow" | "review" | "step_up" | "block";

export const DECISION_LABEL: Record<Decision, string> = {
  allow: "Allow",
  review: "Review",
  step_up: "Step-up",
  block: "Block",
};

/** Text, badge and fill classes per band (tokens only). */
export const BAND_TEXT: Record<RiskBand, string> = {
  minimal: "text-risk-minimal",
  low: "text-risk-low",
  elevated: "text-risk-elevated",
  high: "text-risk-high",
  critical: "text-risk-critical",
};

export const BAND_BADGE: Record<RiskBand, string> = {
  minimal: "text-risk-minimal bg-risk-minimal-bg",
  low: "text-risk-low bg-risk-low-bg",
  elevated: "text-risk-elevated bg-risk-elevated-bg",
  high: "text-risk-high bg-risk-high-bg",
  critical: "text-risk-critical bg-risk-critical-bg",
};

export const BAND_FILL: Record<RiskBand, string> = {
  minimal: "bg-risk-minimal",
  low: "bg-risk-low",
  elevated: "bg-risk-elevated",
  high: "bg-risk-high",
  critical: "bg-risk-critical",
};
