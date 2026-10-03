"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Panel, PanelHeader } from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap } from "@/lib/api/client";
import { formatTimestamp } from "@/lib/format";
import { useFXRates, useUserName } from "@/lib/queries";
import { useSession } from "@/lib/session";

/** The institution's exchange rates, for rules on US dollar amounts (REG-10). */
export function FXRatesTab() {
  const { can } = useSession();
  const q = useFXRates();
  const userName = useUserName();
  const [currency, setCurrency] = useState("NGN");
  const [rate, setRate] = useState("");
  const writable = can("compliance:write");
  const save = useAction(
    async () =>
      unwrap(
        await api().PUT("/api/v1/compliance/fx-rates/{currency}", {
          params: { path: { currency: currency.trim().toUpperCase() } },
          body: { usd_per_unit: Number(rate) },
        }),
      ),
    { invalidate: ["compliance"], onSuccess: () => setRate("") },
  );
  const remove = useAction(
    async (code: string) =>
      unwrap(
        await api().DELETE("/api/v1/compliance/fx-rates/{currency}", {
          params: { path: { currency: code } },
        }),
      ),
    { invalidate: ["compliance"] },
  );
  const valid = /^[A-Za-z]{3}$/.test(currency.trim()) && Number(rate) > 0;
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (valid) void save.run();
  };
  const rates = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Exchange rates"
        legend="US dollars per one unit, set by your institution"
      />
      <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
        Rules on US dollar amounts, such as the foreign transaction report
        (REG-10), convert an event&apos;s amount with these rates when the event
        does not send the dollar amount itself. Without a rate they are not
        evaluated.
      </p>
      {save.error || remove.error ? (
        <div className="px-5 pb-3">
          <Notice tone="error">{save.error ?? remove.error}</Notice>
        </div>
      ) : null}
      {writable ? (
        <form
          onSubmit={onSubmit}
          className="flex flex-wrap items-end gap-3 px-5 pb-4"
        >
          <Field label="Currency" className="w-32">
            {(f) => (
              <Input
                {...f}
                value={currency}
                maxLength={3}
                className="mono uppercase"
                onChange={(e) => setCurrency(e.target.value)}
              />
            )}
          </Field>
          <Field label="US dollars per unit" className="w-56">
            {(f) => (
              <Input
                {...f}
                inputMode="decimal"
                value={rate}
                placeholder="0.00065"
                className="mono"
                onChange={(e) => setRate(e.target.value)}
              />
            )}
          </Field>
          <Button
            type="submit"
            variant="primary"
            disabled={!valid || save.busy}
          >
            Save rate
          </Button>
        </form>
      ) : null}
      <Table aria-label="Exchange rates">
        <thead>
          <tr>
            <Th>Currency</Th>
            <Th>US dollars per unit</Th>
            <Th>Updated</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={4} rows={2} /> : null}
          {rates.map((r) => (
            <Tr key={r.currency}>
              <Td className="mono">{r.currency}</Td>
              <Td className="mono">{r.usd_per_unit}</Td>
              <Td className="text-body-sm text-text-secondary">
                {formatTimestamp(r.updated_at)}
                <span className="block text-label text-text-muted">
                  {userName(r.updated_by)}
                </span>
              </Td>
              <Td className="text-right">
                {writable ? (
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={remove.busy}
                    onClick={() => void remove.run(r.currency)}
                  >
                    Remove
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && rates.length === 0 ? (
        <Empty title="No exchange rates">
          Amounts in US dollars are taken from the events themselves until you
          set a rate.
        </Empty>
      ) : null}
    </Panel>
  );
}
