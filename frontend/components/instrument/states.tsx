import { cn } from "cn";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";

/** Loading: a flat trace that pulses (docs/design-system.md, Focus and states). */
export function Loading({
  label = "Loading",
  className,
}: {
  label?: string;
  className?: string;
}) {
  return (
    <div
      role="status"
      aria-live="polite"
      className={cn("flex items-center gap-3 py-6", className)}
    >
      <svg width="120" height="8" viewBox="0 0 120 8" aria-hidden="true">
        <line
          x1="0"
          x2="120"
          y1="4"
          y2="4"
          className="stroke-signal-ink"
          strokeWidth="1.5"
          style={{ animation: "flat-pulse 1.2s ease-in-out infinite" }}
        />
      </svg>
      <span className="legend">{label}</span>
    </div>
  );
}

/** Skeleton rows at the real row height. */
export function SkeletonRows({
  rows = 6,
  columns = 5,
}: {
  rows?: number;
  columns?: number;
}) {
  return (
    <>
      {Array.from({ length: rows }, (_, r) => (
        <tr
          key={r}
          className="h-row-default border-t border-line-subtle"
          aria-hidden="true"
        >
          {Array.from({ length: columns }, (_, c) => (
            <td key={c} className="px-3">
              <span className="block h-2 w-3/4 bg-bg-raised" />
            </td>
          ))}
        </tr>
      ))}
    </>
  );
}

/** Empty state: one calm sentence and, where useful, the next action. */
export function Empty({
  title,
  children,
  action,
  className,
}: {
  title: string;
  children?: ReactNode;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-start gap-2 px-4 py-8", className)}>
      <p className="text-heading">{title}</p>
      {children ? (
        <div className="max-w-prose text-body text-ink-2">{children}</div>
      ) : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

/** An error from the API, with its request ID for support. */
export function ErrorState({
  error,
  className,
}: {
  error: unknown;
  className?: string;
}) {
  const e = error instanceof ApiError ? error : undefined;
  return (
    <div
      role="alert"
      className={cn(
        "flex flex-col gap-1 border-l-2 border-status-error px-4 py-3",
        className,
      )}
    >
      <p className="text-body text-ink">
        {e?.message ?? "Something went wrong loading this."}
      </p>
      {e ? (
        <p className="legend">
          {e.code}
          {e.requestId ? ` · ${e.requestId}` : ""}
        </p>
      ) : null}
    </div>
  );
}

/** A small inline notice (success, warning or error), always with a leading mark. */
export function Notice({
  tone = "info",
  children,
}: {
  tone?: "info" | "ok" | "warn" | "error";
  children: ReactNode;
}) {
  const mark = { info: "·", ok: "✓", warn: "!", error: "×" }[tone];
  const color = {
    info: "text-ink-2 border-line-control",
    ok: "text-status-ok border-status-ok",
    warn: "text-status-warn border-status-warn",
    error: "text-status-error border-status-error",
  }[tone];
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      className={cn("flex gap-2 border-l-2 px-3 py-2 text-body-sm", color)}
    >
      <span className="mono" aria-hidden="true">
        {mark}
      </span>
      <div className="text-ink">{children}</div>
    </div>
  );
}
