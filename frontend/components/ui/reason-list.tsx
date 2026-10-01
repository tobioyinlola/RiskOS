"use client";

import { useState } from "react";

import { Button } from "./button";

export interface Reason {
  label: string;
  category: string;
  /** Signed points the factor moved the score. */
  contribution: number;
}

/**
 * ReasonList (docs/design-system.md): the factors behind a score, ranked by contribution.
 * ▲ raises risk, ▼ lowers it; the arrow carries the meaning, color only reinforces it.
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
      <h3 className="mb-2 text-heading-3">{title}</h3>
      {ranked.length === 0 ? (
        <p className="border-t border-border-subtle py-2 text-body-sm text-text-secondary">
          No factor moved this score.
        </p>
      ) : (
        <ul>
          {shown.map((r) => {
            const up = r.contribution >= 0;
            const width = Math.max(4, (Math.abs(r.contribution) / max) * 100);
            return (
              <li
                key={`${r.label}-${r.category}`}
                className="grid grid-cols-[16px_1fr_84px_40px] items-center gap-2 border-t border-border-subtle py-1.5"
              >
                <span
                  className={`mono font-medium ${up ? "text-risk-high" : "text-risk-low"}`}
                  aria-label={up ? "raises risk" : "lowers risk"}
                >
                  {up ? "▲" : "▼"}
                </span>
                <span className="text-body-sm">
                  {r.label}
                  <small className="block text-label font-normal text-text-muted">
                    {r.category}
                  </small>
                </span>
                <span
                  className="h-1.5 overflow-hidden rounded-full bg-bg-sunken"
                  aria-hidden="true"
                >
                  <i
                    className="block h-full bg-text-secondary"
                    style={{ width: `${width}%` }}
                  />
                </span>
                <span className="mono text-right text-label text-text-secondary">
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
