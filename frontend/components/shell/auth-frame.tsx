import Link from "next/link";
import type { ReactNode } from "react";

import { StatusPill } from "@/components/ui/pill";
import { RiskBadge } from "@/components/ui/risk-badge";
import { Wordmark } from "@/components/ui/wordmark";

/**
 * Sign-in and sign-up pages (console and staff): the form sits on the page itself, not in a
 * card. On wide screens the right half is a brand panel on the landing wash, with the signature
 * headline and a few floating pieces of the product. The panel is decorative.
 */
export function AuthFrame({
  tag,
  title = "Every decision,",
  accent = "explained.",
  children,
}: {
  tag: ReactNode;
  title?: string;
  accent?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-dvh bg-bg-surface">
      <div className="flex min-w-0 flex-1 flex-col px-6 py-6 sm:px-10">
        <header className="flex items-center justify-between gap-3">
          <Link href="/home" aria-label="RiskOS home">
            <Wordmark />
          </Link>
          <span className="rounded-full border border-border-strong px-3 py-1 text-label whitespace-nowrap text-text-secondary">
            {tag}
          </span>
        </header>
        <main className="flex flex-1 items-center justify-center py-10">
          <div className="w-full max-w-[400px]">{children}</div>
        </main>
        <footer className="text-label text-text-muted">
          © {new Date().getFullYear()} RiskOS · Explainable risk decisions
        </footer>
      </div>

      <aside
        aria-hidden="true"
        className="bg-hero relative m-3 hidden w-[44%] max-w-[640px] overflow-hidden rounded-xl lg:flex lg:flex-col"
      >
        <div className="bg-grid fade-radial pointer-events-none absolute inset-0" />
        <div className="relative px-12 pt-16">
          <p className="max-w-md text-display">
            {title} <span className="accent-italic">{accent}</span>
          </p>
          <p className="mt-4 max-w-sm text-body-lg text-text-secondary">
            A score, a band and a decision for every event, with the signals
            behind it.
          </p>
        </div>
        <div className="relative mt-auto h-[340px]">
          <div className="absolute top-6 left-12 w-60 rounded-md border border-border-subtle bg-bg-surface p-4 shadow-overlay">
            <p className="text-label text-text-muted">Mean risk score</p>
            <p className="text-figure-xl">
              23.<span className="text-text-faint">4</span>
            </p>
            <svg viewBox="0 0 200 48" className="mt-2 h-12 w-full">
              <path
                d="M0 36 C 30 30, 45 12, 75 18 S 125 40, 150 26 S 185 8, 200 14"
                fill="none"
                stroke="var(--chart-1)"
                strokeWidth="2.5"
              />
              <path
                d="M0 44 C 30 42, 50 34, 80 37 S 130 44, 155 38 S 185 30, 200 34"
                fill="none"
                stroke="var(--chart-2)"
                strokeWidth="2.5"
              />
            </svg>
          </div>
          <div className="absolute top-24 right-10 w-56 rounded-md border border-border-subtle bg-bg-surface p-4 shadow-overlay">
            <p className="text-label text-text-muted">Open alert</p>
            <p className="mt-1 text-body-sm font-semibold">
              Card testing burst
            </p>
            <div className="mt-2">
              <RiskBadge score={96} />
            </div>
          </div>
          <div className="absolute bottom-10 left-24 flex w-64 items-center justify-between rounded-md border border-border-subtle bg-bg-surface p-4 shadow-overlay">
            <span className="text-body-sm font-semibold">Decision</span>
            <StatusPill tone="info">Step-up</StatusPill>
          </div>
        </div>
      </aside>
    </div>
  );
}
