import { cn } from "cn";

/**
 * TraceLine (docs/design-system.md): the brand's thin sparkline, flat until it spikes. A
 * trace-stroke line in signal-ink with miter joins and no fill; an optional dashed baseline. It
 * draws left to right once on mount (skipped with reduced motion).
 */
export function TraceLine({
  points,
  baseline,
  width = 200,
  height = 40,
  live = false,
  label,
  className,
}: {
  points: number[];
  baseline?: number;
  width?: number;
  height?: number;
  live?: boolean;
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
  const length = width * 3;
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <svg
        width={width}
        height={height}
        viewBox={`0 0 ${width} ${height}`}
        role={label ? "img" : undefined}
        aria-label={label}
        aria-hidden={label ? undefined : true}
      >
        {baseline !== undefined ? (
          <polyline
            points={`1,${py(baseline)} ${width - 1},${py(baseline)}`}
            fill="none"
            className="stroke-ink-3"
            strokeWidth={1}
            strokeDasharray="3 3"
          />
        ) : null}
        <polyline
          points={poly}
          className="trace"
          style={{
            strokeDasharray: length,
            ["--trace-length" as string]: length,
            animation: "trace-draw 600ms var(--ease-instrument) both",
          }}
        />
      </svg>
      {live ? <LiveDot /> : null}
    </span>
  );
}

/** The live dot: 6px signal-ink, pulsing slowly. */
export function LiveDot({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-block size-1.5 rounded-full bg-signal-ink",
        className,
      )}
      style={{ animation: "live-pulse 2s ease-in-out infinite" }}
    />
  );
}
