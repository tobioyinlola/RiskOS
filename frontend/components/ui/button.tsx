import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";
import { Slot } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";

import { ArrowUpRight } from "./icons";

/**
 * Button (docs/design-system.md, Components → Button). Buttons are pills.
 * - Primary: the ink pill with an inner highlight, one per view, for the main forward action.
 *   Add `arrow` for actions that go somewhere (↗).
 * - Secondary: white with a 1px edge and, usually, a leading icon.
 * - Ghost: a quiet action in toolbars and menus.
 * - Accent: brand blue, for the rare action that must stand apart from the primary one.
 * - Danger: critical outline, for irreversible actions only, always behind a confirmation that
 *   names the object.
 */
export const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 rounded-full border font-medium whitespace-nowrap select-none transition-[background-color,border-color,color,box-shadow] duration-150 ease-out disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-icon-sm [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        primary:
          "border-transparent bg-ink text-white shadow-primary hover:bg-ink-hover",
        secondary:
          "border-border-strong bg-bg-surface text-text-primary hover:border-border-control hover:bg-bg-raised",
        ghost:
          "border-transparent bg-transparent text-text-secondary hover:bg-bg-sunken hover:text-text-primary",
        accent:
          "border-transparent bg-accent text-on-accent hover:bg-accent-hover",
        danger:
          "border-status-danger bg-bg-surface text-status-danger hover:bg-status-danger-bg",
      },
      size: {
        sm: "h-control-sm px-3.5 text-body-sm",
        md: "h-control-md px-4 text-body",
        lg: "h-12 px-6 text-body-lg",
        icon: "size-control-sm p-0",
        "icon-md": "size-control-md p-0 [&_svg]:size-icon-md",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps
  extends ComponentProps<"button">, VariantProps<typeof buttonVariants> {
  shortcut?: string;
  /** A leading icon. */
  icon?: ReactNode;
  /** A trailing ↗ for actions that go somewhere. */
  arrow?: boolean;
  asChild?: boolean;
}

export function Button({
  className,
  variant,
  size,
  shortcut,
  icon,
  arrow,
  asChild,
  children,
  type = "button",
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      className={cn(buttonVariants({ variant, size }), className)}
      type={asChild ? undefined : type}
      aria-keyshortcuts={shortcut}
      title={
        shortcut && !props.title
          ? `Shortcut: ${shortcut.toUpperCase()}`
          : props.title
      }
      {...props}
    >
      {icon}
      <Slot.Slottable>{children}</Slot.Slottable>
      {arrow ? <ArrowUpRight aria-hidden="true" /> : null}
      {shortcut ? (
        <span
          className={cn(
            "mono text-label font-normal",
            variant === "primary" || variant === "accent"
              ? "text-white/70"
              : "text-text-muted",
          )}
        >
          {shortcut.toUpperCase()}
        </span>
      ) : null}
    </Comp>
  );
}
