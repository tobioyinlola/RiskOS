import { cn } from "cn";
import type { ComponentProps, ReactNode } from "react";

/** A flat panel with a border-subtle edge (no shadow: only floating layers take one). */
export function Panel({ className, ...props }: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "rounded-lg border border-border-subtle bg-bg-surface",
        className,
      )}
      {...props}
    />
  );
}

export function PanelHeader({
  title,
  legend,
  actions,
  className,
}: {
  title: ReactNode;
  /** A short secondary label next to the title (a count, a unit). */
  legend?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "flex flex-wrap items-center justify-between gap-2 border-b border-border-subtle px-4 py-3",
        className,
      )}
    >
      <div className="flex items-baseline gap-3">
        <h2 className="text-heading-3">{title}</h2>
        {legend ? (
          <span className="text-label text-text-muted">{legend}</span>
        ) : null}
      </div>
      {actions ? (
        <div className="flex items-center gap-2">{actions}</div>
      ) : null}
    </header>
  );
}

export function PanelBody({ className, ...props }: ComponentProps<"div">) {
  return <div className={cn("p-4", className)} {...props} />;
}

/** Page header: heading-1 title, an optional description and actions. */
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-5 flex flex-wrap items-end justify-between gap-4">
      <div className="max-w-2xl">
        <h1 className="text-heading-1">{title}</h1>
        {description ? (
          <p className="mt-1 text-body text-text-secondary">{description}</p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      ) : null}
    </header>
  );
}

/** A KPI tile: an overline label, a metric reading and an optional chart or note. */
export function Metric({
  label,
  value,
  unit,
  children,
}: {
  label: ReactNode;
  value: ReactNode;
  unit?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-lg border border-border-subtle bg-bg-surface p-4">
      <span className="text-overline uppercase text-text-secondary">{label}</span>
      <span className="mono text-metric">
        {value}
        {unit ? (
          <span className="ml-1 text-body-sm text-text-muted">{unit}</span>
        ) : null}
      </span>
      {children}
    </div>
  );
}
