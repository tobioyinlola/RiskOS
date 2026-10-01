"use client";

import { cn } from "cn";
import type { ComponentProps } from "react";

import { usePreferences } from "@/lib/preferences";

/**
 * Tables (docs/design-system.md, AlertRow): real <table> markup, a label header strip on
 * bg-sunken, rows divided by border-subtle, hover bg-raised, the selected row accent-subtle.
 * Row height follows the density preference (row-default or row-compact) unless set.
 */
export function Table({ className, ...props }: ComponentProps<"table">) {
  return (
    <div className="overflow-x-auto">
      <table
        className={cn("w-full border-collapse text-body-sm", className)}
        {...props}
      />
    </div>
  );
}

export function Th({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "h-8 bg-bg-sunken px-3 text-left text-label whitespace-nowrap text-text-secondary",
        className,
      )}
      {...props}
    />
  );
}

export function Tr({
  className,
  selected,
  density,
  ...props
}: ComponentProps<"tr"> & {
  selected?: boolean;
  density?: "default" | "compact";
}) {
  const prefs = usePreferences();
  const d = density ?? prefs.density;
  return (
    <tr
      data-selected={selected || undefined}
      aria-selected={selected}
      className={cn(
        "group border-t border-border-subtle transition-colors duration-150 ease-out",
        d === "compact" ? "h-row-compact" : "h-row-default",
        selected ? "bg-accent-subtle" : "hover:bg-bg-raised",
        className,
      )}
      {...props}
    />
  );
}

export function Td({ className, ...props }: ComponentProps<"td">) {
  return <td className={cn("px-3 align-middle", className)} {...props} />;
}

/** A mono data cell: IDs, amounts, timestamps. */
export function DataCell({ className, ...props }: ComponentProps<"td">) {
  return <Td className={cn("mono text-data", className)} {...props} />;
}
