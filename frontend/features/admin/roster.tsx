"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Status, Tag } from "@/components/ui/misc";
import { PageHeader, Panel } from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatCount, formatTimestamp, humanize } from "@/lib/format";

import { useStaff } from "./shell";

/** Staff accounts for security administrators: who has access, and switching it off or on. */
export function StaffRosterPage() {
  const me = useStaff();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["staff", "roster"],
    queryFn: async () => unwrap(await adminApi().GET("/api/admin/v1/staff")),
    enabled: me.staff.role === "security_admin",
  });
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const setAccess = async (id: string, active: boolean) => {
    setBusy(true);
    setError(undefined);
    try {
      const opts = { params: { path: { staff_id: id } } };
      const c = adminApi();
      unwrap(
        active
          ? await c.POST("/api/admin/v1/staff/{staff_id}/enable", opts)
          : await c.POST("/api/admin/v1/staff/{staff_id}/disable", opts),
      );
      await qc.invalidateQueries({ queryKey: ["staff", "roster"] });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The action failed.");
    } finally {
      setBusy(false);
    }
  };

  if (me.staff.role !== "security_admin") {
    return (
      <>
        <PageHeader title="Staff" />
        <Notice tone="warn">
          The staff roster is for security administrators.
        </Notice>
      </>
    );
  }
  return (
    <>
      <PageHeader
        title="Staff"
        description="Who can sign in to this console. Disabling an account ends its sessions at once. Accounts are created, and enrollment links issued, with the operator CLI (make staff); that is deliberate, so a hijacked session cannot mint credentials."
      />
      {error ? (
        <div className="mb-4">
          <Notice tone="error">{error}</Notice>
        </div>
      ) : null}
      <Panel className="overflow-hidden">
        <Table aria-label="Staff accounts">
          <thead>
            <tr>
              <Th>Person</Th>
              <Th>Role</Th>
              <Th>Status</Th>
              <Th className="text-right">Passkeys</Th>
              <Th>Last sign-in</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {(q.data?.data ?? []).map((s) => {
              const self = s.id === me.staff.id;
              return (
                <Tr key={s.id}>
                  <Td className="text-body-sm">
                    {s.name}
                    {self ? " (you)" : ""}
                    <span className="block text-label text-text-muted">
                      {s.email}
                    </span>
                  </Td>
                  <Td className="text-body-sm">{humanize(s.role)}</Td>
                  <Td>
                    <Status tone={s.status === "active" ? "ok" : "off"}>
                      {humanize(s.status)}
                    </Status>
                    {s.enrollment_pending ? (
                      <span className="block text-label text-text-muted">
                        Enrollment link outstanding
                      </span>
                    ) : null}
                  </Td>
                  <DataCell>{formatCount(s.passkeys)}</DataCell>
                  <Td className="text-body-sm text-text-secondary">
                    {s.last_sign_in ? formatTimestamp(s.last_sign_in) : "Never"}
                  </Td>
                  <Td className="text-right">
                    {self ? (
                      <Tag>Your account</Tag>
                    ) : s.status === "active" ? (
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={busy}
                        onClick={() => void setAccess(s.id, false)}
                      >
                        Disable
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        disabled={busy}
                        onClick={() => void setAccess(s.id, true)}
                      >
                        Enable
                      </Button>
                    )}
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && q.data.data.length === 0 ? (
          <Empty title="No staff accounts" />
        ) : null}
      </Panel>
    </>
  );
}
