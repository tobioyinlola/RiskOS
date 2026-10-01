"use client";

import { cn } from "cn";
import Link from "next/link";

import type { EntityKind } from "@/lib/queries";

export function entityHref(kind: EntityKind, ref: string) {
  return `/entities/${kind}/${encodeURIComponent(ref)}` as "/entities";
}

/** A user, account or device reference that opens its profile. */
export function EntityLink({
  kind,
  refId,
  className,
}: {
  kind: EntityKind;
  refId: string;
  className?: string;
}) {
  return (
    <Link
      href={entityHref(kind, refId)}
      onClick={(e) => e.stopPropagation()}
      className={cn(
        "mono text-data hover:text-accent hover:underline",
        className,
      )}
      title={`Open the ${kind} profile`}
    >
      {refId}
    </Link>
  );
}
