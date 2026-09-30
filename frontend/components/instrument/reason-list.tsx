"use client";

import { useState } from "react";

import { Button } from "./button";

export interface Reason {
  label: string;
  category: string;
  /** Signed points the factor moved the score. */
  contribution: number;
}

const TICKS = 12;

/**
 * ReasonList (docs/design-system.md): the factors behind a score, ranked by contribution and
 * measured in ticks. ▲ raises risk, ▼ lowers it.
 */
export function ReasonList({
  reasons,
  limit = 5,
  title = "Why this score",
}: {
  reasons: Reason[];
  limit?: number;
  title?: string;
}) {
  const [all, setAll] = useState(false);
  const ranked = [...reasons].sort(
    (a, b) => Math.abs(b.contribution) - Math.abs(a.contribution),
  );
  const max = Math.max(1, ...ranked.map((r) => Math.abs(r.contribution)));
  const shown = all ? ranked : ranked.slice(0, limit);
  return (
    <section aria-label={title}>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-heading">{title}</h3>
        <span className="legend">Contribution</span>
      </div>
      {ranked.length === 0 ? (
        <p className="border-t border-line-subtle py-2 text-body-sm text-ink-2">
          No factor moved this score.
        </p>
      ) : (
        <ul>
          {shown.map((r) => {
            const up = r.contribution >= 0;
            const on = Math.max(
              1,
              Math.round((Math.abs(r.contribution) / max) * TICKS),
            );
            return (
              <li
                key={`${r.label}-${r.category}`}
                className="grid grid-cols-[18px_1fr_96px_40px] items-center gap-2 border-t border-line-subtle py-[7px]"
              >
                <span
                  className={`mono text-body-sm font-medium ${up ? "text-risk-high" : "text-risk-low"}`}
                  aria-label={up ? "raises risk" : "lowers risk"}
                >
                  {up ? "▲" : "▼"}
                </span>
                <span className="text-body-sm">
                  {r.label}
                  <small className="legend mt-px block">{r.category}</small>
                </span>
                <span
                  className="flex h-2.5 items-stretch gap-0.5"
                  aria-hidden="true"
                >
                  {Array.from({ length: TICKS }, (_, i) => (
                    <i
                      key={i}
                      className={`flex-1 ${i < on ? "bg-ink-2" : "bg-line-subtle"}`}
                    />
                  ))}
                </span>
                <span className="mono text-right text-label text-ink-2">
                  {up ? "+" : "−"}
                  {Math.abs(r.contribution)}
                </span>
              </li>
            );
          })}
        </ul>
      )}
      {ranked.length > limit ? (
        <Button
          variant="ghost"
          size="sm"
          className="mt-2"
          onClick={() => setAll((v) => !v)}
        >
          {all ? "Show fewer" : `Show all ${ranked.length}`}
        </Button>
      ) : null}
    </section>
  );
}
