"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/field";
import {
  CodeBlock,
  CopyValue,
  KeyValues,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Tag,
} from "@/components/ui/misc";
import { Drawer } from "@/components/ui/overlay";
import { PageHeader, Panel } from "@/components/ui/panel";
import { RiskBadge } from "@/components/ui/risk-badge";
import {
  Empty,
  ErrorState,
  Loading,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { isTyping, usePageShortcuts } from "@/components/shell/shortcuts";
import type { Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import { useEvent, useEvents, type EventFilter } from "@/lib/queries";
import {
  BANDS,
  DECISION_LABEL,
  type Decision,
  type RiskBand,
} from "@/lib/risk";

import { EntityLink } from "../entities/link";
import { DecisionReading } from "../risk/explain";
import { Close } from "@/components/ui/icons";
import { DecisionPill } from "@/components/ui/pill";

const EVENT_TYPES: Schemas["EventType"][] = [
  "transaction",
  "login",
  "beneficiary_added",
  "device_change",
  "session_change",
  "profile_change",
  "activity",
];

/** Reads the event filters from the URL (?type=&decision=&risk_level=&user=). */
function filterFromParams(params: URLSearchParams): EventFilter {
  const f: EventFilter = {};
  const type = params.get("type");
  const decision = params.get("decision");
  const level = params.get("risk_level");
  const user = params.get("user");
  if (type) f.type = type as Schemas["EventType"];
  if (decision) f.decision = decision as Schemas["Decision"];
  if (level) f.risk_level = level as Schemas["RiskLevel"];
  if (user) f.user = user;
  return f;
}

/** Filters for event listings; changes are written to the URL so a view can be shared. */
export function EventFilters({
  filter,
  onChange,
  showType = true,
}: {
  filter: EventFilter;
  onChange: (f: EventFilter) => void;
  showType?: boolean;
}) {
  const [user, setUser] = useState(filter.user ?? "");
  const active = Object.values(filter).some(Boolean);
  const set = (patch: Partial<EventFilter>) => {
    const next = { ...filter, ...patch };
    for (const k of Object.keys(next) as (keyof EventFilter)[]) {
      if (!next[k]) delete next[k];
    }
    onChange(next);
  };
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      {showType ? (
        <Select
          aria-label="Type"
          className="h-control-sm w-44 text-body-sm"
          value={filter.type ?? ""}
          onChange={(e) =>
            set({ type: (e.target.value || undefined) as EventFilter["type"] })
          }
        >
          <option value="">All types</option>
          {EVENT_TYPES.map((t) => (
            <option key={t} value={t}>
              {humanize(t)}
            </option>
          ))}
        </Select>
      ) : null}
      <Select
        aria-label="Risk band"
        className="h-control-sm w-40 text-body-sm"
        value={filter.risk_level ?? ""}
        onChange={(e) =>
          set({
            risk_level: (e.target.value ||
              undefined) as EventFilter["risk_level"],
          })
        }
      >
        <option value="">All bands</option>
        {BANDS.map((b) => (
          <option key={b.band} value={b.band}>
            {b.label}
          </option>
        ))}
      </Select>
      <Select
        aria-label="Decision"
        className="h-control-sm w-40 text-body-sm"
        value={filter.decision ?? ""}
        onChange={(e) =>
          set({
            decision: (e.target.value || undefined) as EventFilter["decision"],
          })
        }
      >
        <option value="">All decisions</option>
        {(Object.keys(DECISION_LABEL) as Decision[]).map((d) => (
          <option key={d} value={d}>
            {DECISION_LABEL[d]}
          </option>
        ))}
      </Select>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          set({ user: user.trim() || undefined });
        }}
      >
        <Input
          aria-label="User"
          placeholder="User ID, then Enter"
          className="mono h-control-sm w-56 text-body-sm"
          value={user}
          onChange={(e) => setUser(e.target.value)}
          onBlur={() => set({ user: user.trim() || undefined })}
        />
      </form>
      {active ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            setUser("");
            onChange({});
          }}
        >
          <Close /> Clear filters
        </Button>
      ) : null}
    </div>
  );
}

/**
 * A newest-first event table with keyboard navigation (j/k, Enter). It opens events in the
 * drawer; with `shortcuts` the page's keys are registered for the help overlay.
 */
export function EventTable({
  filter = {},
  openId,
  onOpen,
  shortcuts = false,
}: {
  filter?: EventFilter;
  openId?: string;
  onOpen?: (id: string | undefined) => void;
  shortcuts?: boolean;
}) {
  const q = useEvents(50, filter);
  const events = useMemo(
    () => (q.data?.pages ?? []).flatMap((p) => p.data),
    [q.data],
  );
  const [cursor, setCursor] = useState(0);
  const [localOpen, setLocalOpen] = useState<string>();
  const current = onOpen ? openId : localOpen;
  const open = onOpen ?? setLocalOpen;

  useEffect(() => {
    if (!shortcuts) return;
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e) || e.metaKey || e.ctrlKey || e.altKey || current) return;
      if (e.key === "j")
        setCursor((c) => Math.min(c + 1, Math.max(0, events.length - 1)));
      else if (e.key === "k") setCursor((c) => Math.max(0, c - 1));
      else if (e.key === "Enter" && events[cursor]) open(events[cursor].id);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [shortcuts, events, cursor, open, current]);

  return (
    <>
      <Table aria-label="Events">
        <thead>
          <tr>
            <Th>Risk</Th>
            <Th>Event</Th>
            <Th>Type</Th>
            <Th>User</Th>
            <Th>Decision</Th>
            <Th>Received</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={6} /> : null}
          {events.map((e, i) => (
            <Tr
              key={e.id}
              selected={(shortcuts && i === cursor) || e.id === current}
              className="cursor-pointer"
              onClick={() => {
                setCursor(i);
                open(e.id);
              }}
            >
              <Td>
                <RiskBadge
                  band={e.risk_level as RiskBand}
                  score={e.risk_score}
                />
              </Td>
              <DataCell>
                <span className="block">{e.event_key}</span>
                <span className="text-label text-text-muted">{e.id}</span>
              </DataCell>
              <Td className="text-body-sm">{humanize(e.type)}</Td>
              <Td>
                {e.actor ? (
                  <EntityLink kind="user" refId={e.actor.user_id} />
                ) : null}
              </Td>
              <Td>
                <DecisionPill decision={e.decision} />
              </Td>
              <DataCell
                className="text-text-muted"
                title={formatTimestamp(e.received_at)}
              >
                {formatAge(e.received_at)}
              </DataCell>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && events.length === 0 ? (
        <Empty title="No events match">
          {Object.keys(filter).length
            ? "Try fewer filters."
            : "Send events to POST /api/v1/events with an API key from Developer."}
        </Empty>
      ) : null}
      {q.hasNextPage ? (
        <div className="border-t border-border-subtle p-3">
          <Button
            size="sm"
            onClick={() => void q.fetchNextPage()}
            disabled={q.isFetchingNextPage}
          >
            Load more
          </Button>
        </div>
      ) : null}
      {onOpen ? null : (
        <EventDrawer id={localOpen} onClose={() => setLocalOpen(undefined)} />
      )}
    </>
  );
}

export function EventExplorer() {
  const router = useRouter();
  const params = useSearchParams();
  const openId = params.get("event") ?? undefined;
  const filter = useMemo(() => filterFromParams(params), [params]);

  const navigate = useCallback(
    (f: EventFilter, event?: string) => {
      const q = new URLSearchParams(f as Record<string, string>);
      if (event) q.set("event", event);
      const s = q.toString();
      router.replace((s ? `/events?${s}` : "/events") as "/events", {
        scroll: false,
      });
    },
    [router],
  );

  usePageShortcuts([
    { keys: "j", label: "Next event" },
    { keys: "k", label: "Previous event" },
    { keys: "Enter", label: "Open the event" },
  ]);

  return (
    <>
      <PageHeader
        title="Events"
        description="Every event ingested in this environment, newest first, with its decision."
      />
      <EventFilters filter={filter} onChange={(f) => navigate(f)} />
      <Panel className="overflow-hidden">
        <EventTable
          filter={filter}
          openId={openId}
          onOpen={(id) => navigate(filter, id)}
          shortcuts
        />
      </Panel>
      <EventDrawer id={openId} onClose={() => navigate(filter)} />
    </>
  );
}

/** The actor references of a normalized payload, as profile links. */
function actorLinks(normalized: unknown): [string, React.ReactNode][] {
  const actor = (normalized as { actor?: Record<string, string> } | null)
    ?.actor;
  if (!actor) return [];
  const out: [string, React.ReactNode][] = [];
  if (actor.user_id)
    out.push([
      "User",
      <EntityLink key="u" kind="user" refId={actor.user_id} />,
    ]);
  if (actor.account_id)
    out.push([
      "Account",
      <EntityLink key="a" kind="account" refId={actor.account_id} />,
    ]);
  if (actor.device_id)
    out.push([
      "Device",
      <EntityLink key="d" kind="device" refId={actor.device_id} />,
    ]);
  return out;
}

export function EventDrawer({
  id,
  onClose,
}: {
  id: string | undefined;
  onClose: () => void;
}) {
  const q = useEvent(id);
  const e = q.data;
  return (
    <Drawer
      open={!!id}
      onOpenChange={(o) => !o && onClose()}
      title={e ? humanize(e.type) : "Event"}
      subtitle={e ? <CopyValue value={e.event_key} /> : undefined}
    >
      {q.isLoading ? <Loading /> : null}
      {q.isError ? <ErrorState error={q.error} /> : null}
      {e ? (
        <div className="flex flex-col gap-5">
          <DecisionReading decision={e.evaluation} />
          <KeyValues
            items={[
              ...actorLinks(e.normalized_payload),
              ["Event", <CopyValue key="i" value={e.id} />],
              [
                "Evaluation",
                <CopyValue key="v" value={e.evaluation.evaluation_id} />,
              ],
              [
                "Occurred",
                <span key="o" className="mono">
                  {formatTimestamp(e.occurred_at)}
                </span>,
              ],
              [
                "Received",
                <span key="r" className="mono">
                  {formatTimestamp(e.received_at)}
                </span>,
              ],
              ["Environment", <Tag key="env">{e.environment}</Tag>],
              [
                "Request",
                <CopyValue key="q" value={e.evaluation.request_id} />,
              ],
            ]}
          />
          <Tabs defaultValue="normalized">
            <TabList>
              <Tab value="normalized">Normalized</Tab>
              <Tab value="raw">As received</Tab>
            </TabList>
            <TabPanel value="normalized" className="pt-3">
              <CodeBlock value={e.normalized_payload} />
            </TabPanel>
            <TabPanel value="raw" className="pt-3">
              <CodeBlock value={e.raw_payload} />
            </TabPanel>
          </Tabs>
        </div>
      ) : null}
    </Drawer>
  );
}
