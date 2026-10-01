import { cn } from "cn";
import type { ComponentProps } from "react";

/** A keyboard key, as shown in the shortcuts overlay and the command palette. */
export function Kbd({ className, ...props }: ComponentProps<"kbd">) {
  return (
    <kbd
      className={cn(
        "mono inline-flex h-5 min-w-5 items-center justify-center rounded-sm border border-border-subtle bg-bg-sunken px-1 text-label font-normal text-text-muted",
        className,
      )}
      {...props}
    />
  );
}
