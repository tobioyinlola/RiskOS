import { cn } from "@/lib/cn";

import {
  BAND_FILL,
  BAND_TEXT,
  bandFor,
  bandInfo,
  bandRanges,
  DECISION_LABEL,
  DEFAULT_THRESHOLDS,
  type Decision,
  type RiskBand,
  type Thresholds,
} from "@/lib/risk";

import { RiskBadge } from "./risk-badge";

/**
 * The 0–100 meter split at the band thresholds: segments up to the score's band are solid,
 * those above at 35% opacity, with a marker at the score and the thresholds as a legend.
 */
export function RiskMeter({
  score,
  thresholds = DEFAULT_THRESHOLDS,
  className,
}: {
  score?: number;
  thresholds?: Thresholds;
  className?: string;
}) {
  const ranges = bandRanges(thresholds);
  const current = score === undefined ? null : bandFor(score, thresholds);
  const order = ranges.map((r) => r.band);
  const legend = [
    0,
    thresholds.low,
    thresholds.elevated,
    thresholds.high,
    thresholds.critical,
    100,
  ];
  return (
    <div className={cn("w-full", className)} aria-hidden="true">
      <div className="relative flex h-2.5 gap-[3px]">
        {ranges.map((r) => {
          const above =
            current !== null && order.indexOf(r.band) > order.indexOf(current);
          return (
            <span
              key={r.band}
              className={cn(
                "rounded-full",
                BAND_FILL[r.band],
                current === null || above ? "opacity-35" : "",
              )}
              style={{ flexGrow: Math.max(1, Math.min(r.to, 100) - r.from + 1) }}
            />
          );
        })}
        {score !== undefined ? (
          <span
            className="absolute -top-1.5 h-5.5 w-1 rounded-full bg-ink ring-2 ring-bg-surface transition-[left] duration-200 ease-out"
            style={{ left: `calc(${Math.max(0, Math.min(100, score))}% - 1px)` }}
          />
        ) : null}
      </div>
      <div className="tabular relative mt-2 h-4 text-label text-text-muted">
        {legend.map((v, i) => (
          <span
            key={`${v}-${i}`}
            className={cn(
              "absolute",
              i === 0
                ? ""
                : i === legend.length - 1
                  ? "-translate-x-full"
                  : "-translate-x-1/2",
            )}
            style={{ left: `${v}%` }}
          >
            {v}
          </span>
        ))}
      </div>
    </div>
  );
}

export interface RiskScoreProps {
  score: number;
  band?: RiskBand;
  decision: Decision;
  engineVersion?: string;
  thresholds?: Thresholds;
  customCalibration?: boolean;
}

/**
 * RiskScore (docs/design-system.md): the headline score with its band, the meter and the
 * decision taken. One per view; place a ReasonList beside or beneath it.
 */
export function RiskScore({
  score,
  band,
  decision,
  engineVersion,
  thresholds,
  customCalibration,
}: RiskScoreProps) {
  const b = band ?? bandFor(score, thresholds);
  return (
    <div className="flex flex-col gap-3">
      <div
        role="meter"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={score}
        aria-label={`Risk score ${score}, ${bandInfo(b).label}`}
        className="flex items-baseline gap-2"
      >
        <span className={cn("tabular text-score-xl", BAND_TEXT[b])}>{score}</span>
        <span className="tabular text-body-sm text-text-muted">/ 100</span>
        <RiskBadge band={b} showScore={false} className="ml-auto" />
      </div>
      <RiskMeter score={score} thresholds={thresholds} />
      {customCalibration ? (
        <p className="text-label text-text-muted">Custom thresholds</p>
      ) : null}
      <p className="text-body-sm text-text-secondary">
        Decision:{" "}
        <b className="font-semibold text-text-primary">
          {DECISION_LABEL[decision]}
        </b>
        {engineVersion ? (
          <>
            {" "}
            · engine <span className="mono">{engineVersion}</span>
          </>
        ) : null}
      </p>
    </div>
  );
}
