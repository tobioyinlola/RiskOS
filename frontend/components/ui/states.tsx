import { cn } from "cn";
import { CircleCheck, CircleX, Info, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";

import { ApiError } from "@/lib/api/client";

/** Loading: skeleton blocks at the shape of what is coming, never a spinner over data. */
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
      className={cn("flex flex-col gap-3 py-5", className)}
    >
      <span className="sr-only">{label}</span>
      {[60, 92, 78].map((w) => (
        <span
          key={w}
          aria-hidden="true"
          className="block h-3 rounded-sm bg-bg-raised"
          style={{ width: `${w}%`, animation: "pulse-soft 1.4s ease-in-out infinite" }}
        />
      ))}
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
          className="h-row-default border-t border-border-subtle"
          aria-hidden="true"
        >
          {Array.from({ length: columns }, (_, c) => (
            <td key={c} className="px-3">
              <span
                className="block h-2.5 w-3/4 rounded-sm bg-bg-raised"
                style={{ animation: "pulse-soft 1.4s ease-in-out infinite" }}
              />
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
    <div className={cn("flex flex-col items-start gap-2 px-4 py-7", className)}>
      <p className="text-heading-3">{title}</p>
      {children ? (
        <div className="max-w-prose text-body text-text-secondary">
          {children}
        </div>
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
        "flex gap-2 rounded-md border border-status-danger bg-risk-critical-bg px-4 py-3",
        className,
      )}
    >
      <CircleX
        className="mt-0.5 size-icon-sm shrink-0 stroke-[1.5] text-status-danger"
        aria-hidden="true"
      />
      <div className="flex flex-col gap-1">
        <p className="text-body text-text-primary">
          {e?.message ?? "Something went wrong loading this."}
        </p>
        {e ? (
          <p className="mono text-label text-text-muted">
            {e.code}
            {e.requestId ? ` · ${e.requestId}` : ""}
          </p>
        ) : null}
      </div>
    </div>
  );
}

const NOTICE = {
  info: { Icon: Info, color: "text-status-info" },
  ok: { Icon: CircleCheck, color: "text-status-success" },
  warn: { Icon: TriangleAlert, color: "text-status-warning" },
  error: { Icon: CircleX, color: "text-status-danger" },
} as const;

/** An inline notice, always with its icon so it never relies on color. */
export function Notice({
  tone = "info",
  children,
}: {
  tone?: "info" | "ok" | "warn" | "error";
  children: ReactNode;
}) {
  const { Icon, color } = NOTICE[tone];
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      className="flex gap-2 rounded-md border border-border-subtle bg-bg-surface px-3 py-2 text-body-sm"
      style={{ animation: "fade-in 150ms ease-out" }}
    >
      <Icon
        className={cn("mt-px size-icon-sm shrink-0 stroke-[1.5]", color)}
        aria-hidden="true"
      />
      <div className="text-text-primary">{children}</div>
    </div>
  );
}
