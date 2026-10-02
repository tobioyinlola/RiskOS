"use client";

import { cn } from "@/lib/cn";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";

import {
  SegmentLegend,
  SegmentedBar,
  TickScoreBar,
} from "@/components/ui/bars";
import { Button } from "@/components/ui/button";
import { Figure } from "@/components/ui/figure";
import {
  ArrowDown,
  ArrowUp,
  Ban,
  Chart,
  Info,
  Live,
  Plus,
} from "@/components/ui/icons";
import { SeriesChart, SeriesLegend } from "@/components/ui/line-chart";
import { pillTabClass } from "@/components/ui/misc";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { DECISION_TONE, StatusPill, type Tone } from "@/components/ui/pill";
import { BandIcon, RiskBadge } from "@/components/ui/risk-badge";
import { Empty, ErrorState, SkeletonRows } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { Tooltip } from "@/components/ui/tooltip";
import type { Schemas } from "@/lib/api/client";
import { formatAge, formatCount, humanize } from "@/lib/format";
import { useAlerts, useCases, useEvents } from "@/lib/queries";
import {
  BAND_FILL,
  BANDS,
  bandInfo,
  DECISION_FILL,
  DECISION_LABEL,
  type Decision,
  type RiskBand,
} from "@/lib/risk";
import { useSession } from "@/lib/session";

import { OnboardingBanner } from "../onboarding/onboarding";

const SAMPLE = 100;
const BUCKETS = 12;

type EventRow = Schemas["EventList"]["data"][number];

/**
 * The overview reads the newest events, alerts and cases of this environment. Figures are
 * labelled with their window ("last 100 events"): there is no analytics store behind it, so
 * every block is computed from that sample and says so.
 */
export function Overview() {
  const { can, session } = useSession();
  const events = useEvents(SAMPLE);
  const sample = useMemo(
    () => events.data?.pages[0]?.data ?? [],
    [events.data],
  );
  const stats = useMemo(() => summarize(sample), [sample]);
  const firstName = session.user.name.split(/\s+/)[0] || session.user.name;
  const today = new Intl.DateTimeFormat("en-GB", {
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(new Date());

  return (
    <>
      <PageHeader
        eyebrow={today}
        title={`Welcome back, ${firstName}!`}
        description={`${session.tenant.name}: the newest activity in this environment.`}
      />
      <div className="-mt-2 mb-6 flex flex-wrap items-center gap-2">
        {can("alerts:read") ? (
          <Button variant="primary" arrow asChild>
            <Link href="/alerts">Review alerts</Link>
          </Button>
        ) : null}
        <Button icon={<Live />} asChild>
          <Link href="/live">Live monitor</Link>
        </Button>
        {can("rules:write") ? (
          <Button icon={<Plus />} asChild>
            <Link href="/rules/new">New rule</Link>
          </Button>
        ) : null}
        <Button icon={<Chart />} asChild>
          <Link href="/analytics">Analytics</Link>
        </Button>
        {can("rules:read") ? (
          <Button icon={<Ban />} asChild>
            <Link href="/deny-lists">Deny lists</Link>
          </Button>
        ) : null}
      </div>

      <OnboardingBanner />
      <div className="grid grid-cols-1 gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex min-w-0 flex-col gap-5">
          <OverviewCard
            loading={events.isLoading}
            error={events.isError ? events.error : undefined}
            stats={stats}
          />
          <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
            <DecisionsCard stats={stats} canAddRule={can("rules:write")} />
            <RiskMixCard stats={stats} canDeny={can("rules:read")} />
          </div>
          <ActivityTable
            sample={sample}
            loading={events.isLoading}
            error={events.isError ? events.error : undefined}
            success={events.isSuccess}
          />
        </div>
        <div className="flex min-w-0 flex-col gap-5">
          {can("alerts:read") ? <OpenAlertsCard /> : null}
          <ScoreCard stats={stats} showCases={can("cases:read")} />
        </div>
      </div>
    </>
  );
}

interface Stats {
  n: number;
  mean: number;
  /** Mean score of the newer half minus the older half (positive: risk rising). */
  meanDelta: number | null;
  highRisk: number;
  highDelta: number | null;
  byBand: Record<RiskBand, number>;
  byDecision: Record<Decision, number>;
  buckets: { label: string; events: number; elevated: number }[];
}

function summarize(sample: EventRow[]): Stats {
  const n = sample.length;
  const byBand = Object.fromEntries(BANDS.map((b) => [b.band, 0])) as Record<
    RiskBand,
    number
  >;
  const byDecision: Record<Decision, number> = {
    allow: 0,
    step_up: 0,
    review: 0,
    block: 0,
  };
  for (const e of sample) {
    byBand[e.risk_level as RiskBand] += 1;
    byDecision[e.decision as Decision] += 1;
  }
  const mean = n ? sample.reduce((s, e) => s + e.risk_score, 0) / n : 0;
  const isHigh = (e: EventRow) =>
    e.risk_level === "high" || e.risk_level === "critical";
  const highRisk = sample.filter(isHigh).length;

  // The sample is newest first: compare its newer half with its older half.
  const half = Math.floor(n / 2);
  const newer = sample.slice(0, half);
  const older = sample.slice(half, half * 2);
  const avg = (xs: EventRow[]) =>
    xs.reduce((s, e) => s + e.risk_score, 0) / xs.length;
  const meanDelta = half >= 2 ? avg(newer) - avg(older) : null;
  const highDelta =
    half >= 2
      ? newer.filter(isHigh).length - older.filter(isHigh).length
      : null;

  return {
    n,
    mean,
    meanDelta,
    highRisk,
    highDelta,
    byBand,
    byDecision,
    buckets: bucketize(sample),
  };
}

/** Events per equal time slice across the sample, oldest first. */
function bucketize(sample: EventRow[]): Stats["buckets"] {
  if (sample.length < 2) return [];
  const times = sample.map((e) => Date.parse(e.received_at));
  const lo = Math.min(...times);
  const hi = Math.max(...times);
  const span = Math.max(hi - lo, 1);
  const size = span / BUCKETS;
  const fmt = new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    ...(span > 36 * 3600_000 ? { day: "numeric", month: "short" } : {}),
  });
  const out = Array.from({ length: BUCKETS }, (_, i) => ({
    label: fmt.format(new Date(lo + size * (i + 1))),
    events: 0,
    elevated: 0,
  }));
  sample.forEach((e, i) => {
    const b = Math.min(BUCKETS - 1, Math.floor((times[i]! - lo) / size));
    out[b]!.events += 1;
    if (e.risk_level !== "minimal" && e.risk_level !== "low")
      out[b]!.elevated += 1;
  });
  return out;
}

const pct = (part: number, whole: number) =>
  whole ? Math.round((part / whole) * 100) : 0;

/** A delta chip: for risk, up is bad (critical tone) and down is good (success tone). */
function Delta({ value, unit = "" }: { value: number | null; unit?: string }) {
  if (value === null) return null;
  const rounded = Math.round(value * 10) / 10;
  const up = rounded > 0;
  const flat = rounded === 0;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 rounded-full px-2 py-0.5 text-label",
        flat
          ? "bg-risk-minimal-bg text-risk-minimal"
          : up
            ? "bg-status-danger-bg text-status-danger"
            : "bg-status-success-bg text-status-success",
      )}
    >
      {flat ? null : up ? (
        <ArrowUp className="size-3" aria-hidden="true" />
      ) : (
        <ArrowDown className="size-3" aria-hidden="true" />
      )}
      <span className="sr-only">{up ? "up" : flat ? "unchanged" : "down"}</span>
      {Math.abs(rounded)}
      {unit}
    </span>
  );
}

function OverviewCard({
  loading,
  error,
  stats,
}: {
  loading: boolean;
  error?: unknown;
  stats: Stats;
}) {
  const series = [
    { key: "events", label: "All events", color: "var(--chart-1)" },
    { key: "elevated", label: "Elevated or higher", color: "var(--chart-2)" },
  ];
  const topDecision = (
    Object.entries(stats.byDecision) as [Decision, number][]
  ).sort((a, b) => b[1] - a[1])[0];
  return (
    <Panel>
      <PanelHeader title="Risk overview" legend={`last ${SAMPLE} events`} />
      <div className="grid gap-6 px-5 pb-5 lg:grid-cols-[minmax(0,240px)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <div>
            <p className="text-body-sm text-text-secondary">Mean risk score</p>
            <p className="mt-1 flex items-baseline gap-2">
              <span className="text-figure-xl">
                {stats.n ? <Figure value={stats.mean.toFixed(1)} /> : "—"}
              </span>
              <span className="text-body-sm text-text-muted">/ 100</span>
            </p>
            <p className="mt-2 flex items-center gap-2 text-label text-text-muted">
              <Delta value={stats.meanDelta} /> newer vs older half
            </p>
          </div>
          <div>
            <p className="text-body-sm text-text-secondary">High or critical</p>
            <p className="mt-1 flex items-baseline gap-2">
              <span className="tabular text-metric">
                {stats.n ? formatCount(stats.highRisk) : "—"}
              </span>
              <Delta value={stats.highDelta} />
            </p>
          </div>
          {stats.n ? (
            <div className="flex gap-3 rounded-md bg-bg-raised p-3">
              <Info
                className="mt-0.5 size-icon-sm shrink-0 text-accent"
                aria-hidden="true"
              />
              <p className="text-body-sm text-text-secondary">
                <b className="font-semibold text-text-primary">
                  {pct(stats.highRisk, stats.n)}%
                </b>{" "}
                of the latest {formatCount(stats.n)} events scored High or
                Critical.{" "}
                {topDecision ? (
                  <>
                    {DECISION_LABEL[topDecision[0]]} was the most common
                    decision ({pct(topDecision[1], stats.n)}%)
                    {stats.byDecision.block
                      ? `, and ${formatCount(stats.byDecision.block)} were blocked.`
                      : "."}
                  </>
                ) : null}
              </p>
            </div>
          ) : null}
        </div>
        <div className="flex min-w-0 flex-col gap-3">
          <SeriesLegend series={series} />
          {loading ? (
            <div
              className="h-[220px] rounded-md bg-bg-sunken"
              style={{ animation: "pulse-soft 1.4s ease-in-out infinite" }}
            />
          ) : stats.buckets.length ? (
            <SeriesChart
              data={stats.buckets}
              series={series}
              ariaLabel={`Events over time, last ${SAMPLE} events: all events and those elevated or higher`}
            />
          ) : (
            <Empty title="Not enough events yet">
              The chart appears once this environment has received a few events.
            </Empty>
          )}
          {error ? <ErrorState error={error} /> : null}
        </div>
      </div>
    </Panel>
  );
}

/** A "movement" card: a headline, a + action, a segmented bar and a one-line driver. */
function MovementCard({
  title,
  value,
  caption,
  action,
  parts,
}: {
  title: string;
  value: string;
  caption: ReactNode;
  action?: ReactNode;
  parts: { key: string; value: number; fill: string; label: string }[];
}) {
  return (
    <Panel className="flex flex-col gap-4 p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-body-sm text-text-secondary">{title}</h2>
          <p className="mt-1 text-metric">
            <Figure value={value} />
          </p>
        </div>
        {action}
      </div>
      <SegmentedBar parts={parts} />
      <SegmentLegend parts={parts} />
      <p className="border-t border-border-subtle pt-3 text-body-sm text-text-secondary">
        {caption}
      </p>
    </Panel>
  );
}

function PlusLink({ href, label }: { href: string; label: string }) {
  return (
    <Tooltip side="bottom" content={label}>
      <Button variant="secondary" size="icon" asChild>
        <Link href={href as "/"} aria-label={label}>
          <Plus />
        </Link>
      </Button>
    </Tooltip>
  );
}

function DecisionsCard({
  stats,
  canAddRule,
}: {
  stats: Stats;
  canAddRule: boolean;
}) {
  const d = stats.byDecision;
  const interventions = d.step_up + d.review + d.block;
  const parts = [
    { key: "allow", value: d.allow, fill: DECISION_FILL.allow, label: "Allow" },
    {
      key: "step_up",
      value: d.step_up,
      fill: DECISION_FILL.step_up,
      label: "Step-up",
    },
    {
      key: "review",
      value: d.review,
      fill: DECISION_FILL.review,
      label: "Review",
    },
    { key: "block", value: d.block, fill: DECISION_FILL.block, label: "Block" },
  ];
  const driver = parts
    .slice(1)
    .sort((a, b) => b.value - a.value)
    .find((p) => p.value > 0);
  return (
    <MovementCard
      title="Interventions"
      value={stats.n ? formatCount(interventions) : "—"}
      parts={parts}
      action={
        canAddRule ? <PlusLink href="/rules/new" label="New rule" /> : null
      }
      caption={
        driver ? (
          <>
            <b className="font-semibold text-text-primary">{driver.label}</b>{" "}
            drove most interventions · {pct(driver.value, interventions)}%
          </>
        ) : (
          "No interventions in the latest events."
        )
      }
    />
  );
}

function RiskMixCard({ stats, canDeny }: { stats: Stats; canDeny: boolean }) {
  const order: RiskBand[] = ["critical", "high", "elevated", "low", "minimal"];
  const parts = order.map((b) => ({
    key: b,
    value: stats.byBand[b],
    fill: BAND_FILL[b],
    label: bandInfo(b).label,
  }));
  const driver = parts.slice(0, 3).find((p) => p.value > 0);
  return (
    <MovementCard
      title="High or critical"
      value={stats.n ? formatCount(stats.highRisk) : "—"}
      parts={parts}
      action={
        canDeny ? (
          <PlusLink href="/deny-lists" label="Add to a deny list" />
        ) : null
      }
      caption={
        driver ? (
          <>
            <b className="font-semibold text-text-primary">{driver.label}</b> is
            the most severe band present · {pct(driver.value, stats.n)}% of
            events
          </>
        ) : (
          "Nothing above Low in the latest events."
        )
      }
    />
  );
}

/** Masks an identifier to its prefix and last four characters: usr_••••4812. */
function mask(ref: string) {
  const m = /^([a-z]+_)?(.*)$/i.exec(ref);
  const head = m?.[1] ?? "";
  const body = m?.[2] ?? ref;
  return body.length <= 4 ? ref : `${head}••••${body.slice(-4)}`;
}

const SEVERITY_TILE: Record<RiskBand, string> = {
  minimal: "from-risk-minimal-bg text-risk-minimal",
  low: "from-blue-100 text-risk-low",
  elevated: "from-risk-elevated-bg text-risk-elevated",
  high: "from-risk-high-bg text-risk-high",
  critical: "from-risk-critical-bg text-risk-critical",
};

function OpenAlertsCard() {
  const { can } = useSession();
  const q = useAlerts({ status: "open" });
  const alerts = (q.data?.pages[0]?.data ?? [])
    .slice()
    .sort((a, b) => b.max_score - a.max_score)
    .slice(0, 4);
  const total = q.data?.pages[0]?.data.length ?? 0;
  return (
    <Panel className="flex flex-col">
      <PanelHeader
        title="Open alerts"
        legend={
          q.isSuccess
            ? `${formatCount(total)}${q.data.pages[0]?.next_cursor ? "+" : ""}`
            : undefined
        }
        actions={
          <Link href="/alerts" className="text-body-sm font-medium text-accent">
            View all
          </Link>
        }
      />
      <ul className="flex flex-col gap-2 px-5">
        {q.isLoading
          ? [0, 1, 2].map((i) => (
              <li
                key={i}
                className="h-16 rounded-md bg-bg-sunken"
                style={{ animation: "pulse-soft 1.4s ease-in-out infinite" }}
              />
            ))
          : null}
        {alerts.map((a) => {
          const band = a.severity as RiskBand;
          return (
            <li key={a.id}>
              <Link
                href={`/alerts?alert=${a.id}` as "/alerts"}
                className="flex items-center gap-3 rounded-md border border-border-subtle p-2.5 transition-colors duration-150 hover:bg-bg-raised"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "flex size-11 shrink-0 items-center justify-center rounded-md bg-gradient-to-br to-bg-surface",
                    SEVERITY_TILE[band],
                  )}
                >
                  <BandIcon shape={bandInfo(band).shape} className="size-4" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-body-sm font-medium">
                    {a.title}
                  </span>
                  <span className="mono block truncate text-label text-text-muted">
                    {a.subject.type === "user"
                      ? mask(a.subject.ref)
                      : humanize(a.subject.type)}{" "}
                    · {formatAge(a.last_seen_at)}
                  </span>
                </span>
                <span className="text-right">
                  <span className="tabular block text-heading-3">
                    {a.max_score}
                  </span>
                  <span className="sr-only">score, </span>
                  <span className="block text-label text-text-muted">
                    {bandInfo(band).label}
                  </span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
      {q.isError ? <ErrorState error={q.error} className="mx-5 mt-2" /> : null}
      {q.isSuccess && alerts.length === 0 ? (
        <p className="px-5 py-3 text-body-sm text-text-secondary">
          No open alerts. New ones appear here as decisions cross the alert
          threshold.
        </p>
      ) : null}
      <div className="p-5 pt-4">
        <Button className="w-full" icon={<Plus />} asChild>
          {can("cases:write") ? (
            <Link href="/cases">Create a case</Link>
          ) : (
            <Link href="/alerts">Open the alert queue</Link>
          )}
        </Button>
      </div>
    </Panel>
  );
}

const CASE_TONE: Record<Schemas["CaseStatus"], Tone> = {
  open: "warning",
  investigating: "info",
  resolved: "success",
};

function ScoreCard({ stats, showCases }: { stats: Stats; showCases: boolean }) {
  return (
    <Panel className="flex flex-col">
      <PanelHeader title="Average risk" legend={`last ${SAMPLE} events`} />
      <div className="px-5 pb-4">
        <TickScoreBar
          value={Math.round(stats.mean)}
          label={`Mean risk score ${stats.mean.toFixed(1)} of 100`}
          fill={
            stats.mean >= 50
              ? "bg-brand-orange"
              : stats.mean >= 20
                ? "bg-brand-blue"
                : "bg-blue-300"
          }
        >
          {stats.n ? <Figure value={stats.mean.toFixed(1)} /> : "—"}
        </TickScoreBar>
      </div>
      {showCases ? <RecentCases /> : null}
    </Panel>
  );
}

function RecentCases() {
  const q = useCases("open");
  const cases = (q.data?.pages[0]?.data ?? []).slice(0, 4);
  const date = new Intl.DateTimeFormat("en-GB", {
    day: "numeric",
    month: "short",
  });
  return (
    <div className="border-t border-border-subtle px-5 pt-3 pb-4">
      <div className="mb-1 flex items-center justify-between">
        <h3 className="text-body-sm font-medium">Open cases</h3>
        <Link href="/cases" className="text-body-sm font-medium text-accent">
          View all
        </Link>
      </div>
      <ul>
        {cases.map((c) => (
          <li key={c.id}>
            <Link
              href={`/cases/${c.id}` as "/cases"}
              className="grid grid-cols-[52px_auto_minmax(0,1fr)_auto] items-center gap-3 border-t border-border-subtle py-2.5 first:border-t-0 hover:text-accent"
            >
              <span className="text-label text-text-muted">
                {date.format(new Date(c.created_at))}
              </span>
              <StatusPill tone={CASE_TONE[c.status]}>
                {humanize(c.status)}
              </StatusPill>
              <span className="truncate text-body-sm">{c.title}</span>
              <RiskBadge band={c.priority as RiskBand} showScore={false} />
            </Link>
          </li>
        ))}
      </ul>
      {q.isLoading ? (
        <div
          className="h-20 rounded-md bg-bg-sunken"
          style={{ animation: "pulse-soft 1.4s ease-in-out infinite" }}
        />
      ) : null}
      {q.isError ? <ErrorState error={q.error} /> : null}
      {q.isSuccess && cases.length === 0 ? (
        <p className="py-2 text-body-sm text-text-secondary">No open cases.</p>
      ) : null}
    </div>
  );
}

const TABLE_TABS = [
  { key: "all", label: "All", test: () => true },
  {
    key: "high",
    label: "High risk",
    test: (e: EventRow) =>
      e.risk_level === "high" || e.risk_level === "critical",
  },
  {
    key: "review",
    label: "Needs review",
    test: (e: EventRow) => e.decision === "review" || e.decision === "step_up",
  },
  {
    key: "block",
    label: "Blocked",
    test: (e: EventRow) => e.decision === "block",
  },
] as const;

function ActivityTable({
  sample,
  loading,
  error,
  success,
}: {
  sample: EventRow[];
  loading: boolean;
  error?: unknown;
  success: boolean;
}) {
  const [tab, setTab] = useState<(typeof TABLE_TABS)[number]["key"]>("all");
  const active = TABLE_TABS.find((t) => t.key === tab)!;
  const rows = sample.filter(active.test).slice(0, 10);
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Recent risk activity"
        actions={
          <Link href="/events" className="text-body-sm font-medium text-accent">
            All events
          </Link>
        }
      />
      <div
        role="tablist"
        aria-label="Filter recent activity"
        className="flex flex-wrap gap-2 px-5 pb-3"
      >
        {TABLE_TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            role="tab"
            aria-selected={tab === t.key}
            onClick={() => setTab(t.key)}
            className={pillTabClass(tab === t.key)}
          >
            {t.label}
            <span
              className={cn(
                "tabular text-label",
                tab === t.key ? "text-white/70" : "text-text-muted",
              )}
            >
              {sample.filter(t.test).length}
            </span>
          </button>
        ))}
      </div>
      <Table aria-label="Recent risk activity">
        <thead>
          <tr>
            <Th>Event</Th>
            <Th>Type</Th>
            <Th>Risk</Th>
            <Th>Decision</Th>
            <Th className="text-right">Age</Th>
          </tr>
        </thead>
        <tbody>
          {loading ? <SkeletonRows columns={5} rows={5} /> : null}
          {rows.map((e) => (
            <Tr key={e.id}>
              <DataCell>
                <Link
                  href={`/events?event=${e.id}` as "/events"}
                  className="font-medium hover:text-accent"
                >
                  {e.event_key}
                </Link>
              </DataCell>
              <Td className="text-text-secondary">{humanize(e.type)}</Td>
              <Td>
                <RiskBadge
                  band={e.risk_level as RiskBand}
                  score={e.risk_score}
                />
              </Td>
              <Td>
                <StatusPill tone={DECISION_TONE[e.decision as Decision]}>
                  {DECISION_LABEL[e.decision as Decision]}
                </StatusPill>
              </Td>
              <Td className="tabular text-right text-text-muted">
                {formatAge(e.received_at)}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {error ? <ErrorState error={error} className="m-5" /> : null}
      {success && rows.length === 0 ? (
        <Empty title="Nothing here">
          None of the latest {SAMPLE} events match this filter.
        </Empty>
      ) : null}
    </Panel>
  );
}
