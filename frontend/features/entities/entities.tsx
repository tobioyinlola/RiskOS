"use client";

import { cn } from "cn";
import { Smartphone, UserRound, Wallet } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { CopyValue } from "@/components/ui/misc";
import { Metric, PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { RiskBadge } from "@/components/ui/risk-badge";
import {
  Empty,
  ErrorState,
  Loading,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import type { Schemas } from "@/lib/api/client";
import { ApiError } from "@/lib/api/client";
import { formatAge, formatCount, formatTimestamp } from "@/lib/format";
import {
  useAnalytics,
  useEntity,
  type EntityKind,
  type EventFilter,
} from "@/lib/queries";

import { EventTable } from "../events/events";

import { DenyEntityButton } from "../deny/deny";
import { entityHref, EntityLink } from "./link";

export { entityHref, EntityLink };

export const KIND_LABEL: Record<EntityKind, string> = {
  user: "User",
  account: "Account",
  device: "Device",
};

const KIND_ICON: Record<EntityKind, typeof UserRound> = {
  user: UserRound,
  account: Wallet,
  device: Smartphone,
};

/** Look up a user, account or device; below, the highest-risk users of the last 24 hours. */
export function EntityLookup() {
  const router = useRouter();
  const [kind, setKind] = useState<EntityKind>("user");
  const [ref, setRef] = useState("");
  const top = useAnalytics("24h");
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (ref.trim()) router.push(entityHref(kind, ref.trim()));
  };
  return (
    <>
      <PageHeader
        title="Entities"
        description="Users, accounts and devices as RiskOS has seen them: activity, scores and what they are linked to."
      />
      <Panel className="mb-5">
        <form
          onSubmit={onSubmit}
          className="flex flex-wrap items-end gap-3 p-4"
        >
          <Field label="Kind" className="w-40">
            {(p) => (
              <Select
                {...p}
                value={kind}
                onChange={(e) => setKind(e.target.value as EntityKind)}
              >
                <option value="user">User</option>
                <option value="account">Account</option>
                <option value="device">Device</option>
              </Select>
            )}
          </Field>
          <Field label="Identifier" className="min-w-64 flex-1">
            {(p) => (
              <Input
                {...p}
                value={ref}
                onChange={(e) => setRef(e.target.value)}
                placeholder="The actor ID you send, e.g. usr_4812"
                className="mono"
                autoFocus
              />
            )}
          </Field>
          <Button type="submit" variant="primary" disabled={!ref.trim()}>
            Open profile
          </Button>
        </form>
      </Panel>
      <Panel className="overflow-hidden">
        <PanelHeader title="Highest-risk users" legend="last 24 hours" />
        <TopUsers rows={top.data?.top_users} loading={top.isLoading} />
        {top.isError ? <ErrorState error={top.error} className="m-4" /> : null}
      </Panel>
    </>
  );
}

export function TopUsers({
  rows,
  loading,
}: {
  rows: Schemas["TopUser"][] | undefined;
  loading?: boolean;
}) {
  return (
    <>
      <Table aria-label="Highest-risk users">
        <thead>
          <tr>
            <Th>Highest score</Th>
            <Th>User</Th>
            <Th className="text-right">Events</Th>
            <Th className="text-right">Reviewed or blocked</Th>
            <Th>Last seen</Th>
          </tr>
        </thead>
        <tbody>
          {loading ? <SkeletonRows columns={5} rows={4} /> : null}
          {(rows ?? []).map((u) => (
            <Tr key={u.user_id}>
              <Td>
                <RiskBadge score={u.max_score} />
              </Td>
              <Td>
                <EntityLink kind="user" refId={u.user_id} />
              </Td>
              <DataCell className="text-right">
                {formatCount(u.events)}
              </DataCell>
              <DataCell className="text-right">
                {formatCount(u.flagged)}
              </DataCell>
              <DataCell
                className="text-text-muted"
                title={formatTimestamp(u.last_seen)}
              >
                {formatAge(u.last_seen)}
              </DataCell>
            </Tr>
          ))}
        </tbody>
      </Table>
      {rows && rows.length === 0 ? (
        <Empty title="No events in this window" />
      ) : null}
    </>
  );
}

function LinkedPanel({
  title,
  kind,
  items,
}: {
  title: string;
  kind?: EntityKind;
  items: Schemas["LinkedEntity"][];
}) {
  return (
    <Panel className="overflow-hidden">
      <PanelHeader title={title} legend={formatCount(items.length)} />
      {items.length === 0 ? (
        <p className="px-4 py-3 text-body-sm text-text-muted">None seen.</p>
      ) : (
        <Table aria-label={title}>
          <tbody>
            {items.map((l) => (
              <Tr key={l.ref} density="compact">
                <Td>
                  {kind ? (
                    <EntityLink kind={kind} refId={l.ref} />
                  ) : (
                    <span className="mono text-data">{l.ref}</span>
                  )}
                </Td>
                <DataCell className="text-right text-text-secondary">
                  {formatCount(l.events)}
                </DataCell>
                <DataCell
                  className="w-16 text-right text-text-muted"
                  title={formatTimestamp(l.last_seen)}
                >
                  {formatAge(l.last_seen)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </Panel>
  );
}

/** One entity: activity, scores, decisions, links and its events. */
export function EntityProfile({
  kind,
  refId,
}: {
  kind: EntityKind;
  refId: string;
}) {
  const q = useEntity(kind, refId);
  const Icon = KIND_ICON[kind];
  const filter: EventFilter = { [kind]: refId };
  const header = (
    <PageHeader
      title={`${KIND_LABEL[kind]} profile`}
      actions={<DenyEntityButton kind={kind} refId={refId} />}
      description={
        <span className="inline-flex items-center gap-2">
          <Icon
            className="size-icon-sm stroke-[1.5] text-text-muted"
            aria-hidden="true"
          />
          <CopyValue value={refId} />
        </span>
      }
    />
  );
  if (q.isLoading) {
    return (
      <>
        {header}
        <Loading label="Loading the profile" />
      </>
    );
  }
  if (q.isError) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    return (
      <>
        {header}
        {notFound ? (
          <Panel>
            <Empty
              title={`No events for this ${kind}`}
              action={
                <Button asChild>
                  <Link href="/entities">Look up another</Link>
                </Button>
              }
            >
              This environment has no events with this identifier. Identifiers
              are matched exactly, as you send them.
            </Empty>
          </Panel>
        ) : (
          <ErrorState error={q.error} />
        )}
      </>
    );
  }
  const p = q.data!;
  const d = p.decisions;
  const linked: {
    title: string;
    kind?: EntityKind;
    items: Schemas["LinkedEntity"][];
    show: boolean;
  }[] = [
    {
      title: "Users",
      kind: "user",
      items: p.linked.users,
      show: kind !== "user",
    },
    {
      title: "Accounts",
      kind: "account",
      items: p.linked.accounts,
      show: kind !== "account",
    },
    {
      title: "Devices",
      kind: "device",
      items: p.linked.devices,
      show: kind !== "device",
    },
    { title: "Countries", items: p.linked.countries, show: true },
    { title: "IP addresses", items: p.linked.ips, show: true },
  ];
  return (
    <>
      {header}
      <div className="mb-5 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Metric label="Events" value={formatCount(p.event_count)}>
          <span className="text-body-sm text-text-muted">
            since {formatTimestamp(p.first_seen)}
          </span>
        </Metric>
        <Metric label="Highest score" value={p.max_score} unit="/ 100">
          <RiskBadge
            score={p.max_score}
            showScore={false}
            className="self-start"
          />
        </Metric>
        <Metric label="Mean score" value={p.mean_score} unit="/ 100" />
        <Metric label="Last seen" value={formatAge(p.last_seen)}>
          <span className="text-body-sm text-text-muted">
            {formatTimestamp(p.last_seen)}
          </span>
        </Metric>
      </div>
      <Panel className="mb-5 overflow-hidden">
        <PanelHeader
          title="Decisions"
          legend={
            p.event_count >= 5000 ? "most recent 5,000 events" : undefined
          }
        />
        <DecisionBar counts={d} />
      </Panel>
      <div className="mb-5 grid grid-cols-1 gap-5 lg:grid-cols-2 xl:grid-cols-3">
        {linked
          .filter((l) => l.show)
          .map((l) => (
            <LinkedPanel
              key={l.title}
              title={l.title}
              kind={l.kind}
              items={l.items}
            />
          ))}
      </div>
      <Panel className="overflow-hidden">
        <PanelHeader title="Events" legend="newest first" />
        <EventTable filter={filter} />
      </Panel>
    </>
  );
}

/** Decisions as one proportional bar with a legend (and the counts as text). */
export function DecisionBar({ counts }: { counts: Schemas["DecisionCounts"] }) {
  const parts: { key: string; label: string; n: number; cls: string }[] = [
    // Decisions are not bands: a neutral ramp, with the words carrying the meaning.
    { key: "allow", label: "Allow", n: counts.allow, cls: "bg-border-control" },
    {
      key: "step_up",
      label: "Step-up",
      n: counts.step_up,
      cls: "bg-text-muted",
    },
    {
      key: "review",
      label: "Review",
      n: counts.review,
      cls: "bg-text-secondary",
    },
    { key: "block", label: "Block", n: counts.block, cls: "bg-text-primary" },
  ];
  const total = parts.reduce((s, p) => s + p.n, 0) || 1;
  return (
    <div className="flex flex-col gap-3 p-4">
      <div
        className="flex h-2 gap-0.5 overflow-hidden rounded-full bg-bg-sunken"
        aria-hidden="true"
      >
        {parts
          .filter((p) => p.n > 0)
          .map((p) => (
            <span key={p.key} className={p.cls} style={{ flexGrow: p.n }} />
          ))}
      </div>
      <dl className="flex flex-wrap gap-x-5 gap-y-1 text-body-sm">
        {parts.map((p) => (
          <div key={p.key} className="flex items-center gap-2">
            <span
              className={cn("size-2 rounded-full", p.cls)}
              aria-hidden="true"
            />
            <dt className="text-text-secondary">{p.label}</dt>
            <dd className="mono text-data">
              {formatCount(p.n)}{" "}
              <span className="text-text-muted">
                ({Math.round((p.n / total) * 100)}%)
              </span>
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
