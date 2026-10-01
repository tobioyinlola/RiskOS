"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Select, Textarea } from "@/components/ui/field";
import { Status, Tag } from "@/components/ui/misc";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi, type AdminSchemas } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatCount, formatTimestamp, humanize } from "@/lib/format";

import { useStaff } from "./shell";

type Access = AdminSchemas["SupportAccess"];

/** Security signals across tenants: counts only, never tenant data. */
export function SecurityOverview() {
  const q = useQuery({
    queryKey: ["staff", "security"],
    queryFn: async () => unwrap(await adminApi().GET("/api/admin/v1/security")),
    refetchInterval: 60_000,
  });
  const rows = [...(q.data?.data ?? [])].sort(
    (a, b) =>
      b.open_security_alerts +
      b.degraded_evaluations +
      b.failed_logins -
      (a.open_security_alerts + a.degraded_evaluations + a.failed_logins),
  );
  const cell = (n: number) => (
    <DataCell className={n > 0 ? "text-risk-high" : "text-text-muted"}>
      {formatCount(n)}
    </DataCell>
  );
  return (
    <>
      <PageHeader
        title="Security across tenants"
        description={`Counts per tenant since ${q.data ? formatTimestamp(q.data.since) : "24 hours ago"}. Security alerts and disabled webhook endpoints are as of now. No tenant data is shown.`}
      />
      <Panel className="overflow-hidden">
        <Table aria-label="Security across tenants">
          <thead>
            <tr>
              <Th>Tenant</Th>
              <Th>Status</Th>
              <Th className="text-right">Open security alerts</Th>
              <Th className="text-right">Degraded evaluations</Th>
              <Th className="text-right">Failed sign-ins</Th>
              <Th className="text-right">Disabled webhooks</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {rows.map((r) => (
              <Tr key={r.tenant_id}>
                <Td className="text-body-sm">
                  {r.tenant_name}
                  <span className="mono block text-label text-text-muted">
                    {r.tenant_id}
                  </span>
                </Td>
                <Td>
                  <Status tone={r.tenant_status === "active" ? "ok" : "off"}>
                    {humanize(r.tenant_status)}
                  </Status>
                </Td>
                {cell(r.open_security_alerts)}
                {cell(r.degraded_evaluations)}
                {cell(r.failed_logins)}
                {cell(r.disabled_webhooks)}
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rows.length === 0 ? <Empty title="No tenants" /> : null}
      </Panel>
    </>
  );
}

const DURATIONS = [30, 60, 120, 240];

function statusTone(a: Access): "ok" | "warn" | "error" | "off" {
  if (a.active) return "ok";
  if (a.status === "pending") return "warn";
  return "off";
}

/** Support access: request, four-eyes approval, revocation. */
export function SupportAccessPage() {
  const me = useStaff();
  const qc = useQueryClient();
  const list = useQuery({
    queryKey: ["staff", "support"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/support-access")),
    refetchInterval: 30_000,
  });
  const tenants = useQuery({
    queryKey: ["staff", "tenants"],
    queryFn: async () => unwrap(await adminApi().GET("/api/admin/v1/tenants")),
  });
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: ["staff", "support"] });
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The action failed.");
      return false;
    } finally {
      setBusy(false);
    }
  };

  const onRequest = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = e.currentTarget;
    const f = new FormData(form);
    const ok = await act(async () =>
      unwrap(
        await adminApi().POST("/api/admin/v1/support-access", {
          body: {
            tenant_id: String(f.get("tenant")),
            reason: String(f.get("reason")),
            duration_minutes: Number(f.get("duration")),
          },
        }),
      ),
    );
    if (ok) form.reset();
  };

  const decide = (id: string, verb: "approve" | "reject" | "revoke") =>
    void act(async () => {
      const opts = { params: { path: { access_id: id } } };
      const c = adminApi();
      if (verb === "approve")
        return unwrap(
          await c.POST(
            "/api/admin/v1/support-access/{access_id}/approve",
            opts,
          ),
        );
      if (verb === "reject")
        return unwrap(
          await c.POST("/api/admin/v1/support-access/{access_id}/reject", opts),
        );
      return unwrap(
        await c.POST("/api/admin/v1/support-access/{access_id}/revoke", opts),
      );
    });

  return (
    <>
      <PageHeader
        title="Support access"
        description="Read-only access to one tenant, for at most four hours, approved by another staff member. The tenant's audit log records the request, the approval and every page read."
      />
      {error ? <Notice tone="error">{error}</Notice> : null}
      <Panel className="mb-5">
        <PanelHeader title="Request access" />
        <form
          onSubmit={(e) => void onRequest(e)}
          className="flex flex-col gap-4 p-4"
        >
          <div className="flex flex-wrap items-end gap-3">
            <Field label="Tenant" className="min-w-64 flex-1">
              {(p) => (
                <Select {...p} name="tenant" required>
                  {(tenants.data?.data ?? []).map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name} ({t.status})
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="For" className="w-40">
              {(p) => (
                <Select {...p} name="duration" defaultValue="60">
                  {DURATIONS.map((d) => (
                    <option key={d} value={d}>
                      {d < 60
                        ? `${d} minutes`
                        : `${d / 60} hour${d > 60 ? "s" : ""}`}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          </div>
          <Field
            label="Reason"
            hint="The customer sees it in their audit log. Name the ticket."
          >
            {(p) => (
              <Textarea
                {...p}
                name="reason"
                minLength={10}
                maxLength={500}
                rows={2}
                required
              />
            )}
          </Field>
          <Button
            type="submit"
            variant="primary"
            className="self-start"
            disabled={busy}
          >
            Request access
          </Button>
        </form>
      </Panel>
      <Panel className="overflow-hidden">
        <Table aria-label="Support access requests">
          <thead>
            <tr>
              <Th>Tenant</Th>
              <Th>Requested by</Th>
              <Th>Reason</Th>
              <Th>Status</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {list.isLoading ? <SkeletonRows columns={5} /> : null}
            {(list.data?.data ?? []).map((a) => {
              const mine = a.requested_by.id === me.staff.id;
              return (
                <Tr key={a.id}>
                  <Td className="text-body-sm">{a.tenant_name}</Td>
                  <Td className="text-body-sm">
                    {a.requested_by.name}
                    <span className="block text-label text-text-muted">
                      {formatTimestamp(a.created_at)} · {a.duration_minutes} min
                    </span>
                  </Td>
                  <Td
                    className="max-w-80 truncate text-body-sm text-text-secondary"
                    title={a.reason}
                  >
                    {a.reason}
                  </Td>
                  <Td>
                    <Status tone={statusTone(a)}>{humanize(a.status)}</Status>
                    {a.active && a.expires_at ? (
                      <span className="block text-label text-text-muted">
                        until {formatTimestamp(a.expires_at)}
                      </span>
                    ) : null}
                    {a.decided_by ? (
                      <span className="block text-label text-text-muted">
                        by {a.decided_by.name}
                      </span>
                    ) : null}
                  </Td>
                  <Td className="text-right">
                    <div className="flex flex-wrap justify-end gap-2">
                      {a.status === "pending" && !mine ? (
                        <>
                          <Button
                            size="sm"
                            variant="primary"
                            disabled={busy}
                            onClick={() => decide(a.id, "approve")}
                          >
                            Approve
                          </Button>
                          <Button
                            size="sm"
                            disabled={busy}
                            onClick={() => decide(a.id, "reject")}
                          >
                            Reject
                          </Button>
                        </>
                      ) : null}
                      {a.status === "pending" && mine ? (
                        <Tag>Awaiting another staff member</Tag>
                      ) : null}
                      {a.active && mine ? (
                        <Link
                          href={`/admin/support/${a.id}`}
                          className="rounded-sm px-2 py-1 text-body-sm text-accent hover:underline"
                        >
                          Open read-only view
                        </Link>
                      ) : null}
                      {a.active || a.status === "pending" ? (
                        <Button
                          size="sm"
                          variant="danger"
                          disabled={busy}
                          onClick={() => decide(a.id, "revoke")}
                        >
                          {a.active ? "End access" : "Withdraw"}
                        </Button>
                      ) : null}
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {list.isError ? (
          <ErrorState error={list.error} className="m-4" />
        ) : null}
        {list.isSuccess && list.data.data.length === 0 ? (
          <Empty title="No support access requested yet" />
        ) : null}
      </Panel>
    </>
  );
}

/** The staff audit log. */
export function StaffAuditPage() {
  const q = useQuery({
    queryKey: ["staff", "audit"],
    queryFn: async () =>
      unwrap(
        await adminApi().GET("/api/admin/v1/audit-log", {
          params: { query: { limit: 200 } },
        }),
      ),
  });
  return (
    <>
      <PageHeader
        title="Staff audit log"
        description="Every staff action, newest first. Append-only."
      />
      <Panel className="overflow-hidden">
        <Table aria-label="Staff audit log">
          <thead>
            <tr>
              <Th>When</Th>
              <Th>Actor</Th>
              <Th>Action</Th>
              <Th>Resource</Th>
              <Th>Outcome</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={5} /> : null}
            {(q.data?.data ?? []).map((e) => (
              <Tr key={e.id} density="compact">
                <DataCell className="text-text-muted">
                  {formatTimestamp(e.occurred_at)}
                </DataCell>
                <DataCell>{e.actor}</DataCell>
                <Td className="mono text-data">{e.action}</Td>
                <DataCell className="text-text-muted">{e.resource_id}</DataCell>
                <Td>
                  <Status tone={e.outcome === "success" ? "ok" : "error"}>
                    {humanize(e.outcome)}
                  </Status>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      </Panel>
    </>
  );
}
