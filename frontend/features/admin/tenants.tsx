"use client";

import { useQuery } from "@tanstack/react-query";
import type { Route } from "next";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { KeyValues, Status } from "@/components/ui/misc";
import { Metric, PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { Empty, ErrorState, SkeletonRows } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi, type AdminSchemas } from "@/lib/api/admin-client";
import { unwrap } from "@/lib/api/client";
import { formatCount, formatTimestamp, humanize } from "@/lib/format";

import { SecurityOverview } from "./pages";
import { TenantLimitsPanel } from "./tenant-limits";
import { TenantQuotaPanel } from "./tenant-quota";
import { TenantStatusPanel } from "./tenant-status";

type Tenant = AdminSchemas["AdminTenant"];

function statusTone(status: Tenant["status"]): "ok" | "warn" | "off" {
  if (status === "active") return "ok";
  if (status === "suspended") return "warn";
  return "off";
}

/** The work queue: what waits for this staff member, each count a link to the page that clears it. */
export function AdminOverviewPage() {
  const q = useQuery({
    queryKey: ["staff", "overview"],
    queryFn: async () => unwrap(await adminApi().GET("/api/admin/v1/overview")),
    refetchInterval: 30_000,
  });
  const o = q.data;
  const card = (
    href: Route,
    label: string,
    value: number | undefined,
    note: string,
  ) => (
    <Link
      href={href}
      className="rounded-lg focus-visible:outline-2 focus-visible:outline-offset-2"
      aria-label={`${label}: ${value ?? "loading"}. ${note}`}
    >
      <Metric
        label={label}
        value={value === undefined ? "–" : formatCount(value)}
      >
        <span className="text-label text-text-muted">{note}</span>
      </Metric>
    </Link>
  );
  return (
    <>
      <PageHeader
        title="Overview"
        description="What waits for you, and the state of the platform. Counts only: tenant data is reached through approved support access."
      />
      {q.isError ? <ErrorState error={q.error} className="mb-5" /> : null}
      <div className="mb-8 grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
        {card(
          "/admin/onboarding",
          "Onboarding reviews",
          o?.onboarding_submitted,
          "waiting for a decision",
        )}
        {card(
          "/admin/status-requests",
          "Status requests",
          o
            ? o.status_requests_pending + (o.emergency_reviews_pending ?? 0)
            : undefined,
          "to decide, and emergency suspensions to review",
        )}
        {card(
          "/admin/support",
          "Support requests",
          o?.support_pending,
          "from others, for you to decide",
        )}
        {card(
          "/admin/tenants",
          "Tenants",
          o?.tenants_total,
          o
            ? `${formatCount(o.tenants_active)} active, ${formatCount(o.tenants_suspended)} suspended`
            : "",
        )}
        {card(
          "/admin/support",
          "Support access in use",
          o?.support_active,
          "approved and not ended",
        )}
      </div>
      <SecurityOverview embedded />
    </>
  );
}

/** The tenant directory: identity, status and counts, searchable. Never tenant data. */
export function TenantsPage() {
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const q = useQuery({
    queryKey: ["staff", "tenants"],
    queryFn: async () => unwrap(await adminApi().GET("/api/admin/v1/tenants")),
    refetchInterval: 60_000,
  });
  const needle = search.trim().toLowerCase();
  const rows = (q.data?.data ?? []).filter(
    (t) =>
      (status === "" || t.status === status) &&
      (needle === "" ||
        t.name.toLowerCase().includes(needle) ||
        t.id.toLowerCase().includes(needle)),
  );
  return (
    <>
      <PageHeader
        title="Tenants"
        description="Identity, status and counts. Nothing a tenant stores is shown here; request support access to read it."
      />
      <Panel className="overflow-hidden">
        <div className="flex flex-wrap items-end gap-3 border-b border-border-subtle p-4">
          <Field label="Search" className="min-w-64 flex-1">
            {(p) => (
              <Input
                {...p}
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Name or ID"
              />
            )}
          </Field>
          <Field label="Status" className="w-44">
            {(p) => (
              <Select
                {...p}
                value={status}
                onChange={(e) => setStatus(e.target.value)}
              >
                <option value="">All</option>
                <option value="active">Active</option>
                <option value="suspended">Suspended</option>
                <option value="disabled">Disabled</option>
              </Select>
            )}
          </Field>
        </div>
        <Table aria-label="Tenants">
          <thead>
            <tr>
              <Th>Tenant</Th>
              <Th>Status</Th>
              <Th>Origin</Th>
              <Th>Production</Th>
              <Th className="text-right">Users</Th>
              <Th className="text-right">Keys</Th>
              <Th>Created</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={7} /> : null}
            {rows.map((t) => (
              <Tr key={t.id}>
                <Td className="text-body-sm">
                  <Link
                    href={`/admin/tenants/${t.id}`}
                    className="font-medium underline-offset-2 hover:underline"
                  >
                    {t.name}
                  </Link>
                  <span className="mono block text-label text-text-muted">
                    {t.id}
                  </span>
                </Td>
                <Td>
                  <Status tone={statusTone(t.status)}>
                    {humanize(t.status)}
                  </Status>
                </Td>
                <Td className="text-body-sm">{humanize(t.origin)}</Td>
                <Td className="text-body-sm">
                  {t.production_enabled ? "Open" : "Locked"}
                </Td>
                <DataCell>{formatCount(t.active_users)}</DataCell>
                <DataCell>{formatCount(t.active_api_keys)}</DataCell>
                <Td className="text-body-sm text-text-secondary">
                  {formatTimestamp(t.created_at)}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rows.length === 0 ? (
          <Empty
            title={needle || status ? "No tenant matches" : "No tenants yet"}
          />
        ) : null}
      </Panel>
    </>
  );
}

/** One tenant: state, counts, security signals and what staff have done to it. */
export function TenantDetailPage({ id }: { id: string }) {
  const q = useQuery({
    queryKey: ["staff", "tenant", id],
    queryFn: async () =>
      unwrap(
        await adminApi().GET("/api/admin/v1/tenants/{tenant_id}", {
          params: { path: { tenant_id: id } },
        }),
      ),
  });
  if (q.isError) return <ErrorState error={q.error} />;
  const d = q.data;
  const t = d?.tenant;
  const sec = d?.security;
  return (
    <>
      <PageHeader
        title={t?.name ?? "Tenant"}
        description={
          t
            ? `${t.id} · ${humanize(t.origin)} workspace in ${t.region}`
            : undefined
        }
        actions={
          <Button asChild variant="secondary">
            <Link href="/admin/tenants">All tenants</Link>
          </Button>
        }
      />
      <div className="mb-5 grid gap-5 lg:grid-cols-2">
        <Panel>
          <PanelHeader title="State" />
          <div className="p-4">
            {t ? (
              <KeyValues
                items={[
                  [
                    "Status",
                    <Status key="s" tone={statusTone(t.status)}>
                      {humanize(t.status)}
                    </Status>,
                  ],
                  ["Production", t.production_enabled ? "Open" : "Locked"],
                  [
                    "Onboarding",
                    t.onboarding_status
                      ? humanize(t.onboarding_status)
                      : "Not required",
                  ],
                  ["Active users", formatCount(t.active_users)],
                  ["Active API keys", formatCount(t.active_api_keys)],
                  ["Created", formatTimestamp(t.created_at)],
                ]}
              />
            ) : (
              <SkeletonRows columns={1} />
            )}
          </div>
        </Panel>
        <Panel>
          <PanelHeader
            title="Security signals"
            legend={d ? `Since ${formatTimestamp(d.since)}` : "Last 24 hours"}
          />
          <div className="p-4">
            {sec ? (
              <KeyValues
                items={[
                  [
                    "Open security alerts",
                    formatCount(sec.open_security_alerts),
                  ],
                  [
                    "Degraded evaluations",
                    formatCount(sec.degraded_evaluations),
                  ],
                  ["Failed sign-ins", formatCount(sec.failed_logins)],
                  ["Disabled webhooks", formatCount(sec.disabled_webhooks)],
                ]}
              />
            ) : (
              <SkeletonRows columns={1} />
            )}
          </div>
        </Panel>
      </div>
      {t ? <TenantStatusPanel tenant={t} /> : null}
      {t ? <TenantLimitsPanel tenantId={t.id} /> : null}
      {t ? <TenantQuotaPanel tenantId={t.id} /> : null}
      <Panel className="overflow-hidden">
        <PanelHeader
          title="Staff activity on this tenant"
          legend="Most recent entries of the staff audit log that name it"
        />
        <Table aria-label="Staff activity on this tenant">
          <thead>
            <tr>
              <Th>When</Th>
              <Th>Who</Th>
              <Th>Action</Th>
              <Th>Outcome</Th>
            </tr>
          </thead>
          <tbody>
            {(d?.recent_staff_actions ?? []).map((a) => (
              <Tr key={a.id}>
                <Td className="text-body-sm">
                  {formatTimestamp(a.occurred_at)}
                </Td>
                <Td className="text-body-sm">{a.actor}</Td>
                <Td className="mono text-data">{a.action}</Td>
                <Td>
                  <Status tone={a.outcome === "success" ? "ok" : "error"}>
                    {humanize(a.outcome)}
                  </Status>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {d && d.recent_staff_actions.length === 0 ? (
          <Empty title="No staff activity on this tenant" />
        ) : null}
      </Panel>
    </>
  );
}
