import { cn } from "@/lib/cn";
import type React from "react";

/**
 * A segmented horizontal bar: one rounded segment per part, sized by share, with 3px gaps.
 * Decorative: pair it with the figures or a legend it summarizes.
 */
export function SegmentedBar({
  parts,
  className,
}: {
  parts: { key: string; value: number; fill: string; label: string }[];
  className?: string;
}) {
  const total = parts.reduce((s, p) => s + p.value, 0);
  const shown = parts.filter((p) => p.value > 0);
  return (
    <div
      className={cn("flex h-2.5 w-full gap-[3px]", className)}
      aria-hidden="true"
    >
      {total === 0 ? (
        <span className="h-full w-full rounded-full bg-bg-sunken" />
      ) : (
        shown.map((p) => (
          <span
            key={p.key}
            title={`${p.label}: ${p.value}`}
            className={cn(
              "h-full rounded-full transition-[flex-grow] duration-200 ease-out",
              p.fill,
            )}
            style={{ flexGrow: p.value, flexBasis: 0, minWidth: 6 }}
          />
        ))
      )}
    </div>
  );
}

/** A legend for a SegmentedBar: dot, label and share. */
export function SegmentLegend({
  parts,
}: {
  parts: { key: string; value: number; fill: string; label: string }[];
}) {
  const total = parts.reduce((s, p) => s + p.value, 0);
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-label text-text-secondary">
      {parts.map((p) => (
        <li key={p.key} className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className={cn("size-2 rounded-full", p.fill)}
          />
          {p.label}
          <span className="tabular text-text-muted">
            {total ? Math.round((p.value / total) * 100) : 0}%
          </span>
        </li>
      ))}
    </ul>
  );
}

/**
 * The barcode score bar: thin rounded ticks across the width, blue up to the score and pale
 * after it, with the figure on the right. `ticks` is the resolution.
 */
export function TickScoreBar({
  value,
  max = 100,
  ticks = 48,
  label,
  fill = "bg-brand-blue",
  children,
  className,
}: {
  value: number;
  max?: number;
  ticks?: number;
  label: string;
  fill?: string;
  /** The figure shown on the right, e.g. a <Figure>. */
  children?: React.ReactNode;
  className?: string;
}) {
  const filled = Math.round((Math.max(0, Math.min(value, max)) / max) * ticks);
  return (
    <div
      role="meter"
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      aria-label={label}
      className={cn("flex items-center gap-4", className)}
    >
      <div className="flex h-9 flex-1 items-stretch justify-between gap-[2px]">
        {Array.from({ length: ticks }, (_, i) => (
          <span
            key={i}
            aria-hidden="true"
            className={cn(
              "w-[3px] rounded-full transition-colors duration-200",
              i < filled ? fill : "bg-blue-100",
            )}
          />
        ))}
      </div>
      {children ? (
        <span className="shrink-0 text-metric">{children}</span>
      ) : null}
    </div>
  );
}
