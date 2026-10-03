"use client";

import { cn } from "@/lib/cn";
import { Tabs as T } from "radix-ui";
import { useState, type ComponentProps, type ReactNode } from "react";

import { Alert, Check, CheckCircle, Copy, Minus, XCircle } from "./icons";

/**
 * The pill tab look, for tab buttons that drive a URL or a filter rather than Radix Tabs: the
 * selected tab is the ink pill, the others white with a 1px edge.
 */
export function pillTabClass(selected: boolean, className?: string) {
  return cn(
    "inline-flex h-control-sm items-center gap-1.5 rounded-full border px-3.5 text-body-sm font-medium whitespace-nowrap transition-colors duration-150",
    selected
      ? "border-ink bg-ink text-white"
      : "border-border-strong bg-bg-surface text-text-secondary hover:border-border-control hover:text-text-primary",
    className,
  );
}

/** Tabs: pills; the selected tab is the ink pill, the others white with a 1px edge. */
export const Tabs = T.Root;

export function TabList({
  className,
  ...props
}: ComponentProps<typeof T.List>) {
  return (
    <T.List
      className={cn("flex flex-wrap gap-2", className)}
      {...props}
    />
  );
}

export function Tab({ className, ...props }: ComponentProps<typeof T.Trigger>) {
  return (
    <T.Trigger
      className={cn(
        "inline-flex h-control-sm items-center gap-1.5 rounded-full border border-border-strong bg-bg-surface px-3.5 text-body-sm font-medium text-text-secondary transition-colors duration-150 hover:border-border-control hover:text-text-primary data-[state=active]:border-ink data-[state=active]:bg-ink data-[state=active]:text-white",
        className,
      )}
      {...props}
    />
  );
}

export function TabPanel({
  className,
  ...props
}: ComponentProps<typeof T.Content>) {
  return (
    <T.Content
      className={cn("outline-none", className)}
      style={{ animation: "fade-in 150ms ease-out" }}
      {...props}
    />
  );
}

/** A definition list: label terms, values. */
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
          <dt className="pt-0.5 text-label text-text-secondary">{k}</dt>
          <dd className="min-w-0 text-body-sm break-words">{v}</dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * Evidence is one click to copy (docs/design-system.md, Principles): an ID, key or hash in
 * mono with a copy button.
 */
export function CopyValue({
  value,
  display,
  className,
}: {
  value: string;
  display?: ReactNode;
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard access can be denied; the value stays selectable.
    }
  };
  return (
    <span className={cn("group/copy inline-flex min-w-0 items-center gap-1", className)}>
      <span className="mono truncate text-data">{display ?? value}</span>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          void copy();
        }}
        className="inline-flex size-6 shrink-0 items-center justify-center rounded-sm text-text-muted opacity-60 transition-opacity duration-150 group-hover/copy:opacity-100 hover:bg-bg-raised hover:text-text-primary focus-visible:opacity-100"
        aria-label={copied ? "Copied" : `Copy ${value}`}
        title={copied ? "Copied" : "Copy"}
      >
        {copied ? (
          <Check className="size-3.5 text-status-success" />
        ) : (
          <Copy className="size-3.5" />
        )}
      </button>
    </span>
  );
}

/**
 * JSON, code and payloads: mono on bg-sunken, with a copy button. The block scrolls, so it is a
 * labelled region keyboard users can reach (WCAG 2.1.1).
 */
export function CodeBlock({
  value,
  className,
  label = "Code",
}: {
  value: unknown;
  className?: string;
  label?: string;
}) {
  const text =
    typeof value === "string" ? value : JSON.stringify(value, null, 2);
  const [copied, setCopied] = useState(false);
  return (
    <div className="relative">
      <pre
        tabIndex={0}
        role="region"
        aria-label={label}
        className={cn(
          "mono max-h-96 overflow-auto rounded-md border border-border-subtle bg-bg-sunken p-3 pr-10 text-data text-text-secondary",
          className,
        )}
      >
        {text}
      </pre>
      <button
        type="button"
        onClick={() =>
          void navigator.clipboard
            .writeText(text)
            .then(() => {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            })
            .catch(() => undefined)
        }
        className="absolute top-2 right-2 inline-flex size-6 items-center justify-center rounded-sm text-text-muted hover:bg-bg-raised hover:text-text-primary"
        aria-label={copied ? "Copied" : "Copy"}
        title={copied ? "Copied" : "Copy"}
      >
        {copied ? (
          <Check className="size-3.5 text-status-success" />
        ) : (
          <Copy className="size-3.5" />
        )}
      </button>
    </div>
  );
}

const STATUS = {
  ok: { Icon: CheckCircle, color: "text-status-success" },
  warn: { Icon: Alert, color: "text-status-warning" },
  error: { Icon: XCircle, color: "text-status-danger" },
  off: { Icon: Minus, color: "text-text-muted" },
} as const;

/** System health (keys, webhooks): status-* color, always with its icon. Never a risk level. */
export function Status({
  tone,
  children,
}: {
  tone: "ok" | "warn" | "error" | "off";
  children: ReactNode;
}) {
  const { Icon, color } = STATUS[tone];
  return (
    <span className="inline-flex items-center gap-1 text-label">
      <Icon
        aria-hidden="true"
        className={cn("size-icon-sm shrink-0", color)}
      />
      <span className="text-text-primary">{children}</span>
    </span>
  );
}

/** A neutral tag (status words, event types). Never a risk color. */
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
        "inline-flex items-center rounded-full border border-border-strong bg-bg-surface px-2.5 py-0.5 text-label whitespace-nowrap text-text-secondary",
        className,
      )}
    >
      {children}
    </span>
  );
}
