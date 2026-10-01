"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useMemo, useState } from "react";

import { RiskBadge } from "@/components/ui/risk-badge";
import { Status, Tag } from "@/components/ui/misc";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { Select } from "@/components/ui/field";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi, supportApi } from "@/lib/api/admin-client";
import { unwrap, type Environment } from "@/lib/api/client";
import { formatTimestamp, humanize } from "@/lib/format";

/**
 * Read-only view of one tenant through approved support access. Every request is recorded in
 * the tenant's audit log; the API refuses anything but reads.
 */
export function SupportView({ id }: { id: string }) {
  const [env, setEnv] = useState<Environment>("production");
  const access = useQuery({
    queryKey: ["staff", "support"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/support-access")),
    select: (d) => d.data.find((a) => a.id === id),
  });
  const client = useMemo(() => supportApi(id, env), [id, env]);
  const active = access.data?.active ?? false;
  const events = useQuery({
    queryKey: ["staff", "support-view", id, env, "events"],
    enabled: active,
    queryFn: async () =>
      unwrap(
        await client.GET("/api/v1/events", {
          params: { query: { limit: 25 } },
        }),
      ),
  });
  const alerts = useQuery({
    queryKey: ["staff", "support-view", id, env, "alerts"],
    enabled: active,
    queryFn: async () => unwrap(await client.GET("/api/v1/alerts")),
  });
  const audit = useQuery({
    queryKey: ["staff", "support-view", id, "audit"],
    enabled: active,
    queryFn: async () =>
      unwrap(
        await client.GET("/api/v1/audit-logs", {
          params: { query: { limit: 25 } },
        }),
      ),
  });

  if (access.isLoading) return <SkeletonRows columns={3} />;
  if (!access.data || !active) {
    return (
      <Notice tone="warn">
        This support access is not active (it may be pending, ended or someone
        else&apos;s). <Link href="/admin/support">Back to support access</Link>
      </Notice>
    );
  }
  const a = access.data;
  return (
    <>
      <PageHeader
        title={`${a.tenant_name} (read-only)`}
        description={`Support access until ${a.expires_at ? formatTimestamp(a.expires_at) : "—"}. ${a.tenant_name} sees every page you read in its audit log.`}
        actions={
          <Select
            aria-label="Environment"
            className="h-control-sm w-40 text-body-sm"
            value={env}
            onChange={(e) => setEnv(e.target.value as Environment)}
          >
            <option value="production">Production</option>
            <option value="test">Test</option>
          </Select>
        }
      />
      <div className="flex flex-col gap-5">
        <Panel className="overflow-hidden">
          <PanelHeader title="Recent events" />
          <Table aria-label="Recent events">
            <thead>
              <tr>
                <Th>Received</Th>
                <Th>Type</Th>
                <Th>User</Th>
                <Th>Risk</Th>
                <Th>Decision</Th>
              </tr>
            </thead>
            <tbody>
              {events.isLoading ? <SkeletonRows columns={5} /> : null}
              {(events.data?.data ?? []).map((e) => (
                <Tr key={e.id} density="compact">
                  <DataCell className="text-text-muted">
                    {formatTimestamp(e.received_at)}
                  </DataCell>
                  <Td className="text-body-sm">{humanize(e.type)}</Td>
                  <DataCell>{e.actor?.user_id ?? "—"}</DataCell>
                  <Td>
                    <RiskBadge
                      band={e.risk_level}
                      score={e.risk_score}
                      showScore
                    />
                  </Td>
                  <Td>
                    <Tag>{humanize(e.decision)}</Tag>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
          {events.isError ? (
            <ErrorState error={events.error} className="m-4" />
          ) : null}
          {events.isSuccess && events.data.data.length === 0 ? (
            <Empty title="No events in this environment" />
          ) : null}
        </Panel>
        <Panel className="overflow-hidden">
          <PanelHeader title="Alerts" />
          <Table aria-label="Alerts">
            <thead>
              <tr>
                <Th>Raised</Th>
                <Th>Kind</Th>
                <Th>Title</Th>
                <Th>Status</Th>
              </tr>
            </thead>
            <tbody>
              {alerts.isLoading ? <SkeletonRows columns={4} /> : null}
              {(alerts.data?.data ?? []).map((al) => (
                <Tr key={al.id} density="compact">
                  <DataCell className="text-text-muted">
                    {formatTimestamp(al.created_at)}
                  </DataCell>
                  <Td>
                    <Tag>{humanize(al.kind)}</Tag>
                  </Td>
                  <Td className="text-body-sm">{al.title}</Td>
                  <Td>
                    <Status tone={al.status === "resolved" ? "off" : "warn"}>
                      {humanize(al.status)}
                    </Status>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
          {alerts.isError ? (
            <ErrorState error={alerts.error} className="m-4" />
          ) : null}
        </Panel>
        <Panel className="overflow-hidden">
          <PanelHeader title="Tenant audit log" />
          <Table aria-label="Tenant audit log">
            <thead>
              <tr>
                <Th>When</Th>
                <Th>Actor</Th>
                <Th>Action</Th>
              </tr>
            </thead>
            <tbody>
              {(audit.data?.data ?? []).map((e) => (
                <Tr key={e.id} density="compact">
                  <DataCell className="text-text-muted">
                    {formatTimestamp(e.occurred_at)}
                  </DataCell>
                  <DataCell>
                    {e.actor.type}:{e.actor.id}
                  </DataCell>
                  <Td className="mono text-data">{e.action}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
          {audit.isError ? (
            <ErrorState error={audit.error} className="m-4" />
          ) : null}
        </Panel>
      </div>
    </>
  );
}
