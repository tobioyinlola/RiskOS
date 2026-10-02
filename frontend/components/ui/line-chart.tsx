"use client";

import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { formatCount } from "@/lib/format";

export interface SeriesSpec {
  key: string;
  label: string;
  /** A CSS color, normally a token: "var(--chart-1)". */
  color: string;
}

/**
 * The overview chart (docs/design-system.md, Data visualization): smooth lines (blue, then teal)
 * on a dotted grid, muted axis labels with the active (latest) label in bold, and a tooltip card
 * with the bucket and each labelled series. Pair it with the figures it summarizes.
 */
export function SeriesChart<T extends { label: string }>({
  data,
  series,
  height = 220,
  ariaLabel,
}: {
  data: T[];
  series: SeriesSpec[];
  height?: number;
  ariaLabel: string;
}) {
  const active = data.at(-1)?.label;
  return (
    <div role="img" aria-label={ariaLabel} style={{ height }}>
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -16 }}>
          <CartesianGrid
            stroke="var(--chart-grid)"
            strokeDasharray="2 4"
            vertical={false}
          />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            interval="preserveStartEnd"
            minTickGap={16}
            tick={({ x, y, payload }) => (
              <text
                x={x}
                y={Number(y) + 12}
                textAnchor="middle"
                fontSize={12}
                fill={
                  payload.value === active
                    ? "var(--text-primary)"
                    : "var(--chart-axis)"
                }
                fontWeight={payload.value === active ? 600 : 400}
              >
                {payload.value}
              </text>
            )}
          />
          <YAxis
            tickLine={false}
            axisLine={false}
            allowDecimals={false}
            width={48}
            tick={{ fontSize: 12, fill: "var(--chart-axis)" }}
          />
          <Tooltip
            cursor={{ stroke: "var(--border-strong)", strokeWidth: 1 }}
            content={({ active: on, payload, label }) =>
              on && payload?.length ? (
                <div className="min-w-40 rounded-md border border-border-subtle bg-bg-surface px-3 py-2.5 shadow-overlay">
                  <p className="mb-1.5 text-label text-text-muted">{label}</p>
                  <ul className="flex flex-col gap-1">
                    {series.map((s) => {
                      const v = payload.find((p) => p.dataKey === s.key)?.value;
                      return (
                        <li
                          key={s.key}
                          className="flex items-center justify-between gap-4 text-body-sm"
                        >
                          <span className="flex items-center gap-1.5 text-text-secondary">
                            <span
                              aria-hidden="true"
                              className="size-2 rounded-full"
                              style={{ background: s.color }}
                            />
                            {s.label}
                          </span>
                          <span className="tabular font-semibold">
                            {typeof v === "number" ? formatCount(v) : "—"}
                          </span>
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ) : null
            }
          />
          {series.map((s) => (
            <Line
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.label}
              stroke={s.color}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, strokeWidth: 2, stroke: "var(--bg-surface)" }}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

/** A legend for SeriesChart. */
export function SeriesLegend({ series }: { series: SeriesSpec[] }) {
  return (
    <ul className="flex flex-wrap gap-4 text-label text-text-secondary">
      {series.map((s) => (
        <li key={s.key} className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-0.5 w-3 rounded-full"
            style={{ background: s.color }}
          />
          {s.label}
        </li>
      ))}
    </ul>
  );
}
