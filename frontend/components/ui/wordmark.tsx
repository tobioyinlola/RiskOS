import { cn } from "@/lib/cn";

import { initials } from "./avatar";

/**
 * The logo is the name (docs/design-system.md, Iconography): "RiskOS" set bold in the brand
 * face with tight tracking, with "OS" in brand blue. There is no symbol.
 */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-baseline text-heading-2 font-bold tracking-[-0.03em]",
        className,
      )}
    >
      Risk<span className="text-brand-blue">OS</span>
    </span>
  );
}

/**
 * A workspace's monogram: its initials on a blue tile, where a square is needed (the workspace
 * switcher). Decorative; the name is always written beside it.
 */
export function Monogram({
  name,
  className,
}: {
  name: string;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-flex size-8 shrink-0 items-center justify-center rounded-[10px] bg-brand-blue text-label font-bold text-white shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]",
        className,
      )}
    >
      {initials(name)}
    </span>
  );
}
