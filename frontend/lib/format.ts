/** Formatting of readings: numbers always with units, mono in the UI. */

const numberFormat = new Intl.NumberFormat("en-GB");

export function formatCount(n: number): string {
  return numberFormat.format(n);
}

/** Minor units to a currency amount with its code, e.g. 450000 NGN → "4,500.00 NGN". */
export function formatAmount(minor: number, currency: string): string {
  let digits = 2;
  try {
    digits =
      new Intl.NumberFormat("en", {
        style: "currency",
        currency,
      }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    digits = 2;
  }
  const major = minor / 10 ** digits;
  return `${new Intl.NumberFormat("en-GB", {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(major)} ${currency}`;
}

/** Compact age: "12s", "3m", "4h", "2d". */
export function formatAge(iso: string, now: Date = new Date()): string {
  const s = Math.max(
    0,
    Math.round((now.getTime() - new Date(iso).getTime()) / 1000),
  );
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

/** Absolute UTC timestamp, e.g. "2026-09-30 14:02:11 UTC". */
export function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return `${d.toISOString().slice(0, 19).replace("T", " ")} UTC`;
}

/** "snake_case" identifiers to sentence case for display. */
export function humanize(value: string): string {
  const s = value.replaceAll("_", " ").replaceAll(".", " ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}
