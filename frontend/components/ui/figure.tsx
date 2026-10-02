import { cn } from "@/lib/cn";

/**
 * A large figure with its decimals in a lighter gray: "32,940." then "093" in text-faint
 * (docs/design-system.md, Typography → Numbers). text-faint is 3:1 on white, so use it only at
 * metric size or larger; smaller figures render in one color.
 */
export function Figure({
  value,
  className,
}: {
  /** The formatted figure, e.g. "32,940.093", "42.7", "1,204" or "—". */
  value: string;
  className?: string;
}) {
  const dot = value.lastIndexOf(".");
  const hasDecimals = dot > 0 && /^\d+\D*$/.test(value.slice(dot + 1));
  return (
    <span className={cn("tabular", className)}>
      {hasDecimals ? (
        <>
          {value.slice(0, dot + 1)}
          <span className="text-text-faint">{value.slice(dot + 1)}</span>
        </>
      ) : (
        value
      )}
    </span>
  );
}
