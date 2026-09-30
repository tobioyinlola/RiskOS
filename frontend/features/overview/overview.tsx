"use client";

import Link from "next/link";
import { useMemo } from "react";

import {
  Metric,
  PageHeader,
  Panel,
  PanelHeader,
} from "@/components/instrument/panel";
import { BandBars, RiskBadge } from "@/components/instrument/risk-badge";
import {
  Empty,
  ErrorState,
  SkeletonRows,
} from "@/components/instrument/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/instrument/table";
import { TraceLine } from "@/components/instrument/trace-line";
import { formatAge, formatCount, humanize } from "@/lib/format";
import { useAlerts, useCases, useEvents } from "@/lib/queries";
import {
  BANDS,
  DECISION_LABEL,
  type Decision,
  type RiskBand,
} from "@/lib/risk";
import { useSession } from "@/lib/session";

const SAMPLE = 100;

/**
 * The overview reads the newest events, alerts and cases of this environment. Figures are
 * labelled with their window ("last 100 events"): there is no analytics store yet.
 */
export function Overview() {
  const { can, session } = useSession();
  const events = useEvents(SAMPLE);
  const openAlerts = useAlerts({ status: "open" });
  const openCases = useCases("open");
  const sample = useMemo(
    () => events.data?.pages[0]?.data ?? [],
    [events.data],
  );

  const byBand = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of sample) m.set(e.risk_level, (m.get(e.risk_level) ?? 0) + 1);
    return m;
  }, [sample]);
  const byDecision = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of sample) m.set(e.decision, (m.get(e.decision) ?? 0) + 1);
    return m;
  }, [sample]);
  const scores = useMemo(
    () => [...sample].reverse().map((e) => e.risk_score),
    [sample],
  );
  const mean = sample.length
    ? Math.round(sample.reduce((s, e) => s + e.risk_score, 0) / sample.length)
    : 0;
  const highRisk = (byBand.get("high") ?? 0) + (byBand.get("critical") ?? 0);
  const count = (n: number | undefined, more: boolean | undefined) =>
    n === undefined ? "—" : `${formatCount(n)}${more ? "+" : ""}`;

  return (
    <>
      <PageHeader
        title="Overview"
        description={`${session.tenant.name}: what the instrument is reading now.`}
      />
      <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Metric
          label={`Events · last ${SAMPLE}`}
          value={events.isSuccess ? formatCount(sample.length) : "—"}
        >
          {scores.length > 1 ? (
            <TraceLine
              points={scores}
              width={200}
              height={32}
              label="Risk score of recent events"
            />
          ) : null}
        </Metric>
        <Metric
          label={`Mean score · last ${SAMPLE}`}
          value={events.isSuccess ? String(mean).padStart(3, "0") : "—"}
          unit="/ 100"
        />
        <Metric
          label={`High or critical · last ${SAMPLE}`}
          value={events.isSuccess ? formatCount(highRisk) : "—"}
        />
        {can("alerts:read") ? (
          <Metric
            label="Open alerts"
            value={count(
              openAlerts.data?.pages[0]?.data.length,
              !!openAlerts.data?.pages[0]?.next_cursor,
            )}
          >
            {can("cases:read") ? (
              <span className="text-body-sm text-ink-2">
                {count(
                  openCases.data?.pages[0]?.data.length,
                  !!openCases.data?.pages[0]?.next_cursor,
                )}{" "}
                open cases
              </span>
            ) : null}
          </Metric>
        ) : null}
      </div>

      <div className="mb-6 grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Panel>
          <PanelHeader
            title="Risk distribution"
            legend={`last ${SAMPLE} events`}
          />
          <DistributionTable
            rows={BANDS.map((b) => ({
              key: b.band,
              label: <RiskBadge band={b.band} showScore={false} />,
              count: byBand.get(b.band) ?? 0,
            }))}
            total={sample.length}
          />
        </Panel>
        <Panel>
          <PanelHeader title="Decisions" legend={`last ${SAMPLE} events`} />
          <DistributionTable
            rows={(["allow", "step_up", "review", "block"] as Decision[]).map(
              (d) => ({
                key: d,
                label: (
                  <span className="text-body-sm">{DECISION_LABEL[d]}</span>
                ),
                count: byDecision.get(d) ?? 0,
              }),
            )}
            total={sample.length}
          />
        </Panel>
      </div>

      <Panel className="overflow-hidden">
        <PanelHeader
          title="Recent risk activity"
          actions={
            <Link href="/events" className="text-body-sm text-signal-ink">
              All events
            </Link>
          }
        />
        <Table aria-label="Recent risk activity">
          <thead>
            <tr>
              <Th>Risk</Th>
              <Th>Event</Th>
              <Th>Type</Th>
              <Th>Decision</Th>
              <Th>Age</Th>
            </tr>
          </thead>
          <tbody>
            {events.isLoading ? <SkeletonRows columns={5} rows={5} /> : null}
            {sample
              .filter((e) => e.risk_level !== "minimal")
              .slice(0, 10)
              .map((e) => (
                <Tr key={e.id} density="compact">
                  <Td>
                    <RiskBadge
                      band={e.risk_level as RiskBand}
                      score={e.risk_score}
                    />
                  </Td>
                  <DataCell>
                    <Link
                      href={`/events?event=${e.id}` as "/events"}
                      className="hover:text-signal-ink"
                    >
                      {e.event_key}
                    </Link>
                  </DataCell>
                  <Td className="text-body-sm">{humanize(e.type)}</Td>
                  <Td className="text-body-sm">
                    {DECISION_LABEL[e.decision as Decision]}
                  </Td>
                  <DataCell className="text-ink-3">
                    {formatAge(e.received_at)}
                  </DataCell>
                </Tr>
              ))}
          </tbody>
        </Table>
        {events.isError ? (
          <ErrorState error={events.error} className="m-4" />
        ) : null}
        {events.isSuccess && !sample.some((e) => e.risk_level !== "minimal") ? (
          <Empty title="Nothing above minimal risk">
            The newest events all read Minimal.
          </Empty>
        ) : null}
      </Panel>
    </>
  );
}

/** A distribution as a table with inline bars (every chart has a table view). */
function DistributionTable({
  rows,
  total,
}: {
  rows: { key: string; label: React.ReactNode; count: number }[];
  total: number;
}) {
  return (
    <Table>
      <tbody>
        {rows.map((r) => {
          const share = total ? r.count / total : 0;
          return (
            <Tr key={r.key} density="compact">
              <Td className="w-32">{r.label}</Td>
              <Td>
                <span
                  className="flex h-2 w-full items-center bg-bg-sunken"
                  aria-hidden="true"
                >
                  <span
                    className="h-2 bg-ink-2"
                    style={{ width: `${share * 100}%` }}
                  />
                </span>
              </Td>
              <DataCell className="w-16 text-right">
                {formatCount(r.count)}
              </DataCell>
              <DataCell className="w-16 text-right text-ink-3">
                {Math.round(share * 100)}%
              </DataCell>
            </Tr>
          );
        })}
      </tbody>
    </Table>
  );
}

export { BandBars };
