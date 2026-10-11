import { cn } from "@/lib/cn";

import {
  BAND_BADGE,
  bandFor,
  bandInfo,
  type BandShape,
  type RiskBand,
  type Thresholds,
} from "@/lib/risk";

/** Circle for Minimal and Low, triangle for Elevated and High, octagon for Critical. */
export function BandIcon({
  shape,
  className,
}: {
  shape: BandShape;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 12 12"
      aria-hidden="true"
      className={cn("size-3 shrink-0 fill-none stroke-current", className)}
      strokeWidth={2}
    >
      {shape === "circle" ? <circle cx="6" cy="6" r="4" /> : null}
      {shape === "triangle" ? <path d="M6 1.5 11 10.5H1Z" /> : null}
      {shape === "octagon" ? <path d="M4 1h4l3 3v4l-3 3H4L1 8V4Z" /> : null}
    </svg>
  );
}

export interface RiskBadgeProps {
  score?: number;
  band?: RiskBand;
  showScore?: boolean;
  thresholds?: Thresholds;
  className?: string;
}

/**
 * RiskBadge (docs/design-system.md): the band word, its glyph and optionally the score, on the
 * band's tinted ground. Use it in every table row and case header.
 */
export function RiskBadge({
  score,
  band,
  showScore = true,
  thresholds,
  className,
}: RiskBadgeProps) {
  const b = band ?? bandFor(score ?? 0, thresholds);
  const info = bandInfo(b);
  const withScore = showScore && score !== undefined;
  const label = withScore ? `${info.label}, score ${score}` : info.label;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-label whitespace-nowrap",
        BAND_BADGE[b],
        className,
      )}
      // A span has no role that takes a name; role="img" lets one name the badge as a unit.
      role="img"
      aria-label={label}
      title={label}
    >
      <BandIcon shape={info.shape} />
      <span aria-hidden="true">
        {info.label}
        {withScore ? <span className="tabular"> · {score}</span> : null}
      </span>
    </span>
  );
}
