import type { ReactNode } from "react";

import { Wordmark } from "@/components/instrument/symbol";

/** Sign-in pages: a cover on grid paper, one panel. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <main className="grid-paper flex min-h-dvh items-center justify-center px-4 py-16">
      <div className="w-full max-w-md rounded-lg border border-line-subtle bg-bg-surface p-8 shadow-overlay">
        <div className="mb-8 flex items-center justify-between">
          <Wordmark />
          <span className="legend">Risk, measured.</span>
        </div>
        {children}
      </div>
    </main>
  );
}
