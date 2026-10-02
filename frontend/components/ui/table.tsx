"use client";

import { cn } from "@/lib/cn";
import type { ComponentProps } from "react";

import { usePreferences } from "@/lib/preferences";

/**
 * Tables (docs/design-system.md, Table card): real <table> markup, a quiet header of muted
 * labels over a soft divider, rows divided by border-subtle, hover bg-raised and the selected
 * row in blue-50. Row height follows the density preference (row-default or row-compact) unless
 * set. Inside a card, the first and last columns align with the card's padding.
 */
export function Table({ className, ...props }: ComponentProps<"table">) {
  return (
    <div className="overflow-x-auto">
      <table
        className={cn(
          "w-full border-collapse text-body-sm [&_td:first-child]:pl-5 [&_td:last-child]:pr-5 [&_th:first-child]:pl-5 [&_th:last-child]:pr-5",
          className,
        )}
        {...props}
      />
    </div>
  );
}

export function Th({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "h-10 border-b border-border-subtle px-3 text-left text-label font-medium whitespace-nowrap text-text-muted",
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
        "group border-t border-border-subtle transition-colors duration-150 ease-out first:border-t-0",
        d === "compact" ? "h-row-compact" : "h-row-default",
        selected ? "bg-blue-50" : "hover:bg-bg-raised",
        className,
      )}
      {...props}
    />
  );
}

export function Td({ className, ...props }: ComponentProps<"td">) {
  return <td className={cn("px-3 align-middle", className)} {...props} />;
}

/** A mono data cell: IDs, keys and timestamps. */
export function DataCell({ className, ...props }: ComponentProps<"td">) {
  return (
    <Td
      className={cn("mono text-data whitespace-nowrap", className)}
      {...props}
    />
  );
}

/** A row checkbox, styled to the system (ink when checked). */
export function RowCheckbox({
  className,
  ...props
}: Omit<ComponentProps<"input">, "type">) {
  return (
    <input
      type="checkbox"
      className={cn(
        "size-4 cursor-pointer rounded-[5px] border-border-control accent-ink",
        className,
      )}
      {...props}
    />
  );
}
