import { cn } from "@/lib/cn";
import type { ReactNode } from "react";

import { SegmentedBar, TickScoreBar } from "@/components/ui/bars";
import { Figure } from "@/components/ui/figure";
import {
  Alert,
  Bell,
  Chart,
  Code,
  Folder,
  Home,
  Live,
  Search,
  ShieldCheck,
} from "@/components/ui/icons";
import { DecisionPill, StatusPill } from "@/components/ui/pill";
import { RiskBadge } from "@/components/ui/risk-badge";
import { Monogram } from "@/components/ui/wordmark";

/*
 * Illustrations of the product for the landing page, drawn with the real components. They are
 * pictures, not data: every figure in them is illustrative and marked aria-hidden.
 */

const CHART_A =
  "M0 70 C 40 60, 60 30, 100 38 S 160 70, 200 52 S 260 20, 300 30 S 360 58, 400 40";
const CHART_B =
  "M0 88 C 40 84, 70 70, 100 74 S 160 86, 200 78 S 260 60, 300 66 S 360 80, 400 70";

function MiniChart({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 400 100"
      preserveAspectRatio="none"
      className={cn("h-28 w-full", className)}
      aria-hidden="true"
    >
      {[20, 45, 70, 95].map((y) => (
        <line
          key={y}
          x1="0"
          x2="400"
          y1={y}
          y2={y}
          stroke="var(--chart-grid)"
          strokeDasharray="2 4"
        />
      ))}
      <path d={CHART_A} fill="none" stroke="var(--chart-1)" strokeWidth="2.5" />
      <path d={CHART_B} fill="none" stroke="var(--chart-2)" strokeWidth="2.5" />
    </svg>
  );
}

/** The console overview, as a large hero picture. */
export function DashboardPreview() {
  const nav = [
    { icon: Home, label: "Overview", active: true },
    { icon: Live, label: "Live monitor" },
    { icon: Alert, label: "Alerts" },
    { icon: Folder, label: "Cases" },
    { icon: Chart, label: "Analytics" },
    { icon: ShieldCheck, label: "Rules" },
    { icon: Code, label: "Developer" },
  ];
  return (
    <div
      aria-hidden="true"
      className="flex overflow-hidden rounded-xl border border-border-subtle bg-bg-canvas text-left shadow-overlay select-none"
    >
      <div className="hidden w-48 shrink-0 flex-col gap-1 border-r border-border-subtle bg-bg-surface p-3 md:flex">
        <div className="mb-3 flex items-center gap-2 p-1">
          <Monogram name="Acme Payments" className="size-7" />
          <span className="text-body-sm font-semibold">Acme Payments</span>
        </div>
        {nav.map((n) => (
          <span
            key={n.label}
            className={cn(
              "flex items-center gap-2 rounded-full px-3 py-2 text-body-sm",
              n.active
                ? "bg-blue-50 font-medium text-accent"
                : "text-text-secondary",
            )}
          >
            <n.icon weight={n.active ? "fill" : "regular"} className="size-4" />
            {n.label}
          </span>
        ))}
      </div>
      <div className="min-w-0 flex-1 p-5">
        <div className="mb-4 flex items-center justify-between gap-3">
          <div>
            <p className="text-label text-text-muted">Thursday, 1 October</p>
            <p className="text-heading-2 font-bold">Welcome back, Ada!</p>
          </div>
          <div className="flex items-center gap-2">
            <span className="hidden h-8 w-44 items-center gap-2 rounded-full border border-border-strong bg-bg-surface px-3 text-label text-text-muted sm:flex">
              <Search className="size-3.5" /> Search
            </span>
            <span className="flex size-8 items-center justify-center rounded-full border border-border-strong bg-bg-surface">
              <Bell className="size-3.5" />
            </span>
          </div>
        </div>
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_220px]">
          <div className="rounded-lg border border-border-subtle bg-bg-surface p-4 shadow-card">
            <p className="text-label text-text-secondary">Mean risk score</p>
            <p className="text-figure-xl">
              <Figure value="23.4" />
            </p>
            <MiniChart />
          </div>
          <div className="flex flex-col gap-4">
            <div className="rounded-lg border border-border-subtle bg-bg-surface p-4 shadow-card">
              <p className="mb-2 text-label text-text-secondary">
                Average risk
              </p>
              <TickScoreBar value={23} ticks={28} label="Average risk">
                <Figure value="23.4" />
              </TickScoreBar>
            </div>
            <div className="rounded-lg border border-border-subtle bg-bg-surface p-4 shadow-card">
              <p className="mb-2 text-label text-text-secondary">
                Interventions
              </p>
              <SegmentedBar
                parts={[
                  { key: "a", value: 70, fill: "bg-blue-200", label: "Allow" },
                  {
                    key: "s",
                    value: 12,
                    fill: "bg-blue-300",
                    label: "Step-up",
                  },
                  {
                    key: "r",
                    value: 12,
                    fill: "bg-brand-blue",
                    label: "Review",
                  },
                  { key: "b", value: 6, fill: "bg-blue-600", label: "Block" },
                ]}
              />
            </div>
          </div>
        </div>
        <div className="mt-4 rounded-lg border border-border-subtle bg-bg-surface shadow-card">
          {[
            ["txn-8841", 94, "block"],
            ["txn-8840", 63, "review"],
            ["login-2213", 12, "allow"],
          ].map(([key, score, decision]) => (
            <div
              key={key}
              className="flex items-center gap-4 border-t border-border-subtle px-4 py-3 first:border-t-0"
            >
              <span className="mono w-24 text-data">{key}</span>
              <RiskBadge score={score as number} />
              <span className="ml-auto">
                <DecisionPill decision={decision as string} />
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

/** The mobile overview, in a phone frame. */
export function PhonePreview() {
  return (
    <div
      aria-hidden="true"
      className="relative w-64 rounded-[44px] border-[10px] border-ink bg-bg-canvas p-4 shadow-overlay select-none"
    >
      <div className="mx-auto mb-4 h-5 w-20 rounded-full bg-ink" />
      <p className="text-label text-text-muted">Today</p>
      <p className="mb-3 text-heading-3 font-bold">Risk at a glance</p>
      <div className="rounded-lg border border-border-subtle bg-bg-surface p-3 shadow-card">
        <p className="text-label text-text-secondary">Mean risk score</p>
        <p className="text-metric">
          <Figure value="23.4" />
        </p>
        <MiniChart className="h-16" />
      </div>
      <div className="mt-3 flex flex-col gap-2">
        {[
          ["Card testing burst", "critical", 96],
          ["New device, large payout", "high", 78],
        ].map(([title, band, score]) => (
          <div
            key={title as string}
            className="flex items-center gap-2 rounded-md border border-border-subtle bg-bg-surface p-2.5"
          >
            <span className="min-w-0 flex-1 truncate text-label font-medium">
              {title}
            </span>
            <RiskBadge score={score as number} showScore={false} />
            <span className="sr-only">{band}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** A small floating UI card for collages. */
export function FloatCard({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "rounded-md border border-border-subtle bg-bg-surface p-3 shadow-overlay select-none",
        className,
      )}
    >
      {children}
    </div>
  );
}

/** Snippets for the feature cards. */
export const SNIPPETS: Record<string, ReactNode> = {
  scoring: (
    <FloatCard className="w-full max-w-xs">
      <div className="flex items-baseline justify-between">
        <span className="text-figure-xl text-risk-high">78</span>
        <RiskBadge score={78} showScore={false} />
      </div>
      <SegmentedBar
        className="mt-3"
        parts={[
          {
            key: "m",
            value: 20,
            fill: "bg-risk-minimal-fill",
            label: "Minimal",
          },
          { key: "l", value: 30, fill: "bg-risk-low-fill", label: "Low" },
          {
            key: "e",
            value: 20,
            fill: "bg-risk-elevated-fill",
            label: "Elevated",
          },
          { key: "h", value: 20, fill: "bg-risk-high-fill", label: "High" },
          {
            key: "c",
            value: 10,
            fill: "bg-risk-critical-fill",
            label: "Critical",
          },
        ]}
      />
      <p className="mt-3 text-label text-text-secondary">
        Decision: <b className="text-text-primary">Step-up</b>
      </p>
    </FloatCard>
  ),
  signals: (
    <FloatCard className="w-full max-w-xs">
      {[
        ["New device for this user", "+22"],
        ["Beneficiary added today", "+18"],
        ["Amount 6× the user's median", "+15"],
      ].map(([label, w]) => (
        <div
          key={label}
          className="flex items-center justify-between gap-3 border-t border-border-subtle py-2 text-label first:border-t-0 first:pt-0"
        >
          <span>{label}</span>
          <span className="font-semibold text-risk-high">{w}</span>
        </div>
      ))}
    </FloatCard>
  ),
  alerts: (
    <FloatCard className="w-full max-w-xs">
      {[
        ["Card testing burst", "critical"],
        ["Velocity on payouts", "high"],
      ].map(([t, b]) => (
        <div
          key={t}
          className="flex items-center justify-between gap-3 border-t border-border-subtle py-2 text-label first:border-t-0 first:pt-0"
        >
          <span className="truncate">{t}</span>
          <StatusPill tone={b === "critical" ? "danger" : "warning"}>
            {b === "critical" ? "Open" : "Investigating"}
          </StatusPill>
        </div>
      ))}
    </FloatCard>
  ),
  rules: (
    <FloatCard className="w-full max-w-xs">
      <p className="text-label font-semibold">Large transfer</p>
      <p className="mono mt-1 text-label text-text-muted">
        transaction.amount ≥ 1,000,000
      </p>
      <div className="mt-2 flex gap-2">
        <StatusPill tone="info">Shadow</StatusPill>
        <StatusPill tone="success">Approved</StatusPill>
      </div>
    </FloatCard>
  ),
  api: (
    <FloatCard className="mono w-full max-w-xs bg-ink text-label text-white">
      <span className="text-blue-300">POST</span> /v1/events
      <br />
      <span className="text-white/60">{"{"} &quot;decision&quot;: </span>
      <span className="text-blue-200">&quot;review&quot;</span>
      <span className="text-white/60">, &quot;score&quot;: 63 {"}"}</span>
    </FloatCard>
  ),
};
