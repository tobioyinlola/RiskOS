"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Textarea } from "@/components/ui/field";
import { Status, Tag } from "@/components/ui/misc";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi, type AdminSchemas } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatTimestamp, humanize } from "@/lib/format";

import { useStaff } from "./shell";

type StatusRequest = AdminSchemas["TenantStatusRequest"];
type Tenant = AdminSchemas["AdminTenant"];

function tone(r: StatusRequest): "ok" | "warn" | "error" | "off" {
  if (r.status === "pending") return "warn";
  if (r.status === "approved") return "ok";
  return "off";
}

const VERBS = {
  suspend: { noun: "suspension", button: "Request suspension" },
  reactivate: { noun: "reactivation", button: "Request reactivation" },
} as const;

/** Runs a status-change action and keeps the lists fresh; shared by the page and the panel. */
function useStatusActions() {
  const qc = useQueryClient();
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["staff", "status-requests"] }),
        qc.invalidateQueries({ queryKey: ["staff", "tenants"] }),
        qc.invalidateQueries({ queryKey: ["staff", "tenant"] }),
        qc.invalidateQueries({ queryKey: ["staff", "overview"] }),
      ]);
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The action failed.");
      return false;
    } finally {
      setBusy(false);
    }
  };
  return { act, busy, error };
}

/** Requests to suspend or reactivate tenants: a different staff member decides. */
export function TenantStatusRequestsPage() {
  const me = useStaff();
  const { act, busy, error } = useStatusActions();
  const q = useQuery({
    queryKey: ["staff", "status-requests"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/tenant-status-requests")),
    refetchInterval: 30_000,
  });
  const decide = (
    id: string,
    verb: "approve" | "reject" | "withdraw" | "review",
  ) =>
    void act(async () => {
      const opts = { params: { path: { request_id: id } } };
      const c = adminApi();
      if (verb === "review")
        return unwrap(
          await c.POST(
            "/api/admin/v1/tenant-status-requests/{request_id}/review",
            opts,
          ),
        );
      if (verb === "approve")
        return unwrap(
          await c.POST(
            "/api/admin/v1/tenant-status-requests/{request_id}/approve",
            opts,
          ),
        );
      if (verb === "reject")
        return unwrap(
          await c.POST(
            "/api/admin/v1/tenant-status-requests/{request_id}/reject",
            opts,
          ),
        );
      return unwrap(
        await c.POST(
          "/api/admin/v1/tenant-status-requests/{request_id}/withdraw",
          opts,
        ),
      );
    });
  return (
    <>
      <PageHeader
        title="Tenant status requests"
        description="Suspending or reactivating a tenant needs two staff members: one requests with a reason, another decides. Approval takes effect at once. The tenant sees that it happened, not why."
      />
      {error ? (
        <div className="mb-4">
          <Notice tone="error">{error}</Notice>
        </div>
      ) : null}
      <Panel className="overflow-hidden">
        <Table aria-label="Tenant status requests">
          <thead>
            <tr>
              <Th>Tenant</Th>
              <Th>Request</Th>
              <Th>Reason</Th>
              <Th>Status</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={5} /> : null}
            {(q.data?.data ?? []).map((r) => {
              const mine = r.requested_by.id === me.staff.id;
              return (
                <Tr key={r.id}>
                  <Td className="text-body-sm">
                    <Link
                      href={`/admin/tenants/${r.tenant_id}`}
                      className="underline-offset-2 hover:underline"
                    >
                      {r.tenant_name}
                    </Link>
                  </Td>
                  <Td className="text-body-sm">
                    {humanize(r.action)}
                    <span className="block text-label text-text-muted">
                      {r.requested_by.name} · {formatTimestamp(r.created_at)}
                    </span>
                  </Td>
                  <Td
                    className="max-w-80 truncate text-body-sm text-text-secondary"
                    title={r.reason}
                  >
                    {r.reason}
                  </Td>
                  <Td>
                    <Status tone={tone(r)}>{humanize(r.status)}</Status>
                    {r.emergency ? (
                      <span className="block text-label text-risk-high">
                        Emergency ·{" "}
                        {r.reviewed_by
                          ? `reviewed by ${r.reviewed_by.name}`
                          : "awaiting review"}
                      </span>
                    ) : r.decided_by ? (
                      <span className="block text-label text-text-muted">
                        by {r.decided_by.name}
                      </span>
                    ) : null}
                  </Td>
                  <Td className="text-right">
                    <div className="flex flex-wrap justify-end gap-2">
                      {r.status === "pending" && !mine ? (
                        <>
                          <Button
                            size="sm"
                            variant="primary"
                            disabled={busy}
                            onClick={() => decide(r.id, "approve")}
                          >
                            Approve
                          </Button>
                          <Button
                            size="sm"
                            disabled={busy}
                            onClick={() => decide(r.id, "reject")}
                          >
                            Reject
                          </Button>
                        </>
                      ) : null}
                      {r.status === "pending" && mine ? (
                        <>
                          <Tag>Awaiting another staff member</Tag>
                          <Button
                            size="sm"
                            variant="danger"
                            disabled={busy}
                            onClick={() => decide(r.id, "withdraw")}
                          >
                            Withdraw
                          </Button>
                        </>
                      ) : null}
                      {r.emergency && !r.reviewed_by && !mine ? (
                        <Button
                          size="sm"
                          variant="primary"
                          disabled={busy}
                          onClick={() => decide(r.id, "review")}
                        >
                          Mark reviewed
                        </Button>
                      ) : null}
                      {r.emergency && !r.reviewed_by && mine ? (
                        <Tag>Awaiting review by another staff member</Tag>
                      ) : null}
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && q.data.data.length === 0 ? (
          <Empty title="No status requests yet" />
        ) : null}
      </Panel>
    </>
  );
}

/** On a tenant's page: ask for it to be suspended or reactivated, or see that a request waits. */
export function TenantStatusPanel({ tenant }: { tenant: Tenant }) {
  const me = useStaff();
  const { act, busy, error } = useStatusActions();
  const waiting = useQuery({
    queryKey: ["staff", "status-requests"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/tenant-status-requests")),
  });
  const open = (waiting.data?.data ?? []).find(
    (r) => r.tenant_id === tenant.id && r.status === "pending",
  );
  const action =
    tenant.status === "active"
      ? "suspend"
      : tenant.status === "suspended"
        ? "reactivate"
        : undefined;

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!action) return;
    const form = e.currentTarget;
    const reason = String(new FormData(form).get("reason"));
    const ok = await act(async () =>
      unwrap(
        await adminApi().POST(
          "/api/admin/v1/tenants/{tenant_id}/status-requests",
          {
            params: { path: { tenant_id: tenant.id } },
            body: { action, reason },
          },
        ),
      ),
    );
    if (ok) form.reset();
  };

  const onEmergency = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const form = e.currentTarget;
    const reason = String(new FormData(form).get("emergency_reason"));
    const ok = await act(async () =>
      unwrap(
        await adminApi().POST(
          "/api/admin/v1/tenants/{tenant_id}/emergency-suspension",
          {
            params: { path: { tenant_id: tenant.id } },
            body: { reason },
          },
        ),
      ),
    );
    if (ok) form.reset();
  };
  const canBreakGlass =
    me.staff.role === "security_admin" && tenant.status === "active" && !open;

  return (
    <>
      <Panel className="mb-5">
        <PanelHeader title="Suspension" />
        <div className="flex flex-col gap-4 p-4">
          {error ? <Notice tone="error">{error}</Notice> : null}
          {open ? (
            <Notice tone="info">
              A {VERBS[open.action].noun} requested by {open.requested_by.name}{" "}
              is waiting for another staff member.{" "}
              <Link
                href="/admin/status-requests"
                className="underline underline-offset-2"
              >
                Open requests
              </Link>
            </Notice>
          ) : action ? (
            <form
              onSubmit={(e) => void onSubmit(e)}
              className="flex flex-col gap-4"
            >
              <p className="text-body-sm text-text-secondary">
                {action === "suspend"
                  ? "A suspended tenant's API keys and console sessions are refused. Another staff member must approve."
                  : "Reactivating restores the tenant's access. Another staff member must approve."}
              </p>
              <Field
                label="Reason"
                hint="For the staff audit log and the approver. The tenant is not shown it. Name the ticket."
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
                variant={action === "suspend" ? "danger" : "primary"}
                className="self-start"
                disabled={busy}
              >
                {VERBS[action].button}
              </Button>
            </form>
          ) : (
            <p className="text-body-sm text-text-secondary">
              This tenant is {tenant.status}; it cannot be suspended or
              reactivated here.
            </p>
          )}
        </div>
      </Panel>
      {canBreakGlass ? (
        <Panel className="mb-5">
          <PanelHeader
            title="Emergency suspension"
            legend="Security administrators"
          />
          <form
            onSubmit={(e) => void onEmergency(e)}
            className="flex flex-col gap-4 p-4"
          >
            <p className="text-body-sm text-text-secondary">
              For an incident. The tenant is suspended now, without waiting for
              approval. It is flagged as an emergency in both audit logs and
              stays on every other staff member&apos;s queue until one of them
              reviews it. Reactivating it is an ordinary request that needs a
              second person.
            </p>
            <Field
              label="Reason"
              hint="Name the incident. The tenant is not shown it."
            >
              {(p) => (
                <Textarea
                  {...p}
                  name="emergency_reason"
                  minLength={10}
                  maxLength={500}
                  rows={2}
                  required
                />
              )}
            </Field>
            <label className="flex items-start gap-2 text-body-sm">
              <input type="checkbox" required className="mt-1" />
              <span>
                I understand this suspends the tenant immediately and that
                another staff member will review it.
              </span>
            </label>
            <Button
              type="submit"
              variant="danger"
              className="self-start"
              disabled={busy}
            >
              Suspend now
            </Button>
          </form>
        </Panel>
      ) : null}
    </>
  );
}
