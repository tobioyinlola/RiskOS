"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { ErrorState, Notice } from "@/components/ui/states";
import { adminApi } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatCount, formatTimestamp } from "@/lib/format";

const count = formatCount;

/**
 * A tenant's monthly event-volume quota: a soft limit. Nothing is refused for it; an overage is
 * announced once a month to staff and in the tenant's own audit log.
 */
export function TenantQuotaPanel({ tenantId }: { tenantId: string }) {
  const qc = useQueryClient();
  const key = ["staff", "tenant", tenantId, "quota"];
  const q = useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await adminApi().GET("/api/admin/v1/tenants/{tenant_id}/quota", {
          params: { path: { tenant_id: tenantId } },
        }),
      ),
  });
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError(undefined);
    try {
      await fn();
      await qc.invalidateQueries({ queryKey: key });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The action failed.");
    } finally {
      setBusy(false);
    }
  };

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    await run(async () =>
      unwrap(
        await adminApi().PUT("/api/admin/v1/tenants/{tenant_id}/quota", {
          params: { path: { tenant_id: tenantId } },
          body: {
            monthly_events: Number(f.get("monthly_events")),
            reason: String(f.get("quota_reason")),
          },
        }),
      ),
    );
  };

  const quota = q.data?.quota;
  const usage = quota?.usage;
  return (
    <Panel className="mb-5">
      <PanelHeader
        title="Monthly event quota"
        legend={
          usage?.exceeded ? "Over quota" : quota ? "Within quota" : "No quota"
        }
      />
      <div className="flex flex-col gap-4 p-4">
        <p className="text-body-sm text-text-secondary">
          A soft limit: events are never refused for it. The worker counts the
          month&apos;s events hourly and announces the first overage of a month
          to staff and in the tenant&apos;s audit log.
        </p>
        {q.isError ? <ErrorState error={q.error} /> : null}
        {error ? <Notice tone="error">{error}</Notice> : null}
        {quota && usage ? (
          <dl className="grid grid-cols-[minmax(120px,auto)_1fr] gap-x-4 gap-y-1 text-body-sm">
            <dt className="text-text-secondary">Quota</dt>
            <dd>{count(quota.monthly_events)} events a month</dd>
            <dt className="text-text-secondary">This month</dt>
            <dd>
              {usage.checked_at
                ? `${count(usage.events)} events, counted ${formatTimestamp(usage.checked_at)}`
                : "Not counted yet (within the hour)"}
            </dd>
            <dt className="text-text-secondary">Set</dt>
            <dd>
              {quota.updated_by.name} · {formatTimestamp(quota.updated_at)}
            </dd>
          </dl>
        ) : null}
        {q.data ? (
          <form
            key={quota?.updated_at ?? "none"}
            onSubmit={(e) => void onSubmit(e)}
            className="flex flex-col gap-4"
          >
            <Field label="Events a month">
              {(p) => (
                <Input
                  {...p}
                  name="monthly_events"
                  type="number"
                  min={1}
                  max={1000000000000}
                  defaultValue={quota?.monthly_events}
                  required
                />
              )}
            </Field>
            <Field
              label="Quota reason"
              hint="For the staff audit log. The tenant sees the quota, not the reason. Name the contract or ticket."
            >
              {(p) => (
                <Textarea
                  {...p}
                  name="quota_reason"
                  minLength={10}
                  maxLength={500}
                  rows={2}
                  required
                />
              )}
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary" disabled={busy}>
                Save quota
              </Button>
              {quota ? (
                <Button
                  disabled={busy}
                  onClick={() =>
                    void run(async () =>
                      unwrap(
                        await adminApi().DELETE(
                          "/api/admin/v1/tenants/{tenant_id}/quota",
                          { params: { path: { tenant_id: tenantId } } },
                        ),
                      ),
                    )
                  }
                >
                  Remove quota
                </Button>
              ) : null}
            </div>
          </form>
        ) : null}
      </div>
    </Panel>
  );
}
