"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { CLOCK_KIND, flag, formatDue, TASK_LABEL } from "@/lib/compliance";
import {
  useClockLoad,
  useCrossBorder,
  useHolidays,
  useRetention,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { CountriesSideBySide, RuleLink } from "./library";

const RECORD_TYPE: Record<Schemas["RecordType"], string> = {
  customer_file: "Customer files",
  transaction: "Transactions",
  report: "Reports",
  audit_log: "Audit logs",
  other: "Other records",
};

/** "10 years" when a whole number of years, else days. */
function formatDays(days: number): string {
  if (days >= 365 && days % 365 === 0) {
    const y = days / 365;
    return `${y} year${y === 1 ? "" : "s"}`;
  }
  return `${days} day${days === 1 ? "" : "s"}`;
}

/** The work running against each clock now: how much, how much is late, how much is due soon. */
export function ClockLoadPanel() {
  const q = useClockLoad();
  const rows = (q.data?.data ?? []).filter((r) => r.running > 0);
  const unclocked = q.data?.unclocked ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Running now"
        legend="open work against each legal clock, in this environment"
      />
      {q.data?.sensitive_hidden ? (
        <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
          Suspicious transaction reports and compliance-only work are counted
          for compliance officers only.
        </p>
      ) : null}
      <Table aria-label="Work running against each clock">
        <thead>
          <tr>
            <Th>Country</Th>
            <Th>Clock</Th>
            <Th>Running</Th>
            <Th>Overdue</Th>
            <Th>Due within 24h</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={5} rows={3} /> : null}
          {rows.map((r) => (
            <Tr key={`${r.jurisdiction}-${r.clock.id}`}>
              <Td className="whitespace-nowrap">
                <span aria-hidden>{flag(r.jurisdiction)} </span>
                {r.jurisdiction}
              </Td>
              <Td className="py-2.5">
                <span className="font-medium">{r.clock.duration}</span>{" "}
                <StatusPill tone={CLOCK_KIND[r.clock.kind].tone}>
                  {CLOCK_KIND[r.clock.kind].label}
                </StatusPill>
                <span className="block text-body-sm text-text-secondary">
                  {r.clock.duty}
                </span>
              </Td>
              <Td className="mono text-data">{r.running}</Td>
              <Td
                className={
                  r.overdue
                    ? "mono text-data font-medium text-status-danger"
                    : "mono text-data"
                }
              >
                {r.overdue}
              </Td>
              <Td className="mono text-data">{r.due_soon}</Td>
            </Tr>
          ))}
          {unclocked
            .filter((u) => u.running > 0)
            .map((u) => (
              <Tr key={`${u.jurisdiction}-${u.rule}`}>
                <Td className="whitespace-nowrap">
                  <span aria-hidden>{flag(u.jurisdiction)} </span>
                  {u.jurisdiction}
                </Td>
                <Td className="py-2.5 text-body-sm">
                  <RuleLink id={u.rule} jurisdiction={u.jurisdiction} />{" "}
                  <span className="text-text-secondary">
                    (no legal clock listed)
                  </span>
                </Td>
                <Td className="mono text-data">{u.running}</Td>
                <Td className="mono text-data">{u.overdue}</Td>
                <Td className="mono text-data">{u.due_soon}</Td>
              </Tr>
            ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && rows.length === 0 && !unclocked.some((u) => u.running) ? (
        <Empty title="Nothing running">
          Counts appear here when a control opens work with a deadline.
        </Empty>
      ) : null}
    </Panel>
  );
}

/** Events that started a clock in a country other than the one that governed them. */
export function CrossBorderPanel() {
  const q = useCrossBorder();
  const events = q.data?.data ?? [];
  if (q.isSuccess && events.length === 0) return null;
  const now = new Date(q.dataUpdatedAt);
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Cross-border events"
        legend="events that started a clock in another country"
      />
      <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
        Each event is judged by the country that governs it. Duties owed to the
        person the data is about follow them: a breach affecting a Rwandan data
        subject starts Rwanda&rsquo;s 48-hour clock wherever it was booked.
      </p>
      <Table aria-label="Cross-border events">
        <thead>
          <tr>
            <Th>Event</Th>
            <Th>Booked in</Th>
            <Th>Clocks started</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={3} rows={2} /> : null}
          {events.map((e) => (
            <Tr key={e.event_id}>
              <Td className="mono text-data">
                <Link
                  className="underline"
                  href={`/events/${e.event_id}` as "/events"}
                >
                  {e.event_id}
                </Link>
              </Td>
              <Td className="whitespace-nowrap">
                <span aria-hidden>{flag(e.booked_in)} </span>
                {e.booked_in}
              </Td>
              <Td className="py-2.5 text-body-sm">
                <ul className="flex flex-col gap-1">
                  {e.tasks.map((t) => (
                    <li key={t.id}>
                      <span aria-hidden>{flag(t.jurisdiction)} </span>
                      {TASK_LABEL[t.kind] ?? t.kind} (
                      <RuleLink id={t.rule} jurisdiction={t.jurisdiction} />)
                      {t.due_at ? (
                        <span
                          className={
                            t.overdue
                              ? "font-medium text-status-danger"
                              : "text-text-secondary"
                          }
                        >
                          {" "}
                          {t.status === "open" || t.status === "approved"
                            ? formatDue(t.due_at, now)
                            : t.status}
                        </span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
    </Panel>
  );
}

/** The retention in force for each record type: the longest any activated country requires. */
export function RetentionPanel() {
  const q = useRetention();
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Retention in force"
        legend="the longest period any of your countries requires"
      />
      <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
        A deletion request (from a customer or your disposal policy) is refused
        while the record is younger than this, and the refusal is logged with
        the rule that required it (RW-GOV-10).
      </p>
      <Table aria-label="Retention in force">
        <thead>
          <tr>
            <Th>Records</Th>
            <Th>Kept at least</Th>
            <Th>Set by</Th>
            <Th>Also considered</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={4} rows={4} /> : null}
          {(q.data?.data ?? []).map((p) => (
            <Tr key={p.record_type}>
              <Td className="font-medium">{RECORD_TYPE[p.record_type]}</Td>
              <Td className="whitespace-nowrap">
                {p.rule ? formatDays(p.days) : "No minimum"}
              </Td>
              <Td className="text-body-sm">
                {p.rule ? (
                  <>
                    <span aria-hidden>{flag(p.rule.jurisdiction)} </span>
                    <RuleLink
                      id={p.rule.rule}
                      jurisdiction={p.rule.jurisdiction}
                    />
                  </>
                ) : (
                  "–"
                )}
              </Td>
              <Td className="py-2.5 text-body-sm text-text-secondary">
                {p.considered
                  .filter(
                    (c) =>
                      c.rule !== p.rule?.rule ||
                      c.jurisdiction !== p.rule?.jurisdiction,
                  )
                  .map(
                    (c) =>
                      `${c.rule} (${c.jurisdiction}) ${formatDays(c.days)}`,
                  )
                  .join("; ") || "–"}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
    </Panel>
  );
}

/** A country's public holidays, which its working-day clocks skip. */
export function HolidaysPanel({
  country,
  name,
}: {
  country: string;
  name: string;
}) {
  const { can } = useSession();
  const q = useHolidays(country);
  const [date, setDate] = useState("");
  const [label, setLabel] = useState("");
  const configure = can("compliance:configure");
  const add = useAction(
    async () =>
      unwrap(
        await api().POST(
          "/api/v1/compliance/jurisdictions/{jurisdiction}/holidays",
          {
            params: { path: { jurisdiction: country } },
            body: { date, name: label.trim() },
          },
        ),
      ),
    {
      invalidate: ["compliance"],
      onSuccess: () => {
        setDate("");
        setLabel("");
      },
    },
  );
  const remove = useAction(
    async (day: string) => {
      const r = await api().DELETE(
        "/api/v1/compliance/jurisdictions/{jurisdiction}/holidays/{date}",
        { params: { path: { jurisdiction: country, date: day } } },
      );
      if (r.error) unwrap(r);
      return day;
    },
    { invalidate: ["compliance"] },
  );
  const onAdd = (e: FormEvent) => {
    e.preventDefault();
    void add.run();
  };
  const holidays = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title={`${name} public holidays`}
        legend="working-day clocks skip these and weekends"
      />
      <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
        RiskOS ships no holiday dates: they are gazetted year by year. Enter
        them here so a hold counted in working days (an FIC suspension in Ghana)
        ends on the right day.
      </p>
      {(add.error ?? remove.error) ? (
        <div className="px-5 pb-3">
          <Notice tone="error">{add.error ?? remove.error}</Notice>
        </div>
      ) : null}
      <Table aria-label={`${name} public holidays`}>
        <thead>
          <tr>
            <Th>Date</Th>
            <Th>Holiday</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={3} rows={2} /> : null}
          {holidays.map((h) => (
            <Tr key={h.date}>
              <Td className="mono text-data">{h.date}</Td>
              <Td className="py-2.5">{h.name}</Td>
              <Td className="text-right">
                {configure ? (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={remove.busy}
                    aria-label={`Remove ${h.name} on ${h.date}`}
                    onClick={() => void remove.run(h.date)}
                  >
                    Remove
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isSuccess && holidays.length === 0 ? (
        <Empty title="No holidays entered">
          Until you enter them, only Saturdays and Sundays are skipped.
        </Empty>
      ) : null}
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {configure ? (
        <form
          onSubmit={onAdd}
          className="flex flex-wrap items-end gap-2 border-t border-border-subtle px-5 py-4"
        >
          <Field label="Date">
            {(f) => (
              <Input
                {...f}
                type="date"
                required
                value={date}
                onChange={(e) => setDate(e.target.value)}
              />
            )}
          </Field>
          <Field label="Holiday" className="min-w-48 flex-1">
            {(f) => (
              <Input
                {...f}
                required
                maxLength={100}
                value={label}
                onChange={(e) => setLabel(e.target.value)}
              />
            )}
          </Field>
          <Button
            type="submit"
            variant="primary"
            disabled={add.busy || !date || !label.trim()}
          >
            Add holiday
          </Button>
        </form>
      ) : null}
    </Panel>
  );
}

/**
 * The deadlines matrix: the work running against each clock now, events that crossed a border,
 * the retention in force, and the library's side-by-side comparison of the countries.
 */
export function DeadlinesMatrix() {
  return (
    <div className="flex flex-col gap-5">
      <ClockLoadPanel />
      <CrossBorderPanel />
      <RetentionPanel />
      <CountriesSideBySide />
    </div>
  );
}
