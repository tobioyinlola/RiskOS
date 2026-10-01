"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
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
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import { useEvent, useEvents } from "@/lib/queries";
import { DECISION_LABEL, type Decision, type RiskBand } from "@/lib/risk";

import { DecisionReading } from "../risk/explain";

export function EventExplorer() {
  const router = useRouter();
  const params = useSearchParams();
  const openId = params.get("event") ?? undefined;
  const q = useEvents(50);
  const events = useMemo(
    () => (q.data?.pages ?? []).flatMap((p) => p.data),
    [q.data],
  );
  const [cursor, setCursor] = useState(0);

  const open = useCallback(
    (id: string | undefined) =>
      router.replace((id ? `/events?event=${id}` : "/events") as "/events", {
        scroll: false,
      }),
    [router],
  );

  usePageShortcuts([
    { keys: "j", label: "Next event" },
    { keys: "k", label: "Previous event" },
    { keys: "Enter", label: "Open the event" },
  ]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e) || e.metaKey || e.ctrlKey || e.altKey || openId) return;
      if (e.key === "j")
        setCursor((c) => Math.min(c + 1, Math.max(0, events.length - 1)));
      else if (e.key === "k") setCursor((c) => Math.max(0, c - 1));
      else if (e.key === "Enter" && events[cursor]) open(events[cursor].id);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [events, cursor, open, openId]);

  return (
    <>
      <PageHeader
        title="Events"
        description="Every event ingested in this environment, newest first, with its decision."
      />
      <Panel className="overflow-hidden">
        <Table aria-label="Events">
          <thead>
            <tr>
              <Th>Risk</Th>
              <Th>Event</Th>
              <Th>Type</Th>
              <Th>Decision</Th>
              <Th>Occurred</Th>
              <Th>Received</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {events.map((e, i) => (
              <Tr
                key={e.id}
                density="compact"
                selected={i === cursor || e.id === openId}
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
                <Td className="text-body-sm">
                  {DECISION_LABEL[e.decision as Decision]}
                </Td>
                <DataCell className="text-text-secondary">
                  {formatTimestamp(e.occurred_at)}
                </DataCell>
                <DataCell className="text-text-muted">
                  {formatAge(e.received_at)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && events.length === 0 ? (
          <Empty title="No events yet">
            Send events to <code className="mono">POST /api/v1/events</code>{" "}
            with an API key from Developer.
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
      </Panel>
      <EventDrawer id={openId} onClose={() => open(undefined)} />
    </>
  );
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
