"use client";

import { useMemo, useState } from "react";

import { TrendLine } from "@/components/ui/chart";
import { Tab, TabList, TabPanel, Tabs } from "@/components/ui/misc";
import { Metric, PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { Empty, ErrorState, Loading } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import type { Schemas } from "@/lib/api/client";
import { formatCount } from "@/lib/format";
import { useAnalytics, type AnalyticsWindow } from "@/lib/queries";
import { BAND_FILL, BANDS } from "@/lib/risk";

import { DecisionBar, TopUsers } from "../entities/entities";

const WINDOWS: { value: AnalyticsWindow; label: string }[] = [
  { value: "24h", label: "Last 24 hours" },
  { value: "7d", label: "Last 7 days" },
  { value: "30d", label: "Last 30 days" },
];

type Point = Schemas["AnalyticsPoint"];

function bucketLabel(iso: string, bucket: "hour" | "day"): string {
  const d = new Date(iso);
  return bucket === "day"
    ? d.toISOString().slice(0, 10)
    : `${d.toISOString().slice(5, 10)} ${d.toISOString().slice(11, 16)}`;
}

/** Fills empty buckets so the axis is continuous. */
function continuous(
  points: Point[],
  since: string,
  bucket: "hour" | "day",
  count: number,
): Point[] {
  const step = bucket === "day" ? 86_400_000 : 3_600_000;
  const byTime = new Map(points.map((p) => [new Date(p.at).getTime(), p]));
  const start = new Date(since).getTime();
  const zero = { allow: 0, step_up: 0, review: 0, block: 0 };
  const zeroBands = { minimal: 0, low: 0, elevated: 0, high: 0, critical: 0 };
  return Array.from({ length: count }, (_, i) => {
    const t = start + i * step;
    return (
      byTime.get(t) ?? {
        at: new Date(t).toISOString(),
        events: 0,
        mean_score: 0,
        decisions: zero,
        bands: zeroBands,
      }
    );
  });
}

/** Events per bucket as columns stacked by band, in band order (risk tokens only). */
function BandColumns({
  points,
  bucket,
}: {
  points: Point[];
  bucket: "hour" | "day";
}) {
  const max = Math.max(1, ...points.map((p) => p.events));
  const order = [...BANDS].reverse();
  return (
    <div className="flex flex-col gap-2 p-4">
      <div
        className="flex h-48 items-end gap-px"
        role="img"
        aria-label="Events per bucket by risk band; see the table view for values"
      >
        {points.map((p) => (
          <div
            key={p.at}
            className="flex h-full min-w-0 flex-1 flex-col justify-end"
            title={`${bucketLabel(p.at, bucket)}: ${formatCount(p.events)} events`}
          >
            {order.map((b) => {
              const n = p.bands[b.band];
              if (!n) return null;
              return (
                <span
                  key={b.band}
                  className={BAND_FILL[b.band]}
                  style={{ height: `${(n / max) * 100}%` }}
                />
              );
            })}
          </div>
        ))}
      </div>
      <div className="mono flex justify-between text-label text-text-muted">
        <span>{points[0] ? bucketLabel(points[0].at, bucket) : ""}</span>
        <span>
          {points.at(-1) ? bucketLabel(points.at(-1)!.at, bucket) : ""}
        </span>
      </div>
      <ul className="flex flex-wrap gap-4 text-label text-text-secondary">
        {BANDS.map((b) => (
          <li key={b.band} className="flex items-center gap-1">
            <span
              className={`size-2 rounded-sm ${BAND_FILL[b.band]}`}
              aria-hidden="true"
            />
            {b.label}
          </li>
        ))}
      </ul>
    </div>
  );
}

function SeriesTable({
  points,
  bucket,
}: {
  points: Point[];
  bucket: "hour" | "day";
}) {
  return (
    <div className="max-h-96 overflow-y-auto">
      <Table aria-label="Events per bucket">
        <thead>
          <tr>
            <Th>{bucket === "day" ? "Day" : "Hour"} (UTC)</Th>
            <Th className="text-right">Events</Th>
            <Th className="text-right">Mean score</Th>
            {BANDS.map((b) => (
              <Th key={b.band} className="text-right">
                {b.label}
              </Th>
            ))}
          </tr>
        </thead>
        <tbody>
          {[...points].reverse().map((p) => (
            <Tr key={p.at} density="compact">
              <DataCell>{bucketLabel(p.at, bucket)}</DataCell>
              <DataCell className="text-right">
                {formatCount(p.events)}
              </DataCell>
              <DataCell className="text-right">
                {p.events ? p.mean_score : "—"}
              </DataCell>
              {BANDS.map((b) => (
                <DataCell
                  key={b.band}
                  className="text-right text-text-secondary"
                >
                  {formatCount(p.bands[b.band])}
                </DataCell>
              ))}
            </Tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}

export function Analytics() {
  const [window, setWindow] = useState<AnalyticsWindow>("24h");
  const q = useAnalytics(window);
  const a = q.data;
  const points = useMemo(() => {
    if (!a) return [];
    const count = window === "24h" ? 25 : window === "7d" ? 169 : 31;
    return continuous(a.series, a.since, a.bucket, count);
  }, [a, window]);
  const totals = useMemo(() => {
    const t = {
      events: 0,
      allow: 0,
      step_up: 0,
      review: 0,
      block: 0,
      high: 0,
      scoreSum: 0,
    };
    for (const p of a?.series ?? []) {
      t.events += p.events;
      t.allow += p.decisions.allow;
      t.step_up += p.decisions.step_up;
      t.review += p.decisions.review;
      t.block += p.decisions.block;
      t.high += p.bands.high + p.bands.critical;
      t.scoreSum += p.mean_score * p.events;
    }
    return t;
  }, [a]);
  const maxHits = Math.max(1, ...(a?.top_signals ?? []).map((s) => s.hits));

  return (
    <>
      <PageHeader
        title="Analytics"
        description="Decisions, scores and signals over time in this environment."
        actions={
          <div role="tablist" aria-label="Window" className="flex gap-1">
            {WINDOWS.map((w) => (
              <button
                key={w.value}
                role="tab"
                aria-selected={window === w.value}
                onClick={() => setWindow(w.value)}
                className={`h-control-sm rounded-md px-3 text-body-sm transition-colors duration-150 ${window === w.value ? "bg-accent-subtle text-text-primary" : "text-text-secondary hover:bg-bg-raised"}`}
              >
                {w.label}
              </button>
            ))}
          </div>
        }
      />
      {q.isLoading ? <Loading /> : null}
      {q.isError ? <ErrorState error={q.error} /> : null}
      {a ? (
        <>
          <div className="mb-5 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <Metric label="Events" value={formatCount(totals.events)} />
            <Metric
              label="Mean score"
              value={
                totals.events
                  ? Math.round(totals.scoreSum / totals.events)
                  : "—"
              }
              unit="/ 100"
            >
              {points.some((p) => p.events) ? (
                <TrendLine
                  points={points
                    .filter((p) => p.events)
                    .map((p) => p.mean_score)}
                  width={200}
                  height={32}
                  label="Mean score per bucket"
                />
              ) : null}
            </Metric>
            <Metric label="High or critical" value={formatCount(totals.high)} />
            <Metric
              label="Reviewed or blocked"
              value={formatCount(totals.review + totals.block)}
            />
          </div>

          <Panel className="mb-5 overflow-hidden">
            <PanelHeader
              title="Events by risk band"
              legend={`per ${a.bucket}, UTC`}
            />
            {totals.events === 0 ? (
              <Empty title="No events in this window" />
            ) : (
              <Tabs defaultValue="chart">
                <TabList className="px-4 pt-3">
                  <Tab value="chart">Chart</Tab>
                  <Tab value="table">Table</Tab>
                </TabList>
                <TabPanel value="chart">
                  <BandColumns points={points} bucket={a.bucket} />
                </TabPanel>
                <TabPanel value="table">
                  <SeriesTable
                    points={points.filter((p) => p.events)}
                    bucket={a.bucket}
                  />
                </TabPanel>
              </Tabs>
            )}
          </Panel>

          <Panel className="mb-5 overflow-hidden">
            <PanelHeader title="Decisions" />
            <DecisionBar
              counts={{
                allow: totals.allow,
                step_up: totals.step_up,
                review: totals.review,
                block: totals.block,
              }}
            />
          </Panel>

          <div className="grid grid-cols-1 gap-5 xl:grid-cols-2">
            <Panel className="overflow-hidden">
              <PanelHeader title="Highest-risk users" />
              <TopUsers rows={a.top_users} />
            </Panel>
            <Panel className="overflow-hidden">
              <PanelHeader title="Signals that fired most" />
              <Table aria-label="Signals that fired most">
                <thead>
                  <tr>
                    <Th>Signal</Th>
                    <Th />
                    <Th className="text-right">Events</Th>
                  </tr>
                </thead>
                <tbody>
                  {a.top_signals.map((s) => (
                    <Tr key={s.code}>
                      <Td>
                        <span className="block text-body-sm">{s.label}</span>
                        <span className="mono text-label text-text-muted">
                          {s.code}
                        </span>
                      </Td>
                      <Td className="w-1/3">
                        <span
                          className="block h-1.5 overflow-hidden rounded-full bg-bg-sunken"
                          aria-hidden="true"
                        >
                          <i
                            className="block h-full bg-text-secondary"
                            style={{ width: `${(s.hits / maxHits) * 100}%` }}
                          />
                        </span>
                      </Td>
                      <DataCell className="text-right">
                        {formatCount(s.hits)}
                      </DataCell>
                    </Tr>
                  ))}
                </tbody>
              </Table>
              {a.top_signals.length === 0 ? (
                <Empty title="No signal fired in this window" />
              ) : null}
            </Panel>
          </div>
          <p className="mt-3 text-label text-text-muted">
            Figures cover events received since{" "}
            {a.since.slice(0, 16).replace("T", " ")} UTC.
          </p>
        </>
      ) : null}
    </>
  );
}
