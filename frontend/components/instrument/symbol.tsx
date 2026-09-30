import { cn } from "cn";

/**
 * The RiskOS symbol: a dial ring crossed by a trace spike. Ring in ink, trace in signal
 * (signal-ink on paper). The ring never animates; the trace may draw on once.
 */
export function Symbol({
  size = 20,
  className,
}: {
  size?: number;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 64 64"
      width={size}
      height={size}
      aria-hidden="true"
      className={cn("shrink-0", className)}
    >
      <circle
        cx="32"
        cy="32"
        r="22"
        fill="none"
        className="stroke-ink"
        strokeWidth="4"
      />
      <polyline
        points="2,34 20,34 25,34 29,14 35,50 39,26 42,34 62,34"
        fill="none"
        className="stroke-signal-ink"
        strokeWidth="4"
        strokeLinecap="square"
        strokeLinejoin="miter"
      />
    </svg>
  );
}

/** Wordmark: the symbol and "riskos" in lowercase Instrument Sans 600 at -0.03em. */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5", className)}>
      <Symbol size={20} />
      <span className="text-[17px] leading-none font-semibold tracking-[-0.03em]">
        riskos
      </span>
      <span className="sr-only">RiskOS</span>
    </span>
  );
}
