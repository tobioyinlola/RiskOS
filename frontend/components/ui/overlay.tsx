"use client";

import { cn } from "cn";
import { X } from "lucide-react";
import { Dialog as D } from "radix-ui";
import type { ReactNode } from "react";

/**
 * The right drawer (480px) that opens over a queue so the analyst keeps their place
 * (docs/design-system.md, Spacing and layout). It traps focus while open; Escape closes it.
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
          className="fixed inset-y-0 right-0 z-40 flex w-full max-w-drawer flex-col border-l border-border-subtle bg-bg-surface shadow-overlay outline-none"
          style={{ animation: "slide-in-right 180ms ease-out" }}
          aria-describedby={undefined}
        >
          <header className="flex items-start justify-between gap-4 border-b border-border-subtle px-5 py-4">
            <div className="min-w-0">
              <D.Title className="text-heading-2">{title}</D.Title>
              {subtitle ? (
                <div className="mt-1 text-body-sm text-text-muted">
                  {subtitle}
                </div>
              ) : null}
            </div>
            <D.Close
              className="rounded-md p-1 text-text-muted hover:bg-bg-raised hover:text-text-primary"
              aria-label="Close"
            >
              <X className="size-icon-sm stroke-[1.5]" />
            </D.Close>
          </header>
          <div className="flex-1 overflow-y-auto px-5 py-5">{children}</div>
          {footer ? (
            <footer className="flex flex-wrap items-center gap-2 border-t border-border-subtle px-5 py-3">
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
        <D.Overlay
          className="fixed inset-0 z-50 bg-bg-canvas/70"
          style={{ animation: "fade-in 150ms ease-out" }}
        />
        <D.Content
          className={cn(
            "fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border-subtle bg-bg-surface p-5 shadow-overlay outline-none",
            className,
          )}
          style={{ animation: "fade-in 150ms ease-out" }}
          aria-describedby={undefined}
        >
          <D.Title className="text-heading-2">{title}</D.Title>
          {description ? (
            <D.Description className="mt-1 text-body text-text-secondary">
              {description}
            </D.Description>
          ) : null}
          <div className="mt-4">{children}</div>
        </D.Content>
      </D.Portal>
    </D.Root>
  );
}

/**
 * A confirmation step for a danger action. It names the object and the consequence; the
 * confirm button repeats the verb.
 */
export function Confirm({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  onConfirm,
  busy,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: ReactNode;
  description: ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
  busy?: boolean;
}) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
    >
      <div className="flex justify-end gap-2">
        <button
          type="button"
          className="inline-flex h-control-md items-center rounded-md border border-border-control px-4 text-body text-text-primary hover:bg-bg-raised"
          onClick={() => onOpenChange(false)}
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={busy}
          className="inline-flex h-control-md items-center rounded-md border border-status-danger px-4 text-body font-medium text-status-danger hover:bg-risk-critical-bg disabled:opacity-40"
          onClick={onConfirm}
        >
          {confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
