import { cn } from "cn";

/**
 * A small time series (docs/design-system.md, Data visualization): the primary series in accent,
 * an optional baseline dashed in text-muted, no fill. Pair it with the numbers it summarizes.
 */
export function TrendLine({
  points,
  baseline,
  width = 200,
  height = 40,
  label,
  className,
}: {
  points: number[];
  baseline?: number;
  width?: number;
  height?: number;
  label?: string;
  className?: string;
}) {
  const values = points.length > 1 ? points : [points[0] ?? 0, points[0] ?? 0];
  const lo = Math.min(...values, baseline ?? Infinity);
  const hi = Math.max(...values, baseline ?? -Infinity);
  const span = hi - lo || 1;
  const pad = 2;
  const px = (i: number) => 1 + ((width - 2) * i) / (values.length - 1);
  const py = (v: number) =>
    height - pad - ((height - 2 * pad) * (v - lo)) / span;
  const poly = values
    .map((v, i) => `${px(i).toFixed(1)},${py(v).toFixed(1)}`)
    .join(" ");
  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn("max-w-full", className)}
    >
      {baseline !== undefined ? (
        <polyline
          points={`1,${py(baseline)} ${width - 1},${py(baseline)}`}
          fill="none"
          className="stroke-text-muted"
          strokeWidth={1}
          strokeDasharray="3 3"
        />
      ) : null}
      <polyline
        points={poly}
        fill="none"
        className="stroke-accent"
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        data-series="primary"
      />
    </svg>
  );
}

/** A live indicator: a small accent dot that pulses slowly. */
export function LiveDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("inline-block size-1.5 rounded-full bg-accent", className)}
      style={{ animation: "pulse-soft 2s ease-in-out infinite" }}
    />
  );
}
