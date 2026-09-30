import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Slot } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";

import { Kbd } from "./kbd";

/**
 * Button (docs/design-system.md, Components → Button). Primary: one per view, the forward action.
 * Secondary: queue actions, with their shortcut. Danger: irreversible actions only, always
 * followed by a confirmation that names the object.
 */
export const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border font-medium select-none transition-[background-color,filter] duration-150 ease-instrument disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:stroke-[1.5]",
  {
    variants: {
      variant: {
        primary:
          "border-transparent bg-primary-fill text-on-primary hover:brightness-110",
        secondary:
          "border-line-control bg-transparent text-ink hover:bg-bg-raised",
        danger:
          "border-status-error bg-transparent text-status-error hover:bg-risk-critical-wash",
        ghost:
          "border-transparent bg-transparent text-ink-2 hover:bg-bg-raised hover:text-ink",
      },
      size: {
        sm: "h-control-sm px-3 text-body-sm",
        md: "h-control-md px-4 text-body",
        icon: "size-control-sm p-0",
      },
    },
    defaultVariants: { variant: "secondary", size: "md" },
  },
);

export interface ButtonProps
  extends ComponentProps<"button">, VariantProps<typeof buttonVariants> {
  shortcut?: string;
  icon?: ReactNode;
  asChild?: boolean;
}

export function Button({
  className,
  variant,
  size,
  shortcut,
  icon,
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
      {...props}
    >
      {asChild ? (
        children
      ) : (
        <>
          {icon}
          {children}
          {shortcut ? <Kbd>{shortcut.toUpperCase()}</Kbd> : null}
        </>
      )}
    </Comp>
  );
}
