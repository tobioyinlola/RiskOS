"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { ErrorState, Notice } from "@/components/ui/states";
import { adminApi, type AdminSchemas } from "@/lib/api/admin-client";
import { ApiError, unwrap } from "@/lib/api/client";
import { formatTimestamp } from "@/lib/format";

type RateLimit = AdminSchemas["RateLimit"];

const show = (l: RateLimit) => `${l.per_second} per second, burst ${l.burst}`;

/** A tenant's request limits: the platform's, the override staff set, and the form to change it. */
export function TenantLimitsPanel({ tenantId }: { tenantId: string }) {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ["staff", "tenant", tenantId, "limits"],
    queryFn: async () =>
      unwrap(
        await adminApi().GET("/api/admin/v1/tenants/{tenant_id}/limits", {
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
      await qc.invalidateQueries({
        queryKey: ["staff", "tenant", tenantId, "limits"],
      });
      await qc.invalidateQueries({ queryKey: ["staff", "tenant", tenantId] });
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "The action failed.");
      return false;
    } finally {
      setBusy(false);
    }
  };

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const num = (k: string) => Number(f.get(k));
    await run(async () =>
      unwrap(
        await adminApi().PUT("/api/admin/v1/tenants/{tenant_id}/limits", {
          params: { path: { tenant_id: tenantId } },
          body: {
            per_key: { per_second: num("key_rate"), burst: num("key_burst") },
            per_tenant: {
              per_second: num("tenant_rate"),
              burst: num("tenant_burst"),
            },
            reason: String(f.get("limits_reason")),
          },
        }),
      ),
    );
  };

  const d = q.data;
  const o = d?.override;
  const start = o ?? d?.defaults;
  return (
    <Panel className="mb-5">
      <PanelHeader
        title="Request limits"
        legend={o ? "Override in force" : "Platform limits"}
      />
      <div className="flex flex-col gap-4 p-4">
        {q.isError ? <ErrorState error={q.error} /> : null}
        {error ? <Notice tone="error">{error}</Notice> : null}
        {d ? (
          <dl className="grid grid-cols-[minmax(120px,auto)_1fr] gap-x-4 gap-y-1 text-body-sm">
            <dt className="text-text-secondary">Platform, per key</dt>
            <dd>{show(d.defaults.per_key)}</dd>
            <dt className="text-text-secondary">Platform, per tenant</dt>
            <dd>{show(d.defaults.per_tenant)}</dd>
            {o ? (
              <>
                <dt className="text-text-secondary">Override, per key</dt>
                <dd>{show(o.per_key)}</dd>
                <dt className="text-text-secondary">Override, per tenant</dt>
                <dd>{show(o.per_tenant)}</dd>
                <dt className="text-text-secondary">Set</dt>
                <dd>
                  {o.updated_by.name} · {formatTimestamp(o.updated_at)}
                </dd>
              </>
            ) : null}
          </dl>
        ) : null}
        {start ? (
          <form
            key={o?.updated_at ?? "defaults"}
            onSubmit={(e) => void onSubmit(e)}
            className="flex flex-col gap-4"
          >
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Field label="Per key: per second">
                {(p) => (
                  <Input
                    {...p}
                    name="key_rate"
                    type="number"
                    step="any"
                    min={0.001}
                    max={100000}
                    defaultValue={start.per_key.per_second}
                    required
                  />
                )}
              </Field>
              <Field label="Per key: burst">
                {(p) => (
                  <Input
                    {...p}
                    name="key_burst"
                    type="number"
                    min={1}
                    max={1000000}
                    defaultValue={start.per_key.burst}
                    required
                  />
                )}
              </Field>
              <Field label="Per tenant: per second">
                {(p) => (
                  <Input
                    {...p}
                    name="tenant_rate"
                    type="number"
                    step="any"
                    min={0.001}
                    max={100000}
                    defaultValue={start.per_tenant.per_second}
                    required
                  />
                )}
              </Field>
              <Field label="Per tenant: burst">
                {(p) => (
                  <Input
                    {...p}
                    name="tenant_burst"
                    type="number"
                    min={1}
                    max={1000000}
                    defaultValue={start.per_tenant.burst}
                    required
                  />
                )}
              </Field>
            </div>
            <Field
              label="Limits reason"
              hint="For the staff audit log. The tenant sees the new values, not the reason. Name the ticket."
            >
              {(p) => (
                <Textarea
                  {...p}
                  name="limits_reason"
                  minLength={10}
                  maxLength={500}
                  rows={2}
                  required
                />
              )}
            </Field>
            <div className="flex flex-wrap gap-2">
              <Button type="submit" variant="primary" disabled={busy}>
                Save limits
              </Button>
              {o ? (
                <Button
                  disabled={busy}
                  onClick={() =>
                    void run(async () =>
                      unwrap(
                        await adminApi().DELETE(
                          "/api/admin/v1/tenants/{tenant_id}/limits",
                          { params: { path: { tenant_id: tenantId } } },
                        ),
                      ),
                    )
                  }
                >
                  Remove override
                </Button>
              ) : null}
            </div>
          </form>
        ) : null}
      </div>
    </Panel>
  );
}
