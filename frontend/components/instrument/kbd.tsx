import { cn } from "cn";
import type { ComponentProps } from "react";

/** A keyboard key, as shown next to queue actions and in the shortcuts overlay. */
export function Kbd({ className, ...props }: ComponentProps<"kbd">) {
  return (
    <kbd
      className={cn(
        "mono inline-flex h-4 min-w-4 items-center justify-center rounded-sm border border-line-subtle px-1 text-legend font-normal text-ink-3",
        className,
      )}
      {...props}
    />
  );
}
