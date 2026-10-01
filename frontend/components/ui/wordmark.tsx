import { cn } from "cn";

/**
 * The wordmark (docs/design-system.md, Iconography): no logo has been supplied, so "RiskOS" is set
 * in IBM Plex Sans 600. `compact` shows the monogram for the collapsed navigation rail.
 */
export function Wordmark({
  compact = false,
  className,
}: {
  compact?: boolean;
  className?: string;
}) {
  if (compact) {
    return (
      <span
        className={cn(
          "inline-flex size-control-sm items-center justify-center rounded-md bg-accent text-body font-semibold text-on-accent",
          className,
        )}
        aria-label="RiskOS"
      >
        R
      </span>
    );
  }
  return (
    <span className={cn("text-heading-3 font-semibold", className)}>
      RiskOS
    </span>
  );
}
