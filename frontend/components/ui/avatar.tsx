import { cn } from "@/lib/cn";
import type { ReactNode } from "react";

/** Initials of a name or email: "Ada Lovelace" → "AL", "ops@acme.io" → "O". */
export function initials(name: string): string {
  const words = name
    .replace(/@.*/, "")
    .split(/[\s._-]+/)
    .filter(Boolean);
  return (
    (words[0]?.[0] ?? "?") + (words.length > 1 ? (words.at(-1)?.[0] ?? "") : "")
  ).toUpperCase();
}

const TINTS = [
  "bg-blue-100 text-blue-600",
  "bg-risk-elevated-bg text-risk-elevated",
  "bg-status-success-bg text-status-success",
  "bg-risk-high-bg text-risk-high",
  "bg-risk-minimal-bg text-risk-minimal",
];

/** A round avatar: initials on a soft tint chosen from the name, or an icon. */
export function Avatar({
  name,
  icon,
  size = "md",
  className,
}: {
  name: string;
  icon?: ReactNode;
  size?: "sm" | "md" | "lg";
  className?: string;
}) {
  const tint =
    TINTS[[...name].reduce((s, c) => s + c.charCodeAt(0), 0) % TINTS.length];
  return (
    <span
      aria-hidden="true"
      title={name}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full font-semibold ring-2 ring-bg-surface select-none",
        size === "sm" && "size-[28px] text-label [&_svg]:size-3.5",
        size === "md" && "size-9 text-body-sm [&_svg]:size-icon-sm",
        size === "lg" && "size-11 text-body [&_svg]:size-icon-md",
        tint,
        className,
      )}
    >
      {icon ?? initials(name)}
    </span>
  );
}

/** Overlapping avatars with an optional caption. */
export function AvatarStack({
  people,
  size = "md",
  caption,
  className,
}: {
  people: { name: string; icon?: ReactNode }[];
  size?: "sm" | "md" | "lg";
  caption?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <div className="flex -space-x-2.5">
        {people.map((p) => (
          <Avatar key={p.name} name={p.name} icon={p.icon} size={size} />
        ))}
      </div>
      {caption ? (
        <span className="text-body-sm text-text-secondary">{caption}</span>
      ) : null}
    </div>
  );
}
