import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "cn";
import { Slot } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";

/**
 * Button (docs/design-system.md, Components → Button).
 * - Primary: accent fill, one per view, for the main forward action.
 * - Secondary: border-control edge, for queue actions, with their shortcut.
 * - Danger: status-danger outline, for irreversible actions only, always behind a confirmation
 *   that names the object.
 * - Ghost: a quiet secondary action in toolbars and menus.
 */
export const buttonVariants = cva(
  "inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border font-medium select-none transition-colors duration-150 ease-out disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-icon-sm [&_svg]:shrink-0 [&_svg]:stroke-[1.5]",
  {
    variants: {
      variant: {
        primary:
          "border-transparent bg-accent text-on-accent hover:bg-accent-hover",
        secondary:
          "border-border-control bg-transparent text-text-primary hover:bg-bg-raised",
        danger:
          "border-status-danger bg-transparent text-status-danger hover:bg-risk-critical-bg",
        ghost:
          "border-transparent bg-transparent text-text-secondary hover:bg-bg-raised hover:text-text-primary",
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
      title={shortcut && !props.title ? `Shortcut: ${shortcut.toUpperCase()}` : props.title}
      {...props}
    >
      {asChild ? (
        children
      ) : (
        <>
          {icon}
          {children}
          {shortcut ? (
            <span className="mono text-label font-normal text-text-muted">
              {shortcut.toUpperCase()}
            </span>
          ) : null}
        </>
      )}
    </Comp>
  );
}
