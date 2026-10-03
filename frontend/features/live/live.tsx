"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { LiveDot } from "@/components/ui/chart";
import { Select } from "@/components/ui/field";
import { Metric, PageHeader, Panel } from "@/components/ui/panel";
import { RiskBadge } from "@/components/ui/risk-badge";
import { Empty, ErrorState, SkeletonRows } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import {
  formatAmount,
  formatCount,
  formatTimestamp,
  humanize,
} from "@/lib/format";
import { BANDS, type RiskBand } from "@/lib/risk";
import { useSession } from "@/lib/session";

import { EntityLink } from "../entities/link";
import { EventDrawer } from "../events/events";
import { Pause, Play } from "@/components/ui/icons";
import { DecisionPill } from "@/components/ui/pill";

type Event = Schemas["EventSummary"];

/** How often the monitor asks for newer events, and how many rows it keeps. */
const POLL_MS = 2000;
const KEEP = 200;

const BAND_ORDER = BANDS.map((b) => b.band);

function clock(iso: string): string {
  return new Date(iso).toISOString().slice(11, 19);
}

/**
 * The live risk monitor (docs/design-system.md, Motion): new events stream in at the top. While
 * the pointer is over the table, an event is open, or the monitor is paused, new events wait
 * above the table instead of moving the rows being read. Arrivals are announced politely.
 */
export function LiveMonitor() {
  const { environment } = useSession();
  const [rows, setRows] = useState<Event[]>([]);
  const [waiting, setWaiting] = useState<Event[]>([]);
  const [fresh, setFresh] = useState<Set<string>>(new Set());
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<unknown>();
  const [paused, setPaused] = useState(false);
  const [hovering, setHovering] = useState(false);
  const [openId, setOpenId] = useState<string>();
  const [minBand, setMinBand] = useState<RiskBand>("minimal");
  const [announce, setAnnounce] = useState("");
  // The clock for "last minute" figures, advanced by a timer (render stays pure).
  const [now, setNow] = useState(0);
  useEffect(() => {
    const tick = () => setNow(Date.now());
    const first = window.setTimeout(tick, 0);
    const timer = window.setInterval(tick, 1000);
    return () => {
      window.clearTimeout(first);
      window.clearInterval(timer);
    };
  }, []);
  const newest = useRef<string | undefined>(undefined);
  const holding = paused || hovering || !!openId;
  const holdingRef = useRef(holding);
  useEffect(() => {
    holdingRef.current = holding;
  }, [holding]);

  const apply = useCallback((incoming: Event[]) => {
    if (incoming.length === 0) return;
    setRows((r) => [...incoming, ...r].slice(0, KEEP));
    setFresh(new Set(incoming.map((e) => e.id)));
    setAnnounce(
      `${incoming.length} new event${incoming.length === 1 ? "" : "s"}`,
    );
  }, []);

  // Start over when the environment changes.
  useEffect(() => {
    let cancelled = false;
    newest.current = undefined;
    const load = async () => {
      try {
        const first = unwrap(
          await api().GET("/api/v1/events", {
            params: { query: { limit: 50 } },
          }),
        );
        if (cancelled) return;
        setRows(first.data);
        setWaiting([]);
        setLoaded(true);
        setError(undefined);
        newest.current = first.data[0]?.id;
      } catch (e) {
        if (!cancelled) setError(e);
      }
    };
    void load();

    const tick = async () => {
      if (document.visibilityState !== "visible") return;
      try {
        const page = unwrap(
          await api().GET("/api/v1/events", {
            params: {
              query: {
                limit: 100,
                ...(newest.current ? { after: newest.current } : {}),
              },
            },
          }),
        );
        if (cancelled || page.data.length === 0) return;
        newest.current = page.data[0]!.id;
        if (holdingRef.current)
          setWaiting((w) => [...page.data, ...w].slice(0, KEEP));
        else apply(page.data);
        setError(undefined);
      } catch (e) {
        if (!cancelled) setError(e);
      }
    };
    const timer = window.setInterval(() => void tick(), POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [environment, apply]);

  const release = () => {
    apply(waiting);
    setWaiting([]);
  };

  // Releasing the hold (pointer leaves, drawer closes) shows what arrived meanwhile.
  const [wasHolding, setWasHolding] = useState(holding);
  if (wasHolding !== holding) {
    setWasHolding(holding);
    if (!holding && waiting.length > 0) {
      setRows((r) => [...waiting, ...r].slice(0, KEEP));
      setFresh(new Set(waiting.map((e) => e.id)));
      setAnnounce(`${waiting.length} new events`);
      setWaiting([]);
    }
  }

  const shown = useMemo(
    () =>
      rows.filter(
        (e) =>
          BAND_ORDER.indexOf(e.risk_level as RiskBand) >=
          BAND_ORDER.indexOf(minBand),
      ),
    [rows, minBand],
  );
  const minuteAgo = now - 60_000;
  const lastMinute = rows.filter(
    (e) => new Date(e.received_at).getTime() >= minuteAgo,
  );
  const flagged = lastMinute.filter(
    (e) => e.decision === "review" || e.decision === "block",
  );

  return (
    <>
      <PageHeader
        title="Live monitor"
        description="Events as they arrive in this environment. Hover the table to hold it still."
        actions={
          <Button
            onClick={() => setPaused((p) => !p)}
            aria-pressed={paused}
            icon={paused ? <Play /> : <Pause />}
          >
            {paused ? "Resume" : "Pause"}
          </Button>
        }
      />
      <div className="mb-5 grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Metric
          label="Status"
          value={paused ? "Paused" : holding ? "Holding" : "Live"}
        >
          <span className="flex items-center gap-2 text-body-sm text-text-muted">
            {holding ? null : <LiveDot />}
            {holding
              ? "New events wait above the table"
              : `Checking every ${POLL_MS / 1000}s`}
          </span>
        </Metric>
        <Metric
          label="Events · last minute"
          value={formatCount(lastMinute.length)}
        />
        <Metric
          label="Review or block · last minute"
          value={formatCount(flagged.length)}
        />
      </div>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <Select
          aria-label="Lowest band shown"
          className="h-control-sm w-52 text-body-sm"
          value={minBand}
          onChange={(e) => setMinBand(e.target.value as RiskBand)}
        >
          {BANDS.map((b) => (
            <option key={b.band} value={b.band}>
              {b.band === "minimal" ? "All bands" : `${b.label} and above`}
            </option>
          ))}
        </Select>
        {waiting.length > 0 ? (
          <Button size="sm" variant="primary" onClick={release}>
            Show {formatCount(waiting.length)} new event
            {waiting.length === 1 ? "" : "s"}
          </Button>
        ) : null}
      </div>
      <p className="sr-only" aria-live="polite">
        {announce}
      </p>
      {error ? <ErrorState error={error} className="mb-3" /> : null}
      <Panel
        className="overflow-hidden"
        onMouseEnter={() => {
          // Hold at once: a poll landing before the next render must not move the rows.
          holdingRef.current = true;
          setHovering(true);
        }}
        onMouseLeave={() => setHovering(false)}
      >
        <Table aria-label="Live events">
          <thead>
            <tr>
              <Th>Time</Th>
              <Th>Risk</Th>
              <Th>Type</Th>
              <Th>User</Th>
              <Th className="text-right">Amount</Th>
              <Th>Country</Th>
              <Th>Decision</Th>
            </tr>
          </thead>
          <tbody>
            {!loaded && !error ? <SkeletonRows columns={7} /> : null}
            {shown.map((e) => (
              <Tr
                key={e.id}
                density="compact"
                selected={e.id === openId}
                className="cursor-pointer"
                onClick={() => setOpenId(e.id)}
                style={
                  fresh.has(e.id)
                    ? { animation: "fade-in 200ms ease-out" }
                    : undefined
                }
              >
                <DataCell
                  className="text-text-muted"
                  title={formatTimestamp(e.received_at)}
                >
                  {clock(e.received_at)}
                </DataCell>
                <Td>
                  <RiskBadge
                    band={e.risk_level as RiskBand}
                    score={e.risk_score}
                  />
                </Td>
                <Td className="text-body-sm">{humanize(e.type)}</Td>
                <Td>
                  {e.actor ? (
                    <EntityLink kind="user" refId={e.actor.user_id} />
                  ) : null}
                </Td>
                <DataCell className="text-right whitespace-nowrap">
                  {e.transaction
                    ? formatAmount(e.transaction.amount, e.transaction.currency)
                    : ""}
                </DataCell>
                <DataCell className="text-text-secondary">
                  {e.country ?? ""}
                </DataCell>
                <Td>
                  <DecisionPill decision={e.decision} />
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {loaded && shown.length === 0 ? (
          <Empty title="Waiting for events">
            {rows.length === 0
              ? "New events appear here as they are ingested."
              : "No recent event reaches the lowest band shown."}
          </Empty>
        ) : null}
      </Panel>
      <EventDrawer id={openId} onClose={() => setOpenId(undefined)} />
    </>
  );
}
