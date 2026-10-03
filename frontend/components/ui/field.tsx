import { cn } from "@/lib/cn";
import { useId, type ComponentProps, type ReactNode } from "react";

const control =
  "w-full rounded-md border border-border-control bg-bg-surface px-3.5 text-body text-text-primary transition-[border-color,box-shadow] duration-150 placeholder:text-text-muted hover:border-text-secondary focus-visible:border-accent focus-visible:shadow-[0_0_0_4px_var(--blue-100)] focus-visible:outline-none disabled:opacity-40 aria-invalid:border-status-danger";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input className={cn(control, "h-control-md", className)} {...props} />
  );
}

export function Textarea({ className, ...props }: ComponentProps<"textarea">) {
  return (
    <textarea className={cn(control, "min-h-24 py-2", className)} {...props} />
  );
}

export function Select({ className, ...props }: ComponentProps<"select">) {
  return (
    <select
      className={cn(control, "h-control-md pr-7", className)}
      {...props}
    />
  );
}

/** A labelled form field; the control receives the id and error wiring through a render prop. */
export function Field({
  label,
  hint,
  error,
  children,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: string;
  children: (props: {
    id: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
  }) => ReactNode;
  className?: string;
}) {
  const id = useId();
  const hintId = hint ? `${id}-hint` : undefined;
  const errorId = error ? `${id}-error` : undefined;
  const describedBy = [hintId, errorId].filter(Boolean).join(" ") || undefined;
  return (
    <div className={cn("flex flex-col gap-1", className)}>
      <label htmlFor={id} className="text-label text-text-primary">
        {label}
      </label>
      {children({
        id,
        "aria-describedby": describedBy,
        "aria-invalid": error ? true : undefined,
      })}
      {hint ? (
        <p id={hintId} className="text-body-sm text-text-muted">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p
          id={errorId}
          className="text-body-sm text-status-danger"
          role="alert"
        >
          {error}
        </p>
      ) : null}
    </div>
  );
}
