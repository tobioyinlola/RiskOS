import { cn } from "@/lib/cn";
import type { ComponentProps } from "react";

/** A keyboard key, as shown in the shortcuts overlay and the command palette. */
export function Kbd({ className, ...props }: ComponentProps<"kbd">) {
  return (
    <kbd
      className={cn(
        "inline-flex h-5 min-w-5 items-center justify-center rounded-sm border border-border-strong bg-bg-surface px-1.5 font-sans text-label font-medium text-text-muted",
        className,
      )}
      {...props}
    />
  );
}
