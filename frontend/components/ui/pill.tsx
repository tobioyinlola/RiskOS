import { cn } from "@/lib/cn";
import type { ReactNode } from "react";

import { DECISION_LABEL, type Decision } from "@/lib/risk";

/** Soft status tones; each pairs an AA text color with its tinted ground and a vivid dot. */
export type Tone = "neutral" | "info" | "success" | "warning" | "danger";

const TONE: Record<Tone, { pill: string; dot: string }> = {
  neutral: {
    pill: "bg-risk-minimal-bg text-risk-minimal",
    dot: "bg-risk-minimal-fill",
  },
  info: { pill: "bg-status-info-bg text-status-info", dot: "bg-brand-blue" },
  success: {
    pill: "bg-status-success-bg text-status-success",
    dot: "bg-status-success-fill",
  },
  warning: {
    pill: "bg-status-warning-bg text-status-warning",
    dot: "bg-brand-orange",
  },
  danger: {
    pill: "bg-status-danger-bg text-status-danger",
    dot: "bg-brand-red",
  },
};

/**
 * StatusPill: a soft tinted pill with a colored dot and a word. The word always carries the
 * meaning; the color only reinforces it.
 */
export function StatusPill({
  tone = "neutral",
  dot = true,
  children,
  className,
}: {
  tone?: Tone;
  dot?: boolean;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-label whitespace-nowrap",
        TONE[tone].pill,
        className,
      )}
    >
      {dot ? (
        <span
          aria-hidden="true"
          className={cn("size-1.5 shrink-0 rounded-full", TONE[tone].dot)}
        />
      ) : null}
      {children}
    </span>
  );
}

/** Decisions as pills: Allow reads as completed, Block as stopped, Review as pending. */
export const DECISION_TONE: Record<Decision, Tone> = {
  allow: "success",
  fee: "neutral",
  flag: "info",
  step_up: "info",
  restrict: "danger",
  hold: "warning",
  block_session: "danger",
  decline: "danger",
  review: "warning",
  block: "danger",
};

/** A decision as a status pill: "Allow", "Step-up", "Review" or "Block". */
export function DecisionPill({ decision }: { decision: string }) {
  const d = decision as Decision;
  return (
    <StatusPill tone={DECISION_TONE[d] ?? "neutral"}>
      {DECISION_LABEL[d] ?? decision}
    </StatusPill>
  );
}

/** An outlined pill tag (landing panels, filters). */
export function OutlinePill({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border border-border-strong bg-bg-surface px-3 py-1 text-label text-text-secondary",
        className,
      )}
    >
      {children}
    </span>
  );
}
