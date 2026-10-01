import type { ReactNode } from "react";

import { Wordmark } from "@/components/ui/wordmark";

/** Sign-in pages: one panel on the canvas. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-bg-canvas px-4 py-7">
      <div className="w-full max-w-md rounded-lg border border-border-subtle bg-bg-surface p-6 shadow-overlay">
        <div className="mb-6 flex items-center justify-between">
          <Wordmark />
          <span className="text-label text-text-muted">
            Explainable risk decisions
          </span>
        </div>
        {children}
      </div>
    </main>
  );
}
