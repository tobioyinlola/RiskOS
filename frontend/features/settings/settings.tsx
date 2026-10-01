"use client";

import { useState } from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input, Select } from "@/components/instrument/field";
import { KeyValues } from "@/components/instrument/misc";
import {
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/instrument/panel";
import { Ruler } from "@/components/instrument/risk-score";
import { ErrorState, Loading, Notice } from "@/components/instrument/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/instrument/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAmount, formatTimestamp, humanize } from "@/lib/format";
import { useCatalog, useRiskPolicy, useRiskSettings } from "@/lib/queries";
import { useSession } from "@/lib/session";
import { DEFAULT_THRESHOLDS, type Thresholds } from "@/lib/risk";

type Settings = Schemas["RiskSettings"];

const EVENT_TYPES = [
  "transaction",
  "login",
  "beneficiary_added",
  "device_change",
  "session_change",
  "profile_change",
];

export function RiskSettings() {
  const q = useRiskSettings();
  if (q.isError) return <ErrorState error={q.error} />;
  if (!q.data) return <Loading />;
  // Keyed by the stored settings, so a save or an environment switch starts a fresh draft.
  return <SettingsForm key={JSON.stringify(q.data)} initial={q.data} />;
}

function SettingsForm({ initial }: { initial: Settings }) {
  const catalog = useCatalog();
  const [s, setS] = useState<Settings>(initial);
  const [saved, setSaved] = useState(false);
  const save = useAction(
    // Automatic blocking has its own controls; leaving it out keeps the current mode.
    async (body: Settings) =>
      unwrap(
        await api().PUT("/api/v1/risk/settings", {
          body: { ...body, auto_block: undefined },
        }),
      ),
    {
      invalidate: ["risk-settings", "risk-policy", "risk-catalog"],
      onSuccess: () => setSaved(true),
    },
  );
  const t: Thresholds = s.thresholds ?? DEFAULT_THRESHOLDS;
  const thresholdsValid =
    1 <= t.low &&
    t.low < t.elevated &&
    t.elevated < t.high &&
    t.high < t.critical &&
    t.critical <= 100;
  const update = (patch: Partial<Settings>) => {
    setSaved(false);
    setS((x) => (x ? { ...x, ...patch } : x));
  };
  const currencies = Object.entries(s.high_value_by_currency);

  return (
    <>
      <PageHeader
        title="Risk settings"
        description="How this environment scores and decides. Changes apply within seconds and are audited."
        actions={
          <Button
            variant="primary"
            onClick={() => void save.run(s)}
            disabled={save.busy || !thresholdsValid}
          >
            Save settings
          </Button>
        }
      />
      <div className="mb-4 flex flex-col gap-2">
        {save.error ? <Notice tone="error">{save.error}</Notice> : null}
        {saved ? <Notice tone="ok">Settings saved.</Notice> : null}
      </div>
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Panel>
          <PanelHeader title="Bands" legend="calibration" />
          <PanelBody className="flex flex-col gap-4">
            <Ruler score={0} thresholds={t} animate={false} />
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {(["low", "elevated", "high", "critical"] as const).map((k) => (
                <Field key={k} label={`${humanize(k)} from`}>
                  {(p) => (
                    <Input
                      {...p}
                      type="number"
                      min={1}
                      max={100}
                      className="mono"
                      value={t[k]}
                      onChange={(e) =>
                        update({
                          thresholds: { ...t, [k]: Number(e.target.value) },
                        })
                      }
                    />
                  )}
                </Field>
              ))}
            </div>
            {!thresholdsValid ? (
              <Notice tone="warn">
                Thresholds must increase: 1 ≤ low &lt; elevated &lt; high &lt;
                critical ≤ 100.
              </Notice>
            ) : null}
            <Button
              size="sm"
              className="self-start"
              onClick={() => update({ thresholds: DEFAULT_THRESHOLDS })}
            >
              Reset to 20 / 50 / 70 / 90
            </Button>
          </PanelBody>
        </Panel>
        <AutoBlockPanel stored={initial} />
        <Panel>
          <PanelHeader title="Alerts" />
          <PanelBody className="flex flex-col gap-4">
            <Field
              label="Raise an alert for"
              hint="Alerts are raised off the decision path and grouped per user within an hour."
            >
              {(p) => (
                <Select
                  {...p}
                  value={s.alert_min_decision ?? "review"}
                  onChange={(e) =>
                    update({
                      alert_min_decision: e.target
                        .value as Settings["alert_min_decision"],
                    })
                  }
                >
                  <option value="review">Review decisions</option>
                  <option value="step_up">Step-up and review decisions</option>
                </Select>
              )}
            </Field>
            <p className="text-body-sm text-ink-3">
              Evaluations that ran with missing signals always raise a security
              alert.
            </p>
          </PanelBody>
        </Panel>
        <Panel className="overflow-hidden">
          <PanelHeader title="Signal weights" legend="0–50 points" />
          <Table>
            <thead>
              <tr>
                <Th>Signal</Th>
                <Th>Source</Th>
                <Th className="text-right">Default</Th>
                <Th className="w-28 text-right">Weight</Th>
              </tr>
            </thead>
            <tbody>
              {(catalog.data?.signals ?? []).map((sig) => (
                <Tr key={sig.code} density="compact">
                  <Td className="text-body-sm">
                    {sig.label}
                    <span className="mono block text-legend text-ink-3">
                      {sig.code}
                    </span>
                  </Td>
                  <Td className="legend">{sig.source}</Td>
                  <DataCell className="text-right text-ink-3">
                    {sig.default_weight}
                  </DataCell>
                  <Td className="text-right">
                    <Input
                      type="number"
                      min={0}
                      max={50}
                      aria-label={`Weight of ${sig.label}`}
                      className="mono h-control-sm w-20 text-right"
                      value={s.weights[sig.code] ?? sig.default_weight}
                      onChange={(e) => {
                        const v = Number(e.target.value);
                        const next = { ...s.weights };
                        if (v === sig.default_weight) delete next[sig.code];
                        else next[sig.code] = v;
                        update({ weights: next });
                      }}
                    />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        </Panel>
        <div className="flex flex-col gap-4">
          <Panel>
            <PanelHeader title="High value" legend="minor units" />
            <PanelBody className="flex flex-col gap-4">
              <p className="text-body-sm text-ink-2">
                While any signal is missing, events at or above this amount get
                at least Review.
              </p>
              <Field
                label="Default threshold"
                hint={`In minor units: ${s.high_value_default} is ${formatAmount(s.high_value_default, "NGN")}.`}
              >
                {(p) => (
                  <Input
                    {...p}
                    type="number"
                    min={1}
                    className="mono"
                    value={s.high_value_default}
                    onChange={(e) =>
                      update({ high_value_default: Number(e.target.value) })
                    }
                  />
                )}
              </Field>
              {currencies.map(([cur, v]) => (
                <div key={cur} className="flex items-end gap-2">
                  <Field label={`${cur} threshold`} className="flex-1">
                    {(p) => (
                      <Input
                        {...p}
                        type="number"
                        min={1}
                        className="mono"
                        value={v}
                        onChange={(e) =>
                          update({
                            high_value_by_currency: {
                              ...s.high_value_by_currency,
                              [cur]: Number(e.target.value),
                            },
                          })
                        }
                      />
                    )}
                  </Field>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      const next = { ...s.high_value_by_currency };
                      delete next[cur];
                      update({ high_value_by_currency: next });
                    }}
                  >
                    Remove
                  </Button>
                </div>
              ))}
              <form
                className="flex items-end gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  const f = new FormData(e.currentTarget);
                  const cur = String(f.get("currency") ?? "").toUpperCase();
                  if (/^[A-Z]{3}$/.test(cur)) {
                    update({
                      high_value_by_currency: {
                        ...s.high_value_by_currency,
                        [cur]: s.high_value_default,
                      },
                    });
                    e.currentTarget.reset();
                  }
                }}
              >
                <Field label="Add a currency" className="flex-1">
                  {(p) => (
                    <Input
                      {...p}
                      name="currency"
                      placeholder="USD"
                      maxLength={3}
                      className="mono uppercase"
                    />
                  )}
                </Field>
                <Button size="md" type="submit">
                  Add
                </Button>
              </form>
            </PanelBody>
          </Panel>
          <Panel className="overflow-hidden">
            <PanelHeader
              title="When signals are missing"
              legend="fail policy"
            />
            <Table>
              <tbody>
                {EVENT_TYPES.map((type) => (
                  <Tr key={type} density="compact">
                    <Td className="text-body-sm">{humanize(type)}</Td>
                    <Td className="w-56 text-right">
                      <Select
                        aria-label={`Fail policy for ${humanize(type)}`}
                        className="h-control-sm"
                        value={s.fail_policies[type] ?? "closed"}
                        onChange={(e) =>
                          update({
                            fail_policies: {
                              ...s.fail_policies,
                              [type]: e.target.value as "open" | "closed",
                            },
                          })
                        }
                      >
                        <option value="closed">Closed: at least Review</option>
                        <option value="open">Open: decide as scored</option>
                      </Select>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </Panel>
        </div>
      </div>
    </>
  );
}

const MODE_TEXT: Record<Schemas["AutoBlockMode"], string> = {
  off: "Off: Critical scores get Review.",
  shadow:
    "Shadow: Critical scores still get Review, and the engine records which ones it would block.",
  on: "On: Critical scores get Block.",
};

/**
 * Automatic blocking: off by default, shadow evidence first, on only after the shadow period,
 * and a kill switch that takes effect at once (docs/RISK_ENGINE.md).
 */
function AutoBlockPanel({ stored }: { stored: Settings }) {
  const policy = useRiskPolicy();
  const { environment } = useSession();
  const status = policy.data?.auto_block;
  const setMode = useAction(
    async (mode: Schemas["AutoBlockMode"]) =>
      unwrap(
        await api().PUT("/api/v1/risk/settings", {
          body: { ...stored, auto_block: { mode } },
        }),
      ),
    { invalidate: ["risk-settings", "risk-policy"] },
  );
  if (!status) return null;
  const eligible =
    status.eligible_at !== null && new Date(status.eligible_at) <= new Date();
  return (
    <Panel>
      <PanelHeader title="Automatic blocking" legend={status.mode} />
      <PanelBody className="flex flex-col gap-4">
        {status.mode === "on" ? (
          <Notice tone="warn">{MODE_TEXT.on}</Notice>
        ) : (
          <p className="text-body-sm text-ink-2">{MODE_TEXT[status.mode]}</p>
        )}
        {status.mode === "off" ? (
          <p className="text-body-sm text-ink-3">
            Blocking can be switched on after {status.shadow_days_required} days
            in shadow mode, with the evidence in front of you.
          </p>
        ) : (
          <KeyValues
            items={[
              [
                "Shadow since",
                status.shadow_since
                  ? formatTimestamp(status.shadow_since)
                  : "—",
              ],
              [
                "Would have blocked",
                <span key="n" className="mono">
                  {status.would_block_last_14_days} in the last 14 days
                </span>,
              ],
              [
                "Can switch on",
                status.mode === "on"
                  ? "On"
                  : status.eligible_at
                    ? eligible
                      ? "Now"
                      : `From ${formatTimestamp(status.eligible_at)}`
                    : "—",
              ],
            ]}
          />
        )}
        {setMode.error ? <Notice tone="error">{setMode.error}</Notice> : null}
        <div className="flex flex-wrap gap-2">
          {status.mode === "off" ? (
            <Button
              onClick={() => void setMode.run("shadow")}
              disabled={setMode.busy}
            >
              Start shadow mode
            </Button>
          ) : null}
          {status.mode === "shadow" ? (
            <>
              <Button
                variant="primary"
                onClick={() => void setMode.run("on")}
                disabled={setMode.busy || !eligible}
              >
                Switch blocking on
              </Button>
              <Button
                onClick={() => void setMode.run("off")}
                disabled={setMode.busy}
              >
                Turn off
              </Button>
            </>
          ) : null}
          {status.mode === "on" ? (
            <>
              <Button
                variant="primary"
                onClick={() => void setMode.run("shadow")}
                disabled={setMode.busy}
              >
                Stop blocking
              </Button>
              <Button
                onClick={() => void setMode.run("off")}
                disabled={setMode.busy}
              >
                Turn off
              </Button>
            </>
          ) : null}
        </div>
        {environment === "production" && status.mode !== "on" ? (
          <p className="text-body-sm text-ink-3">
            Decisions stay advisory: your own flow decides what a Block does.
          </p>
        ) : null}
      </PanelBody>
    </Panel>
  );
}
