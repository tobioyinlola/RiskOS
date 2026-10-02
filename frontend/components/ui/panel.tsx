import { cn } from "@/lib/cn";
import type { ComponentProps, ReactNode } from "react";

import { Figure } from "./figure";

/** A card: white, a 1px soft edge, 20px radius and a very soft shadow, floating on the canvas. */
export function Panel({ className, ...props }: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "rounded-lg border border-border-subtle bg-bg-surface shadow-card",
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
        "flex flex-wrap items-center justify-between gap-2 px-5 pt-5 pb-3",
        className,
      )}
    >
      <div className="flex min-w-0 items-baseline gap-3">
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
  return <div className={cn("px-5 pb-5", className)} {...props} />;
}

/**
 * Page header: an optional small muted line above (a date, a breadcrumb), the heading-1 title
 * with an optional lighter italic accent (the signature "Bold *italic.*" pattern), a description
 * and actions.
 */
export function PageHeader({
  title,
  accent,
  eyebrow,
  description,
  actions,
}: {
  title: string;
  accent?: string;
  eyebrow?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div className="max-w-2xl">
        {eyebrow ? (
          <p className="mb-1 text-body-sm text-text-muted">{eyebrow}</p>
        ) : null}
        <h1 className="text-heading-1">
          {title}
          {accent ? (
            <>
              {" "}
              <span className="accent-italic text-text-secondary">
                {accent}
              </span>
            </>
          ) : null}
        </h1>
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

/** A KPI card: a small label, a large figure (lighter decimals) and an optional chart or note. */
export function Metric({
  label,
  value,
  unit,
  icon,
  children,
}: {
  label: ReactNode;
  value: ReactNode;
  unit?: string;
  icon?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-lg border border-border-subtle bg-bg-surface p-5 shadow-card">
      <span className="flex items-center gap-2 text-body-sm text-text-secondary">
        {icon ? <IconTile size="sm">{icon}</IconTile> : null}
        {label}
      </span>
      <span className="text-metric">
        {typeof value === "string" ? <Figure value={value} /> : value}
        {unit ? (
          <span className="ml-1 text-body-sm font-normal tracking-normal text-text-muted">
            {unit}
          </span>
        ) : null}
      </span>
      {children}
    </div>
  );
}

/** An icon in a subtle rounded tile, as beside card titles and list items. */
export function IconTile({
  children,
  size = "md",
  className,
}: {
  children: ReactNode;
  size?: "sm" | "md";
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-flex shrink-0 items-center justify-center border border-border-subtle bg-bg-surface text-text-secondary",
        size === "sm"
          ? "size-[28px] rounded-sm [&_svg]:size-icon-sm"
          : "size-10 rounded-md [&_svg]:size-icon-md",
        className,
      )}
    >
      {children}
    </span>
  );
}
