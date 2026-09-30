"use client";

import { cn } from "cn";
import { X } from "lucide-react";
import { Dialog as D } from "radix-ui";
import type { ReactNode } from "react";

/**
 * The right drawer (480px) that opens over a queue so the analyst keeps their place
 * (docs/design-system.md, Grid and spacing). Non-modal to the page behind it visually, but it
 * traps focus while open.
 */
export function Drawer({
  open,
  onOpenChange,
  title,
  subtitle,
  children,
  footer,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  subtitle?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <D.Root open={open} onOpenChange={onOpenChange}>
      <D.Portal>
        <D.Content
          className="fixed inset-y-0 right-0 z-40 flex w-full max-w-drawer flex-col border-l border-line-subtle bg-bg-surface shadow-overlay outline-none"
          aria-describedby={undefined}
        >
          <header className="flex items-start justify-between gap-4 border-b border-line-subtle px-6 py-4">
            <div className="min-w-0">
              <D.Title className="text-heading">{title}</D.Title>
              {subtitle ? (
                <div className="mt-1 text-body-sm text-ink-3">{subtitle}</div>
              ) : null}
            </div>
            <D.Close
              className="rounded-md p-1 text-ink-3 hover:bg-bg-raised hover:text-ink"
              aria-label="Close"
            >
              <X className="size-4 stroke-[1.5]" />
            </D.Close>
          </header>
          <div className="flex-1 overflow-y-auto px-6 py-5">{children}</div>
          {footer ? (
            <footer className="flex flex-wrap items-center gap-2 border-t border-line-subtle px-6 py-3">
              {footer}
            </footer>
          ) : null}
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}

/** A centred modal for confirmations and short forms. */
export function Modal({
  open,
  onOpenChange,
  title,
  description,
  children,
  className,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <D.Root open={open} onOpenChange={onOpenChange}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-50 bg-bg-canvas/70" />
        <D.Content
          className={cn(
            "fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-line-subtle bg-bg-surface p-6 shadow-overlay outline-none",
            className,
          )}
          aria-describedby={description ? undefined : undefined}
        >
          <D.Title className="text-heading">{title}</D.Title>
          {description ? (
            <D.Description className="mt-1 text-body text-ink-2">
              {description}
            </D.Description>
          ) : null}
          <div className="mt-4">{children}</div>
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}
