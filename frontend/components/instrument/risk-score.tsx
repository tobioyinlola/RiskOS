"use client";

import { useEffect, useState } from "react";

import {
  bandFor,
  bandInfo,
  DECISION_LABEL,
  DEFAULT_THRESHOLDS,
  readout,
  type Decision,
  type RiskBand,
  type Thresholds,
} from "@/lib/risk";

import { RiskBadge } from "./risk-badge";

const W = 360;
const X0 = 10;
const X1 = 344;
const x = (v: number) => X0 + ((X1 - X0) * v) / 100;

const ZONE_CLASS: Record<RiskBand, string> = {
  minimal: "fill-risk-minimal",
  low: "fill-risk-low",
  elevated: "fill-risk-elevated",
  high: "fill-risk-high",
  critical: "fill-risk-critical",
};

/** The 0–100 ruler: minor ticks every 2, major with a number every 10, thresholds, needle. */
export function Ruler({
  score,
  thresholds = DEFAULT_THRESHOLDS,
  animate = true,
}: {
  score: number;
  thresholds?: Thresholds;
  animate?: boolean;
}) {
  const [needle, setNeedle] = useState(animate ? 0 : score);
  useEffect(() => {
    const id = requestAnimationFrame(() => setNeedle(score));
    return () => cancelAnimationFrame(id);
  }, [score]);

  const edges = [
    0,
    thresholds.low,
    thresholds.elevated,
    thresholds.high,
    thresholds.critical,
    100,
  ];
  const bands: RiskBand[] = ["minimal", "low", "elevated", "high", "critical"];
  const current = bandFor(score, thresholds);
  const ticks = Array.from({ length: 51 }, (_, i) => i * 2);
  return (
    <svg
      width="100%"
      viewBox={`0 0 ${W} 52`}
      className="block max-w-[360px]"
      aria-hidden="true"
    >
      {bands.map((b, i) => {
        const from = edges[i]!;
        const to = edges[i + 1]!;
        const above = bands.indexOf(b) > bands.indexOf(current);
        return (
          <rect
            key={b}
            x={x(from) + 1}
            y={30}
            width={Math.max(0, x(to) - x(from) - 2)}
            height={4}
            className={ZONE_CLASS[b]}
            opacity={above ? 0.3 : 1}
          />
        );
      })}
      {ticks.map((t) => {
        const major = t % 10 === 0;
        return (
          <g key={t}>
            <line
              x1={x(t)}
              x2={x(t)}
              y1={major ? 16 : 22}
              y2={26}
              className={major ? "stroke-ink-3" : "stroke-line-subtle"}
              strokeWidth={1}
            />
            {major ? (
              <text
                x={x(t)}
                y={48}
                textAnchor="middle"
                className="mono fill-ink-3 text-[10px]"
              >
                {t}
              </text>
            ) : null}
          </g>
        );
      })}
      {[
        thresholds.low,
        thresholds.elevated,
        thresholds.high,
        thresholds.critical,
      ].map((t) => (
        <line
          key={`thr-${t}`}
          x1={x(t)}
          x2={x(t)}
          y1={10}
          y2={36}
          className="stroke-line-control"
          strokeWidth={1}
          strokeDasharray="2 2"
        />
      ))}
      <g
        style={{ transform: `translateX(${x(needle) - x(0)}px)` }}
        className="transition-transform duration-[180ms] ease-instrument"
      >
        <line
          x1={x(0)}
          x2={x(0)}
          y1={4}
          y2={38}
          className="stroke-ink"
          strokeWidth={2}
        />
        <rect x={x(0) - 3} y={2} width={6} height={6} className="fill-ink" />
      </g>
    </svg>
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
 * RiskScore (docs/design-system.md): a three-digit readout in a sunken well, the band badge, a
 * ruler with the needle at the score, and the decision. One per view; follow it with ReasonList.
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
  const r = readout(score);
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end gap-4">
        <div
          role="meter"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={score}
          aria-label={`Risk score ${score} of 100, ${bandInfo(b).label}`}
          className="mono rounded-0 bg-bg-sunken px-3 pt-1.5 pb-1 text-readout"
        >
          <span className="text-ink-3">{r.lead}</span>
          {r.value}
        </div>
        <div className="flex flex-col gap-1.5 pb-1">
          <span className="legend">Risk score / 100</span>
          <RiskBadge score={score} band={b} />
        </div>
      </div>
      <div className="flex items-center gap-2">
        <Ruler score={score} thresholds={thresholds} />
        {customCalibration ? (
          <span className="legend shrink-0">Custom calibration</span>
        ) : null}
      </div>
      <div className="flex items-center justify-between gap-4 text-body-sm text-ink-2">
        <span>
          Decision{" "}
          <b className="font-semibold text-ink">{DECISION_LABEL[decision]}</b>
        </span>
        {engineVersion ? <span className="legend">{engineVersion}</span> : null}
      </div>
    </div>
  );
}
