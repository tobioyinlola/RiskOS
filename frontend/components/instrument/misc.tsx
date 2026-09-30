"use client";

import { cn } from "cn";
import { Tabs as T } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";

/** Tabs: the active tab is underlined by the trace (signal-ink). */
export const Tabs = T.Root;

export function TabList({
  className,
  ...props
}: ComponentProps<typeof T.List>) {
  return (
    <T.List
      className={cn("flex gap-4 border-b border-line-subtle", className)}
      {...props}
    />
  );
}

export function Tab({ className, ...props }: ComponentProps<typeof T.Trigger>) {
  return (
    <T.Trigger
      className={cn(
        "-mb-px border-b-[1.5px] border-transparent pb-2 text-body text-ink-2 hover:text-ink data-[state=active]:border-signal-ink data-[state=active]:text-ink",
        className,
      )}
      {...props}
    />
  );
}

export const TabPanel = T.Content;

/** A definition list of readings: legend labels, mono values. */
export function KeyValues({
  items,
  className,
}: {
  items: [ReactNode, ReactNode][];
  className?: string;
}) {
  return (
    <dl
      className={cn(
        "grid grid-cols-[minmax(120px,auto)_1fr] gap-x-4 gap-y-2",
        className,
      )}
    >
      {items.map(([k, v], i) => (
        <div key={i} className="contents">
          <dt className="legend pt-0.5">{k}</dt>
          <dd className="min-w-0 text-body-sm break-words">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

/** JSON, code and payloads: mono on bg-sunken, square corners. */
export function CodeBlock({
  value,
  className,
}: {
  value: unknown;
  className?: string;
}) {
  const text =
    typeof value === "string" ? value : JSON.stringify(value, null, 2);
  return (
    <pre
      className={cn(
        "mono max-h-96 overflow-auto rounded-0 bg-bg-sunken p-3 text-data text-ink-2",
        className,
      )}
    >
      {text}
    </pre>
  );
}

/** Status of a system object (key, webhook): the line is live, or it is not. */
export function Status({
  tone,
  children,
}: {
  tone: "ok" | "warn" | "error" | "off";
  children: ReactNode;
}) {
  const color = {
    ok: "text-status-ok",
    warn: "text-status-warn",
    error: "text-status-error",
    off: "text-ink-3",
  }[tone];
  const mark = { ok: "●", warn: "▲", error: "×", off: "○" }[tone];
  return (
    <span className={cn("inline-flex items-center gap-1.5 text-label", color)}>
      <span aria-hidden="true" className="mono text-[9px]">
        {mark}
      </span>
      <span className="text-ink">{children}</span>
    </span>
  );
}

/** A neutral tag (status words, event types). Square-ish, never a risk colour. */
export function Tag({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-[22px] items-center rounded-sm border border-line-subtle px-2 text-label text-ink-2",
        className,
      )}
    >
      {children}
    </span>
  );
}
