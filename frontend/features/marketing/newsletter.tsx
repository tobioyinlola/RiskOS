"use client";

import { useId, useState } from "react";

import { Button } from "@/components/ui/button";

/**
 * The newsletter field. There is no mailing list service yet, so submitting says so plainly
 * instead of pretending to subscribe; wire `onSubmit` to the service when it exists.
 */
export function Newsletter() {
  const id = useId();
  const [sent, setSent] = useState(false);
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        setSent(true);
      }}
    >
      <label htmlFor={id} className="sr-only">
        Email address
      </label>
      <div className="flex items-center gap-1 rounded-full border border-border-control bg-bg-surface p-1 pl-4 focus-within:border-accent focus-within:shadow-[0_0_0_4px_var(--blue-100)]">
        <input
          id={id}
          type="email"
          required
          placeholder="you@company.com"
          className="min-w-0 flex-1 bg-transparent text-body outline-none placeholder:text-text-muted"
        />
        <Button type="submit" variant="primary" size="sm" arrow>
          Sign up
        </Button>
      </div>
      <p role="status" className="mt-2 min-h-5 text-label text-text-muted">
        {sent
          ? "Thanks. The newsletter isn't live yet, so nothing was sent."
          : "Product updates, now and then."}
      </p>
    </form>
  );
}
