"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";

import { Button } from "@/components/ui/button";
import { PageHeader, Panel } from "@/components/ui/panel";
import { RiskBadge } from "@/components/ui/risk-badge";
import { Empty, ErrorState, SkeletonRows } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import type { Schemas } from "@/lib/api/client";
import {
  formatAge,
  formatAmount,
  formatTimestamp,
  humanize,
} from "@/lib/format";
import { useEvents, type EventFilter } from "@/lib/queries";
import { type RiskBand } from "@/lib/risk";

import { EntityLink } from "../entities/link";
import { EventDrawer, EventFilters } from "../events/events";
import { DecisionPill } from "@/components/ui/pill";

/** Transactions: the transaction events with the fields an analyst scans for. */
export function Transactions() {
  const router = useRouter();
  const params = useSearchParams();
  const openId = params.get("event") ?? undefined;
  const filter = useMemo<EventFilter>(() => {
    const f: EventFilter = { type: "transaction" };
    const decision = params.get("decision");
    const level = params.get("risk_level");
    const user = params.get("user");
    if (decision) f.decision = decision as Schemas["Decision"];
    if (level) f.risk_level = level as Schemas["RiskLevel"];
    if (user) f.user = user;
    return f;
  }, [params]);
  const q = useEvents(50, filter);
  const rows = useMemo(
    () => (q.data?.pages ?? []).flatMap((p) => p.data),
    [q.data],
  );

  const navigate = useCallback(
    (f: EventFilter, event?: string) => {
      const { type: _type, ...rest } = f;
      void _type;
      const s = new URLSearchParams(rest as Record<string, string>);
      if (event) s.set("event", event);
      const qs = s.toString();
      router.replace(
        (qs ? `/transactions?${qs}` : "/transactions") as "/transactions",
        {
          scroll: false,
        },
      );
    },
    [router],
  );

  return (
    <>
      <PageHeader
        title="Transactions"
        description="Money movements as you sent them, with their score and decision. Newest first."
      />
      <EventFilters
        filter={filter}
        showType={false}
        onChange={(f) => navigate({ ...f, type: "transaction" })}
      />
      <Panel className="overflow-hidden">
        <Table aria-label="Transactions">
          <thead>
            <tr>
              <Th>Risk</Th>
              <Th className="text-right">Amount</Th>
              <Th>Direction</Th>
              <Th>User</Th>
              <Th>Beneficiary</Th>
              <Th>Channel</Th>
              <Th>Decision</Th>
              <Th>Received</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={8} /> : null}
            {rows.map((e) => {
              const t = e.transaction;
              return (
                <Tr
                  key={e.id}
                  selected={e.id === openId}
                  className="cursor-pointer"
                  onClick={() => navigate(filter, e.id)}
                >
                  <Td>
                    <RiskBadge
                      band={e.risk_level as RiskBand}
                      score={e.risk_score}
                    />
                  </Td>
                  <DataCell className="text-right whitespace-nowrap">
                    {t ? formatAmount(t.amount, t.currency) : "—"}
                  </DataCell>
                  <Td className="text-body-sm">
                    {t ? humanize(t.direction) : "—"}
                  </Td>
                  <Td>
                    {e.actor ? (
                      <EntityLink kind="user" refId={e.actor.user_id} />
                    ) : null}
                  </Td>
                  <DataCell className="text-text-secondary">
                    {t?.beneficiary_id ?? "—"}
                  </DataCell>
                  <Td className="text-body-sm text-text-secondary">
                    {t?.channel ? humanize(t.channel) : "—"}
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
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rows.length === 0 ? (
          <Empty title="No transactions match">
            Transactions are events of type{" "}
            <code className="mono">transaction</code>.
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
      <EventDrawer id={openId} onClose={() => navigate(filter)} />
    </>
  );
}
