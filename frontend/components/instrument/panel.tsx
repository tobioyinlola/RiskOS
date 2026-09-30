import { cn } from "cn";
import type { ComponentProps, ReactNode } from "react";

/** A flat panel with a line-subtle edge (no shadow: only floating layers take one). */
export function Panel({ className, ...props }: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "rounded-lg border border-line-subtle bg-bg-surface",
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
  legend?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        "flex flex-wrap items-center justify-between gap-2 border-b border-line-subtle px-4 py-3",
        className,
      )}
    >
      <div className="flex items-baseline gap-3">
        <h2 className="text-heading">{title}</h2>
        {legend ? <span className="legend">{legend}</span> : null}
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

/** Page header: title, an optional description and actions. */
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
    <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div className="max-w-2xl">
        <h1 className="text-title">{title}</h1>
        {description ? (
          <p className="mt-1 text-body text-ink-2">{description}</p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      ) : null}
    </header>
  );
}

/** A KPI tile: a legend label, a metric reading and an optional trace or note. */
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
    <div className="flex min-w-0 flex-col gap-1.5 rounded-lg border border-line-subtle bg-bg-surface p-4">
      <span className="legend">{label}</span>
      <span className="mono text-metric">
        {value}
        {unit ? (
          <span className="ml-1 text-body-sm text-ink-3">{unit}</span>
        ) : null}
      </span>
      {children}
    </div>
  );
}
