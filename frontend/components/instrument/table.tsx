import { cn } from "cn";
import type { ComponentProps } from "react";

/**
 * Tables: header strip in legend on bg-sunken, rows divided by line-subtle, hover bg-raised.
 * A selected row takes signal-wash plus a 2px signal-ink needle at its left edge.
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
        "legend h-[30px] bg-bg-sunken px-3 text-left font-medium whitespace-nowrap",
        className,
      )}
      {...props}
    />
  );
}

export function Tr({
  className,
  selected,
  density = "default",
  ...props
}: ComponentProps<"tr"> & {
  selected?: boolean;
  density?: "default" | "compact";
}) {
  return (
    <tr
      data-selected={selected || undefined}
      aria-selected={selected}
      className={cn(
        "group border-t border-line-subtle",
        density === "compact" ? "h-row-compact" : "h-row-default",
        selected
          ? "bg-signal-wash [&>td:first-child]:shadow-[inset_2px_0_0_var(--signal-ink)]"
          : "hover:bg-bg-raised",
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
