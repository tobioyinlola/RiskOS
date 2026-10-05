"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Modal } from "@/components/ui/overlay";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import {
  bestUnit,
  formatParameterValue,
  fromInput,
  inputUnits,
  MODE_HELP,
  MODE_LABEL,
  toInput,
  type InputUnit,
  type Mode,
} from "@/lib/compliance";
import { formatCount, formatTimestamp } from "@/lib/format";
import { useComplianceHistory, useUserName } from "@/lib/queries";
import { useSession } from "@/lib/session";

type Rule = Schemas["ComplianceRule"];
type Parameter = Schemas["ComplianceParameter"];
type Change = Schemas["PackRuleChange"];
type Backtest = Schemas["Backtest"];

const INVALIDATE = ["compliance"];

/** The country whose pack a rule is configured in (an inherited rule is in several). */
const country = (rule: Rule) =>
  rule.jurisdiction ? { jurisdiction: rule.jurisdiction } : {};

/** "Floor: 100,000.00 NGN; Window: 1 day", for settings by name. */
export function describeSettings(
  rule: Rule,
  settings: Record<string, number>,
): string {
  const parts = Object.entries(settings).map(([name, v]) => {
    const p = rule.parameters.find((x) => x.name === name);
    return p ? `${p.label}: ${formatParameterValue(p, v)}` : `${name}: ${v}`;
  });
  return parts.length ? parts.join("; ") : "The library's defaults";
}

function SettingInput({
  p,
  text,
  unit,
  onText,
  onUnit,
}: {
  p: Parameter;
  text: string;
  unit: InputUnit;
  onText: (t: string) => void;
  onUnit: (u: InputUnit) => void;
}) {
  const units = inputUnits(p);
  const range = `${toInput(p.min, unit)} to ${toInput(p.max, unit)}${unit.label ? ` ${unit.label}` : ""}`;
  return (
    <Field
      label={p.label}
      hint={`Allowed: ${range}${p.default === null ? ". No default: your institution sets it." : ""}`}
    >
      {(f) => (
        <div className="flex gap-2">
          <Input
            {...f}
            inputMode="decimal"
            value={text}
            onChange={(e) => onText(e.target.value)}
            placeholder={p.default === null ? "Not set" : undefined}
            className="mono"
          />
          {units.length > 1 ? (
            <Select
              aria-label={`${p.label} unit`}
              className="w-40"
              value={unit.id}
              onChange={(e) =>
                onUnit(units.find((u) => u.id === e.target.value) ?? unit)
              }
            >
              {units.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.label}
                </option>
              ))}
            </Select>
          ) : unit.label ? (
            <span className="self-center text-body-sm text-text-secondary">
              {unit.label}
            </span>
          ) : null}
        </div>
      )}
    </Field>
  );
}

function BacktestResult({ result }: { result: Backtest }) {
  const rows: [string, Schemas["BacktestTally"]][] = [
    ["Current settings", result.current],
    ["These settings", result.candidate],
  ];
  return (
    <div className="flex flex-col gap-2">
      <p className="text-body-sm text-text-secondary">
        {formatCount(result.replayed)} events since{" "}
        {formatTimestamp(result.from)}
        {result.truncated ? " (the oldest 20,000)" : ""}.
      </p>
      <Table aria-label="Backtest">
        <thead>
          <tr>
            <Th />
            <Th>Fired</Th>
            <Th>Clear</Th>
            <Th>Not evaluated</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([label, t]) => (
            <Tr key={label}>
              <Td>{label}</Td>
              <Td className="mono">{formatCount(t.fired)}</Td>
              <Td className="mono">{formatCount(t.clear)}</Td>
              <Td className="mono">{formatCount(t.not_evaluated)}</Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {result.candidate.missing.length ? (
        <p className="text-body-sm text-text-secondary">
          Most often missing:{" "}
          {result.candidate.missing
            .slice(0, 3)
            .map((m) => `${m.item} (${formatCount(m.events)})`)
            .join(", ")}
        </p>
      ) : null}
    </div>
  );
}

/** Edit a rule's mode and settings in the current environment, with a backtest. */
export function ConfigureDialog({
  rule,
  open,
  onOpenChange,
  onDone,
}: {
  rule: Rule;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: (message: string) => void;
}) {
  const { environment } = useSession();
  const [mode, setMode] = useState<Mode>(rule.mode);
  const [reason, setReason] = useState("");
  const [units, setUnits] = useState<Record<string, InputUnit>>(() =>
    Object.fromEntries(
      rule.parameters.map((p) => [p.name, bestUnit(p, p.value)]),
    ),
  );
  const [texts, setTexts] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      rule.parameters.map((p) => [
        p.name,
        toInput(p.value, bestUnit(p, p.value)),
      ]),
    ),
  );
  const [backtest, setBacktest] = useState<Backtest>();

  // The settings that differ from the defaults, or the reason they cannot be sent.
  const settings = (): Record<string, number> | string => {
    const out: Record<string, number> = {};
    for (const p of rule.parameters) {
      const v = fromInput(p, texts[p.name] ?? "", units[p.name]!);
      if (v === null) {
        if ((texts[p.name] ?? "").trim() !== "")
          return `${p.label} is not a number.`;
        continue;
      }
      if (v < p.min || v > p.max) {
        return `${p.label} must be between ${formatParameterValue(p, p.min)} and ${formatParameterValue(p, p.max)}.`;
      }
      if (v !== p.default) out[p.name] = v;
    }
    return out;
  };
  const current = settings();
  const problem = typeof current === "string" ? current : undefined;

  const save = useAction(
    async () => {
      const res = await api().PUT("/api/v1/compliance/rules/{rule_id}/config", {
        params: { path: { rule_id: rule.id }, query: country(rule) },
        body: { mode, settings: current as Record<string, number>, reason },
      });
      const body = unwrap(res);
      return res.response.status === 202
        ? `Proposed. In ${environment}, another person with approval rights must approve this change before it applies.`
        : `Saved as version ${(body as Schemas["PackRuleConfig"]).version}.`;
    },
    {
      invalidate: INVALIDATE,
      onSuccess: (message) => {
        onDone(message);
        onOpenChange(false);
      },
    },
  );
  const test = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/rules/{rule_id}/backtest", {
          params: { path: { rule_id: rule.id } },
          body: { days: 7, settings: current as Record<string, number> },
        }),
      ),
    { onSuccess: setBacktest },
  );
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!problem && reason.trim()) void save.run();
  };
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Configure ${rule.id}`}
      description={`Applies to the ${environment} environment. Every change is a new version in the history.`}
      className="max-h-[90vh] max-w-xl overflow-y-auto"
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        {save.error ? <Notice tone="error">{save.error}</Notice> : null}
        {problem ? <Notice tone="warn">{problem}</Notice> : null}
        <Field label="Mode" hint={MODE_HELP[mode]}>
          {(f) => (
            <Select
              {...f}
              value={mode}
              onChange={(e) => setMode(e.target.value as Mode)}
            >
              {(Object.keys(MODE_LABEL) as Mode[]).map((m) => (
                <option key={m} value={m}>
                  {MODE_LABEL[m]}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {rule.parameters.map((p) => (
          <SettingInput
            key={p.name}
            p={p}
            text={texts[p.name] ?? ""}
            unit={units[p.name]!}
            onText={(t) => setTexts((cur) => ({ ...cur, [p.name]: t }))}
            onUnit={(u) => {
              const v = fromInput(p, texts[p.name] ?? "", units[p.name]!);
              setUnits((cur) => ({ ...cur, [p.name]: u }));
              setTexts((cur) => ({ ...cur, [p.name]: toInput(v, u) }));
            }}
          />
        ))}
        {rule.kind !== "governance" ? (
          <div className="flex flex-col gap-2 rounded-md border border-border-subtle p-3">
            <div className="flex items-center justify-between gap-2">
              <span className="text-label text-text-secondary">
                What would these settings have done in the last 7 days?
              </span>
              <Button
                type="button"
                size="sm"
                disabled={test.busy || !!problem}
                onClick={() => void test.run()}
              >
                Backtest
              </Button>
            </div>
            {test.error ? <Notice tone="error">{test.error}</Notice> : null}
            {backtest ? <BacktestResult result={backtest} /> : null}
          </div>
        ) : null}
        <Field
          label="Reason"
          hint="For the audit log and the person who approves it."
        >
          {(f) => (
            <Textarea
              {...f}
              value={reason}
              maxLength={500}
              required
              onChange={(e) => setReason(e.target.value)}
            />
          )}
        </Field>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={save.busy || !!problem || !reason.trim()}
          >
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/** A change awaiting a second person: approve, reject, or withdraw it. */
export function PendingChange({
  rule,
  change,
}: {
  rule: Rule;
  change: Change;
}) {
  const { can, session } = useSession();
  const userName = useUserName();
  const mine = change.proposed_by === session.user.id;
  const decide = useAction(
    async (verb: "approve" | "reject") => {
      const path =
        verb === "approve"
          ? "/api/v1/compliance/changes/{change_id}/approve"
          : "/api/v1/compliance/changes/{change_id}/reject";
      return unwrap(
        await api().POST(path, {
          params: { path: { change_id: change.id } },
          body: {},
        }),
      );
    },
    { invalidate: INVALIDATE },
  );
  return (
    <Notice tone="warn">
      <div className="flex flex-col gap-2">
        <p>
          <span className="font-medium">Awaiting approval. </span>
          {userName(change.proposed_by)} proposed: {MODE_LABEL[change.mode]};{" "}
          {describeSettings(rule, change.settings)}. Reason: {change.reason}
        </p>
        {decide.error ? (
          <p className="text-status-danger">{decide.error}</p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {can("rules:approve") && !mine ? (
            <Button
              size="sm"
              variant="primary"
              disabled={decide.busy}
              onClick={() => void decide.run("approve")}
            >
              Approve
            </Button>
          ) : null}
          {can("rules:approve") || mine ? (
            <Button
              size="sm"
              disabled={decide.busy}
              onClick={() => void decide.run("reject")}
            >
              {mine ? "Withdraw" : "Reject"}
            </Button>
          ) : null}
          {mine ? (
            <span className="self-center text-label text-text-muted">
              Another person must approve your change.
            </span>
          ) : null}
        </div>
      </div>
    </Notice>
  );
}

/** The rule's configuration versions in this environment, with rollback. */
export function ConfigHistory({
  rule,
  onDone,
}: {
  rule: Rule;
  onDone: (message: string) => void;
}) {
  const { can } = useSession();
  const q = useComplianceHistory(rule.id, rule.jurisdiction);
  const userName = useUserName();
  const [target, setTarget] = useState<number>();
  const [reason, setReason] = useState("");
  const rollback = useAction(
    async () => {
      const res = await api().POST(
        "/api/v1/compliance/rules/{rule_id}/rollback",
        {
          params: { path: { rule_id: rule.id }, query: country(rule) },
          body: { version: target!, reason },
        },
      );
      unwrap(res);
      return res.response.status === 202
        ? `Rollback to version ${target} proposed; it applies when another person approves it.`
        : `Version ${target} restored.`;
    },
    {
      invalidate: INVALIDATE,
      onSuccess: (m) => {
        onDone(m);
        setTarget(undefined);
        setReason("");
      },
    },
  );
  const versions = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader title="History" legend="in this environment" />
      <Table aria-label="Configuration history">
        <thead>
          <tr>
            <Th>Version</Th>
            <Th>Mode and settings</Th>
            <Th>Changed</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={4} rows={2} /> : null}
          {versions.map((v, i) => (
            <Tr key={v.version}>
              <Td className="mono">v{v.version}</Td>
              <Td className="py-2.5 text-body-sm">
                <StatusPill tone={v.mode === "live" ? "success" : "neutral"}>
                  {MODE_LABEL[v.mode]}
                </StatusPill>{" "}
                {describeSettings(rule, v.settings)}
                <span className="block text-text-muted">
                  {v.reason}
                  {v.restored_from ? ` (restores v${v.restored_from})` : ""}
                  {v.change_id ? " · approved by a second person" : ""}
                </span>
              </Td>
              <Td className="text-body-sm text-text-secondary">
                {formatTimestamp(v.created_at)}
                <span className="block text-label text-text-muted">
                  {userName(v.created_by)}
                </span>
              </Td>
              <Td className="text-right">
                {i > 0 && can("compliance:configure") ? (
                  <Button size="sm" onClick={() => setTarget(v.version)}>
                    Restore
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && versions.length === 0 ? (
        <Empty title="Running with the library's defaults">
          No one has changed this rule in this environment.
        </Empty>
      ) : null}
      <Modal
        open={target !== undefined}
        onOpenChange={(o) => (o ? undefined : setTarget(undefined))}
        title={`Restore version ${target ?? ""}?`}
        description="It becomes the next version. In production, restoring a regulatory control's settings needs a second person."
      >
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (reason.trim()) void rollback.run();
          }}
        >
          {rollback.error ? (
            <Notice tone="error">{rollback.error}</Notice>
          ) : null}
          <Field label="Reason">
            {(f) => (
              <Textarea
                {...f}
                value={reason}
                maxLength={500}
                required
                onChange={(e) => setReason(e.target.value)}
              />
            )}
          </Field>
          <div className="flex justify-end gap-2">
            <Button type="button" onClick={() => setTarget(undefined)}>
              Cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              disabled={rollback.busy || !reason.trim()}
            >
              Restore
            </Button>
          </div>
        </form>
      </Modal>
    </Panel>
  );
}
