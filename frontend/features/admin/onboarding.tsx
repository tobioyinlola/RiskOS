"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Textarea } from "@/components/ui/field";
import { KeyValues, pillTabClass } from "@/components/ui/misc";
import { Drawer } from "@/components/ui/overlay";
import { PageHeader, Panel } from "@/components/ui/panel";
import { StatusPill, type Tone } from "@/components/ui/pill";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { adminApi, type AdminSchemas } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatTimestamp, humanize } from "@/lib/format";

import { useStaff } from "./shell";

type Item = AdminSchemas["OnboardingQueueItem"];
type Review = AdminSchemas["OnboardingReview"];

const TONE: Record<Item["status"], Tone> = {
  draft: "neutral",
  submitted: "warning",
  approved: "success",
  rejected: "danger",
};
const LABEL: Record<Item["status"], string> = {
  draft: "In progress",
  submitted: "Awaiting review",
  approved: "Approved",
  rejected: "Changes requested",
};

const FILTERS = [
  { key: "submitted", label: "Awaiting review" },
  { key: "all", label: "All" },
] as const;

/** Self-serve workspaces' compliance onboarding, for operations admins. */
export function OnboardingReviewsPage() {
  const { staff } = useStaff();
  const [filter, setFilter] =
    useState<(typeof FILTERS)[number]["key"]>("submitted");
  const [open, setOpen] = useState<string>();
  const allowed = staff.role === "operations_admin";
  const q = useQuery({
    queryKey: ["staff", "onboarding"],
    enabled: allowed,
    queryFn: async () =>
      unwrap(await adminApi().GET("/api/admin/v1/onboarding")),
  });
  const rows = (q.data?.data ?? []).filter(
    (r) => filter === "all" || r.status === "submitted",
  );
  const waiting = (q.data?.data ?? []).filter(
    (r) => r.status === "submitted",
  ).length;

  return (
    <>
      <PageHeader
        title="Onboarding reviews"
        description="Compliance answers from self-serve workspaces. Approving opens a workspace's production environment. Every read and decision is in both audit logs."
      />
      {!allowed ? (
        <Notice tone="info">
          Onboarding reviews need the operations_admin role.
        </Notice>
      ) : (
        <>
          <div
            role="tablist"
            aria-label="Filter"
            className="mb-4 flex flex-wrap gap-2"
          >
            {FILTERS.map((f) => (
              <button
                key={f.key}
                type="button"
                role="tab"
                aria-selected={filter === f.key}
                onClick={() => setFilter(f.key)}
                className={pillTabClass(filter === f.key)}
              >
                {f.label}
                {f.key === "submitted" ? (
                  <span
                    className={
                      filter === f.key ? "text-white/70" : "text-text-muted"
                    }
                  >
                    {waiting}
                  </span>
                ) : null}
              </button>
            ))}
          </div>
          <Panel className="overflow-hidden">
            <Table aria-label="Onboarding reviews">
              <thead>
                <tr>
                  <Th>Workspace</Th>
                  <Th>Status</Th>
                  <Th>Signed up</Th>
                  <Th>Submitted</Th>
                  <Th>Reviewed</Th>
                </tr>
              </thead>
              <tbody>
                {q.isLoading ? <SkeletonRows columns={5} rows={3} /> : null}
                {rows.map((r) => (
                  <Tr
                    key={r.tenant_id}
                    onClick={() => setOpen(r.tenant_id)}
                    className="cursor-pointer"
                  >
                    <Td>
                      <span className="block font-medium">{r.tenant_name}</span>
                      <span className="mono text-label text-text-muted">
                        {r.tenant_id}
                      </span>
                    </Td>
                    <Td>
                      <StatusPill tone={TONE[r.status]}>
                        {LABEL[r.status]}
                      </StatusPill>
                    </Td>
                    <DataCell className="text-text-secondary">
                      {formatTimestamp(r.tenant_created_at)}
                    </DataCell>
                    <DataCell className="text-text-secondary">
                      {r.submitted_at ? formatTimestamp(r.submitted_at) : "—"}
                    </DataCell>
                    <DataCell className="text-text-secondary">
                      {r.reviewed_at ? formatTimestamp(r.reviewed_at) : "—"}
                    </DataCell>
                  </Tr>
                ))}
              </tbody>
            </Table>
            {q.isError ? <ErrorState error={q.error} className="m-5" /> : null}
            {q.isSuccess && rows.length === 0 ? (
              <Empty
                title={
                  filter === "submitted"
                    ? "Nothing awaiting review"
                    : "No self-serve workspaces yet"
                }
              >
                Submitted onboarding appears here, oldest first.
              </Empty>
            ) : null}
          </Panel>
        </>
      )}
      {open ? (
        <ReviewDrawer tenantId={open} onClose={() => setOpen(undefined)} />
      ) : null}
    </>
  );
}

function ReviewDrawer({
  tenantId,
  onClose,
}: {
  tenantId: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [note, setNote] = useState("");
  const [rejecting, setRejecting] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const q = useQuery({
    queryKey: ["staff", "onboarding", tenantId],
    queryFn: async () =>
      unwrap(
        await adminApi().GET("/api/admin/v1/onboarding/{tenant_id}", {
          params: { path: { tenant_id: tenantId } },
        }),
      ),
  });

  const decide = async (approve: boolean) => {
    setBusy(true);
    setError(undefined);
    try {
      const c = adminApi();
      const opts = {
        params: { path: { tenant_id: tenantId } },
        body: { note },
      };
      const r = approve
        ? await c.POST("/api/admin/v1/onboarding/{tenant_id}/approve", opts)
        : await c.POST("/api/admin/v1/onboarding/{tenant_id}/reject", opts);
      qc.setQueryData(["staff", "onboarding", tenantId], unwrap(r));
      await qc.invalidateQueries({
        queryKey: ["staff", "onboarding"],
        exact: true,
      });
      setRejecting(false);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? err.message
          : "The decision could not be saved. Try again.",
      );
    } finally {
      setBusy(false);
    }
  };

  const o = q.data;
  return (
    <Drawer
      open
      onOpenChange={(v) => (v ? undefined : onClose())}
      title={o?.tenant_name ?? "Onboarding"}
      subtitle={
        o ? (
          <StatusPill tone={TONE[o.status]}>{LABEL[o.status]}</StatusPill>
        ) : null
      }
      footer={
        o?.status === "submitted" ? (
          rejecting ? (
            <div className="flex w-full flex-col gap-3">
              <Field
                label="Note for the customer"
                hint="What to change, 10 to 1000 characters. The customer sees it."
              >
                {(p) => (
                  <Textarea
                    {...p}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    maxLength={1000}
                  />
                )}
              </Field>
              <div className="flex gap-2">
                <Button
                  variant="danger"
                  disabled={busy || note.trim().length < 10}
                  onClick={() => void decide(false)}
                >
                  Send back for changes
                </Button>
                <Button variant="ghost" onClick={() => setRejecting(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <>
              <Button
                variant="primary"
                arrow
                disabled={busy}
                onClick={() => void decide(true)}
              >
                Approve and open production
              </Button>
              <Button onClick={() => setRejecting(true)}>
                Request changes
              </Button>
            </>
          )
        ) : null
      }
    >
      {q.isLoading ? <Loading label="Loading the answers" /> : null}
      {q.isError ? <ErrorState error={q.error} /> : null}
      {error ? <Notice tone="error">{error}</Notice> : null}
      {o ? <Answers o={o} /> : null}
    </Drawer>
  );
}

function Answers({ o }: { o: Review }) {
  const a = o.answers as Record<string, unknown>;
  const show = (k: string) => {
    const v = a[k];
    if (v === undefined || v === "" || (Array.isArray(v) && v.length === 0))
      return "—";
    if (typeof v === "boolean") return v ? "Yes" : "No";
    if (Array.isArray(v)) return v.map((x) => humanize(String(x))).join(", ");
    return k.endsWith("email") ||
      k === "website" ||
      k === "country" ||
      k.endsWith("_number")
      ? String(v)
      : humanize(String(v));
  };
  const section = (title: string, keys: string[]) => (
    <section className="mb-6">
      <h3 className="mb-3 text-heading-3">{title}</h3>
      <KeyValues
        items={keys.map((k) => [humanize(k), show(k)] as [string, string])}
      />
    </section>
  );
  return (
    <>
      {o.missing.length ? (
        <div className="mb-5">
          <Notice tone="warn">
            Not answered yet: {o.missing.map(humanize).join(", ")}.
          </Notice>
        </div>
      ) : null}
      {o.review_note ? (
        <div className="mb-5">
          <Notice tone="info">
            Last note to the customer: {o.review_note}
          </Notice>
        </div>
      ) : null}
      {section("Business", [
        "legal_name",
        "trading_name",
        "country",
        "registration_number",
        "website",
        "industry",
      ])}
      {section("Regulation", [
        "regulated",
        "regulator",
        "licence_number",
        "aml_program",
        "compliance_officer_name",
        "compliance_officer_email",
        "screening_provider",
      ])}
      {section("Use case", [
        "monitored_events",
        "monthly_volume",
        "regions_served",
        "go_live",
      ])}
      {section("Data and privacy", [
        "data_protection",
        "data_residency",
        "dpo_email",
        "accept_terms",
        "accept_dpa",
      ])}
      <p className="text-label text-text-muted">
        Submitted {o.submitted_at ? formatTimestamp(o.submitted_at) : "—"}
        {o.reviewed_at ? ` · reviewed ${formatTimestamp(o.reviewed_at)}` : ""}
      </p>
    </>
  );
}
