import type { Schemas } from "@/lib/api/client";

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

/**
 * Decisions are verbs (docs/design-system.md, Voice and content). Besides allow, step-up,
 * review and block, regulatory controls set the action the law requires (ADR-0006).
 */
export type Decision = Schemas["Decision"];

export const DECISION_LABEL: Record<Decision, string> = {
  allow: "Allow",
  fee: "Charge fee",
  flag: "Flag",
  step_up: "Step-up",
  review: "Review",
  restrict: "Restrict",
  hold: "Hold",
  block_session: "End session",
  decline: "Decline",
  block: "Block",
};

/** Every decision at zero. */
export const NO_DECISIONS: Record<Decision, number> = {
  allow: 0,
  fee: 0,
  flag: 0,
  step_up: 0,
  review: 0,
  restrict: 0,
  hold: 0,
  block_session: 0,
  decline: 0,
  block: 0,
};

/**
 * Charts group decisions into four: allowed (with or without a fee or flag), stepped up,
 * reviewed, and stopped or held (restrict, hold, end session, decline, block).
 */
export type DecisionGroup = "allow" | "step_up" | "review" | "stop";

export const DECISION_GROUP: Record<Decision, DecisionGroup> = {
  allow: "allow",
  fee: "allow",
  flag: "allow",
  step_up: "step_up",
  review: "review",
  restrict: "stop",
  hold: "stop",
  block_session: "stop",
  decline: "stop",
  block: "stop",
};

export const GROUP_LABEL: Record<DecisionGroup, string> = {
  allow: "Allow",
  step_up: "Step-up",
  review: "Review",
  stop: "Stop or hold",
};

/** Group fills for bars: the blue family, darker as the intervention grows. */
export const GROUP_FILL: Record<DecisionGroup, string> = {
  allow: "bg-blue-200",
  step_up: "bg-blue-300",
  review: "bg-brand-blue",
  stop: "bg-blue-600",
};

/** Sums decision counts into the chart groups. */
export function groupDecisions(
  counts: Partial<Record<Decision, number>>,
): Record<DecisionGroup, number> {
  const out: Record<DecisionGroup, number> = {
    allow: 0,
    step_up: 0,
    review: 0,
    stop: 0,
  };
  for (const [d, n] of Object.entries(counts) as [Decision, number][]) {
    const g = DECISION_GROUP[d];
    if (g) out[g] += n;
  }
  return out;
}

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

/** Vivid fills for meters, bars and chart marks (text uses BAND_TEXT, which meets AA). */
export const BAND_FILL: Record<RiskBand, string> = {
  minimal: "bg-risk-minimal-fill",
  low: "bg-risk-low-fill",
  elevated: "bg-risk-elevated-fill",
  high: "bg-risk-high-fill",
  critical: "bg-risk-critical-fill",
};
