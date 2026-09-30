import { cn } from "cn";

import { bandFor, bandInfo, type RiskBand, type Thresholds } from "@/lib/risk";

const BAND_CLASS: Record<RiskBand, string> = {
  minimal: "text-risk-minimal bg-risk-minimal-wash",
  low: "text-risk-low bg-risk-low-wash",
  elevated: "text-risk-elevated bg-risk-elevated-wash",
  high: "text-risk-high bg-risk-high-wash",
  critical: "text-risk-critical bg-risk-critical-wash",
};

const BAR_HEIGHT = ["h-1", "h-[5.5px]", "h-[7px]", "h-[8.5px]", "h-2.5"];

/** The band glyph: 1–5 filled bars, like signal strength, so bands survive greyscale and print. */
export function BandBars({ bars }: { bars: number }) {
  return (
    <span
      className="inline-flex h-2.5 items-end gap-[1.5px]"
      aria-hidden="true"
    >
      {BAR_HEIGHT.map((h, i) => (
        <i
          key={h}
          className={cn(
            "w-[2.5px] bg-current",
            h,
            i < bars ? "opacity-100" : "opacity-[0.28]",
          )}
        />
      ))}
    </span>
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
 * RiskBadge (docs/design-system.md): the band three ways at once (bars, word, colour on its
 * wash), followed by the score. Use it in every row, drawer header and export.
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
  const label =
    score === undefined ? info.label : `${info.label}, score ${score}`;
  return (
    <span
      className={cn(
        "inline-flex h-[22px] items-center gap-1.5 rounded-sm px-2 text-label",
        BAND_CLASS[b],
        className,
      )}
      aria-label={label}
      title={label}
    >
      <BandBars bars={info.bars} />
      <span aria-hidden="true">{info.label}</span>
      {showScore && score !== undefined ? (
        <span className="mono opacity-85" aria-hidden="true">
          {String(score).padStart(2, "0")}
        </span>
      ) : null}
    </span>
  );
}
