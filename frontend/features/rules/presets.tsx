"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Tag } from "@/components/ui/misc";
import { Modal } from "@/components/ui/overlay";
import { Metric, Panel } from "@/components/ui/panel";
import { StatusPill, type Tone } from "@/components/ui/pill";
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
  matchesFilter,
  NO_FILTER,
  OFF_REASON,
  PROVENANCE,
  type RuleFilter,
} from "@/lib/compliance";
import { formatCount } from "@/lib/format";
import {
  useCompliancePack,
  useComplianceRules,
  useJurisdictions,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

type Rule = Schemas["ComplianceRule"];
type Mode = Rule["mode"];

/** What a preset is, for the person choosing which to activate. */
const KIND: Record<Rule["kind"], { label: string; tone: Tone }> = {
  control: { label: "Required by law", tone: "danger" },
  signal: { label: "Risk signal", tone: "info" },
  governance: { label: "Governance", tone: "neutral" },
};

const STATE: Record<Mode, { label: string; tone: Tone }> = {
  live: { label: "Active", tone: "success" },
  shadow: { label: "Monitoring", tone: "warning" },
  off: { label: "Off", tone: "neutral" },
};

type Show = "" | "active" | "off";
type Origin = "" | NonNullable<Rule["provenance"]> | "inherited";

/** Turn a preset on or off, keeping its settings. Legal controls need a reason to go off. */
function useSetMode() {
  return useAction(
    async ({
      rule,
      mode,
      reason,
    }: {
      rule: Rule;
      mode: Mode;
      reason: string;
    }) => {
      const path = { params: { path: { rule_id: rule.id } } };
      const current = unwrap(
        await api().GET("/api/v1/compliance/rules/{rule_id}/config", path),
      );
      const res = await api().PUT("/api/v1/compliance/rules/{rule_id}/config", {
        ...path,
        body: { mode, settings: current.settings, reason },
      });
      unwrap(res);
      return res.response.status === 202 ? "proposed" : "applied";
    },
    { invalidate: ["compliance"] },
  );
}

/** Where a preset comes from: its provenance, or the Nigerian rule it inherits. */
function Origin({ rule }: { rule: Rule }) {
  return (
    <span className="flex flex-col items-start gap-1">
      {rule.provenance ? (
        <StatusPill tone={PROVENANCE[rule.provenance].tone}>
          {PROVENANCE[rule.provenance].label}
        </StatusPill>
      ) : null}
      {rule.inherited_from ? <Tag>From Nigeria&apos;s library</Tag> : null}
      {rule.template ? <Tag>Based on {rule.template}</Tag> : null}
      {rule.off_reason ? <Tag>{OFF_REASON[rule.off_reason]}</Tag> : null}
    </span>
  );
}

/**
 * Rules → Preset rules: one country's pack. In the default country each rule is activated or
 * turned off here; other countries are read here, and switched on with a readiness check.
 */
export function PresetRules({
  country,
  fallback,
}: {
  country: string;
  fallback: string | undefined;
}) {
  const { can, environment } = useSession();
  const jurisdictions = useJurisdictions();
  const here = jurisdictions.data?.data.find((j) => j.code === country);
  const browsing = !!here && !here.activated;
  const pack = useCompliancePack(country || undefined);
  const q = useComplianceRules(country || undefined);
  const setMode = useSetMode();
  const [f, setF] = useState<RuleFilter>(NO_FILTER);
  const [show, setShow] = useState<Show>("");
  const [origin, setOrigin] = useState<Origin>("");
  const [turningOff, setTurningOff] = useState<Rule>();
  const [message, setMessage] = useState<string>();
  const all = useMemo(() => q.data?.data ?? [], [q.data]);
  const mine = all.filter((r) => r.applicable !== false);
  const rules = all.filter(
    (r) =>
      matchesFilter(r, f) &&
      (!show || (show === "active" ? r.mode !== "off" : r.mode === "off")) &&
      (!origin ||
        (origin === "inherited"
          ? !!r.inherited_from
          : r.provenance === origin)),
  );
  const tagged = all.some((r) => r.provenance);
  const set = <K extends keyof RuleFilter>(k: K, v: RuleFilter[K]) =>
    setF((cur) => ({ ...cur, [k]: v }));
  const groups = pack.data?.groups ?? [];
  // Rules of the default country are configured here; another country's, once its
  // activation has its own settings (activation and governance).
  const configurable =
    can("compliance:configure") && !browsing && country === fallback;
  const count = (k: Rule["kind"]) => {
    const of = mine.filter((r) => r.kind === k);
    return `${formatCount(of.filter((r) => r.mode === "live").length)} of ${formatCount(of.length)}`;
  };
  const run = async (rule: Rule, mode: Mode, reason: string) => {
    setMessage(undefined);
    const out = await setMode.run({ rule, mode, reason });
    if (out === "proposed")
      setMessage(
        `${rule.name}: sent for approval. It changes when another person with approval rights approves it.`,
      );
    return out;
  };

  return (
    <>
      {browsing ? (
        <div className="mb-5">
          <Notice tone="info">
            {pack.data?.name ?? country} is not switched on for your
            institution, so these are its rules as the library ships them.
            Switching a country on asks which licensed entity operates there,
            checks what data its rules need, and needs a second person&apos;s
            approval to go live.
          </Notice>
        </div>
      ) : country !== fallback ? (
        <div className="mb-5">
          <Notice tone="info">
            {pack.data?.name ?? country} is switched on for your institution.
            Its rules run on events booked by its entity; changing their
            settings here comes with country activation.
          </Notice>
        </div>
      ) : (
        <div className="mb-5 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Metric label="Legal controls active" value={count("control")} />
          <Metric label="Risk signals active" value={count("signal")} />
          <Metric
            label="Governance presets active"
            value={count("governance")}
          />
        </div>
      )}
      {message ? (
        <div className="mb-4">
          <Notice tone="info">{message}</Notice>
        </div>
      ) : null}
      {setMode.error ? (
        <div className="mb-4">
          <Notice tone="error">{setMode.error}</Notice>
        </div>
      ) : null}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Input
          type="search"
          aria-label="Search preset rules"
          placeholder="Search rules"
          className="h-control-sm w-60 text-body-sm"
          value={f.query}
          onChange={(e) => set("query", e.target.value)}
        />
        <Select
          aria-label="Type"
          className="h-control-sm w-44 text-body-sm"
          value={f.kind}
          onChange={(e) => set("kind", e.target.value as RuleFilter["kind"])}
        >
          <option value="">All types</option>
          {(Object.keys(KIND) as Rule["kind"][]).map((k) => (
            <option key={k} value={k}>
              {KIND[k].label}
            </option>
          ))}
        </Select>
        <Select
          aria-label="Group"
          className="h-control-sm w-52 text-body-sm"
          value={f.group}
          onChange={(e) => set("group", e.target.value)}
        >
          <option value="">All groups</option>
          {groups.map((g) => (
            <option key={g.id} value={g.id}>
              {g.label}
            </option>
          ))}
        </Select>
        <Select
          aria-label="Status"
          className="h-control-sm w-40 text-body-sm"
          value={show}
          onChange={(e) => setShow(e.target.value as Show)}
        >
          <option value="">Any status</option>
          <option value="active">On</option>
          <option value="off">Off</option>
        </Select>
        {tagged ? (
          <Select
            aria-label="Where it comes from"
            className="h-control-sm w-52 text-body-sm"
            value={origin}
            onChange={(e) => setOrigin(e.target.value as Origin)}
          >
            <option value="">Any origin</option>
            {(Object.keys(PROVENANCE) as (keyof typeof PROVENANCE)[]).map(
              (p) => (
                <option key={p} value={p}>
                  {PROVENANCE[p].label}
                </option>
              ),
            )}
            <option value="inherited">From Nigeria&apos;s library</option>
          </Select>
        ) : null}
        <label className="inline-flex items-center gap-2 text-body-sm text-text-secondary">
          <input
            type="checkbox"
            className="size-4 accent-ink"
            checked={f.applicableOnly}
            onChange={(e) => set("applicableOnly", e.target.checked)}
          />
          Only rules for our institution type
        </label>
        <span
          className="ml-auto text-label text-text-muted"
          role="status"
          aria-live="polite"
        >
          {q.data
            ? `${formatCount(rules.length)} of ${formatCount(all.length)} rules`
            : ""}
        </span>
      </div>
      {tagged ? (
        <dl
          aria-label="Where rules come from"
          className="mb-3 grid gap-x-6 gap-y-1 text-body-sm text-text-secondary md:grid-cols-3"
        >
          {(Object.keys(PROVENANCE) as (keyof typeof PROVENANCE)[]).map((p) => (
            <div key={p}>
              <dt className="inline font-medium text-text-primary">
                {PROVENANCE[p].label}:{" "}
              </dt>
              <dd className="inline">{PROVENANCE[p].meaning}</dd>
            </div>
          ))}
        </dl>
      ) : null}
      <Panel className="overflow-hidden">
        <Table aria-label="Preset rules">
          <thead>
            <tr>
              <Th>Rule</Th>
              {tagged ? <Th>Where it comes from</Th> : null}
              <Th>Type</Th>
              <Th>Status</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={tagged ? 5 : 4} /> : null}
            {rules.map((r) => {
              const fits = r.applicable !== false;
              return (
                <Tr key={r.id}>
                  <Td className="max-w-2xl py-3">
                    <Link
                      href={
                        `/compliance/rules/${r.id}?jurisdiction=${country}` as "/compliance"
                      }
                      className="font-medium text-text-primary hover:text-accent"
                    >
                      {r.name}
                    </Link>
                    <p className="line-clamp-2 text-body-sm text-text-secondary">
                      {r.description}
                    </p>
                    <p className="text-label text-text-muted">
                      {groups.find((g) => g.id === r.group)?.label ?? r.group} ·{" "}
                      {r.regulators.join(", ")} · {r.id}
                    </p>
                  </Td>
                  {tagged ? (
                    <Td>
                      <Origin rule={r} />
                    </Td>
                  ) : null}
                  <Td>
                    <StatusPill tone={KIND[r.kind].tone}>
                      {KIND[r.kind].label}
                    </StatusPill>
                  </Td>
                  <Td className="whitespace-nowrap">
                    {browsing ? (
                      <span className="text-body-sm text-text-muted">
                        {r.defaults.enabled
                          ? "On by default"
                          : "Off by default"}
                      </span>
                    ) : fits ? (
                      <StatusPill tone={STATE[r.mode].tone}>
                        {STATE[r.mode].label}
                      </StatusPill>
                    ) : (
                      <Tag>Not for your institution type</Tag>
                    )}
                  </Td>
                  <Td className="text-right whitespace-nowrap">
                    {configurable && fits ? (
                      r.mode === "live" ? (
                        <Button
                          size="sm"
                          aria-label={`Turn off ${r.name}`}
                          disabled={setMode.busy}
                          onClick={() =>
                            r.kind === "control"
                              ? setTurningOff(r)
                              : void run(
                                  r,
                                  "off",
                                  "Turned off on the rules page",
                                )
                          }
                        >
                          Turn off
                        </Button>
                      ) : (
                        <Button
                          size="sm"
                          variant="primary"
                          aria-label={`Activate ${r.name}`}
                          disabled={setMode.busy}
                          onClick={() =>
                            void run(r, "live", "Activated on the rules page")
                          }
                        >
                          Activate
                        </Button>
                      )
                    ) : null}
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rules.length === 0 ? (
          <Empty title="No rules match" />
        ) : null}
      </Panel>
      {turningOff ? (
        <TurnOffControl
          rule={turningOff}
          production={environment === "production"}
          busy={setMode.busy}
          onCancel={() => setTurningOff(undefined)}
          onConfirm={async (reason) => {
            await run(turningOff, "off", reason);
            setTurningOff(undefined);
          }}
        />
      ) : null}
    </>
  );
}

/** Turning off a control the law requires: a reason, and in production a second person. */
function TurnOffControl({
  rule,
  production,
  busy,
  onCancel,
  onConfirm,
}: {
  rule: Rule;
  production: boolean;
  busy: boolean;
  onCancel: () => void;
  onConfirm: (reason: string) => Promise<void>;
}) {
  const [reason, setReason] = useState("");
  return (
    <Modal
      open
      onOpenChange={(o) => (o ? undefined : onCancel())}
      title={`Turn off ${rule.name}?`}
      description={`This control is required by ${rule.regulators.join(", ")}. While it is off, RiskOS does not enforce it.${
        production ? " In production a second person must approve this." : ""
      }`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (reason.trim()) void onConfirm(reason.trim());
        }}
      >
        <Field
          label="Why"
          hint="Kept in the audit log and shown to the approver."
        >
          {(p) => (
            <Textarea
              {...p}
              value={reason}
              maxLength={500}
              onChange={(e) => setReason(e.target.value)}
              required
            />
          )}
        </Field>
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={onCancel}>
            Keep it on
          </Button>
          <Button
            type="submit"
            variant="danger"
            disabled={busy || !reason.trim()}
          >
            {production ? "Send for approval" : "Turn off"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
