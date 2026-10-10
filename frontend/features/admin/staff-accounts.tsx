"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Status, Tag } from "@/components/ui/misc";
import { Panel, PanelHeader } from "@/components/ui/panel";
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

type AccountRequest = AdminSchemas["StaffAccountRequest"];

function tone(r: AccountRequest): "ok" | "warn" | "off" {
  if (r.status === "pending") return "warn";
  if (r.status === "approved") return "ok";
  return "off";
}

/**
 * Adding a person: one security administrator asks, a different one approves, and the enrollment
 * link is emailed to the new person. No session is shown it.
 */
export function StaffAccountRequests() {
  const me = useStaff();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["staff", "account-requests"],
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/staff/account-requests")),
    refetchInterval: 30_000,
  });
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["staff", "account-requests"] }),
        qc.invalidateQueries({ queryKey: ["staff", "roster"] }),
      ]);
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
        await adminApi().POST("/api/admin/v1/staff/account-requests", {
          body: {
            email: String(f.get("account_email")),
            name: String(f.get("account_name")),
            role: String(f.get("account_role")) as
              "operations_admin" | "security_admin",
            reason: String(f.get("account_reason")),
          },
        }),
      ),
    );
    if (ok) form.reset();
  };

  const decide = (id: string, verb: "approve" | "reject" | "withdraw") =>
    void act(async () => {
      const opts = { params: { path: { request_id: id } } };
      const c = adminApi();
      if (verb === "approve")
        return unwrap(
          await c.POST(
            "/api/admin/v1/staff/account-requests/{request_id}/approve",
            opts,
          ),
        );
      if (verb === "reject")
        return unwrap(
          await c.POST(
            "/api/admin/v1/staff/account-requests/{request_id}/reject",
            opts,
          ),
        );
      return unwrap(
        await c.POST(
          "/api/admin/v1/staff/account-requests/{request_id}/withdraw",
          opts,
        ),
      );
    });

  return (
    <>
      <Panel className="my-5">
        <PanelHeader
          title="Add a person"
          legend="Needs a second security administrator"
        />
        <form
          onSubmit={(e) => void onRequest(e)}
          className="flex flex-col gap-4 p-4"
        >
          <p className="text-body-sm text-text-secondary">
            Another security administrator approves the request. On approval the
            account is created and its enrollment link is emailed to the address
            below; neither of you is shown the link. If email is not configured
            for this deployment, accounts are created with the operator CLI.
          </p>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Email">
              {(p) => (
                <Input {...p} name="account_email" type="email" required />
              )}
            </Field>
            <Field label="Name">
              {(p) => (
                <Input {...p} name="account_name" maxLength={100} required />
              )}
            </Field>
            <Field label="Role">
              {(p) => (
                <Select
                  {...p}
                  name="account_role"
                  defaultValue="operations_admin"
                >
                  <option value="operations_admin">
                    Operations administrator
                  </option>
                  <option value="security_admin">Security administrator</option>
                </Select>
              )}
            </Field>
          </div>
          <Field
            label="Reason for access"
            hint="For the staff audit log and the approver. Name the ticket."
          >
            {(p) => (
              <Textarea
                {...p}
                name="account_reason"
                minLength={10}
                maxLength={500}
                rows={2}
                required
              />
            )}
          </Field>
          {error ? <Notice tone="error">{error}</Notice> : null}
          <Button
            type="submit"
            variant="primary"
            className="self-start"
            disabled={busy}
          >
            Request account
          </Button>
        </form>
      </Panel>
      <Panel className="overflow-hidden">
        <Table aria-label="Account requests">
          <thead>
            <tr>
              <Th>Person</Th>
              <Th>Role</Th>
              <Th>Requested by</Th>
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
                    {r.name}
                    <span className="block text-label text-text-muted">
                      {r.email}
                    </span>
                  </Td>
                  <Td className="text-body-sm">{humanize(r.role)}</Td>
                  <Td className="text-body-sm">
                    {r.requested_by.name}
                    <span className="block text-label text-text-muted">
                      {formatTimestamp(r.created_at)}
                    </span>
                  </Td>
                  <Td>
                    <Status tone={tone(r)}>{humanize(r.status)}</Status>
                    {r.status === "approved" ? (
                      <span className="block text-label text-text-muted">
                        Link emailed
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
                          <Tag>Awaiting another security administrator</Tag>
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
                    </div>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && q.data.data.length === 0 ? (
          <Empty title="No account requests yet" />
        ) : null}
      </Panel>
    </>
  );
}
