"use client";

import { Tooltip as T } from "radix-ui";
import type { ReactNode } from "react";

export const TooltipProvider = T.Provider;

/** A short label on hover or focus (collapsed navigation, icon buttons). */
export function Tooltip({
  content,
  side = "right",
  children,
  disabled,
}: {
  content: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  children: ReactNode;
  disabled?: boolean;
}) {
  if (disabled) return <>{children}</>;
  return (
    <T.Root delayDuration={200}>
      <T.Trigger asChild>{children}</T.Trigger>
      <T.Portal>
        <T.Content
          side={side}
          sideOffset={8}
          className="z-50 rounded-sm bg-ink px-2.5 py-1.5 text-label text-white shadow-overlay"
          style={{ animation: "fade-in 120ms ease-out" }}
        >
          {content}
        </T.Content>
      </T.Portal>
    </T.Root>
  );
}
