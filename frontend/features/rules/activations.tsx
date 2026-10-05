"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { KeyValues } from "@/components/ui/misc";
import { Modal } from "@/components/ui/overlay";
import { Panel, PanelHeader } from "@/components/ui/panel";
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
import { flag } from "@/lib/compliance";
import { formatTimestamp } from "@/lib/format";
import {
  useActivationChanges,
  useActivationReadiness,
  useActivations,
  useActiveRules,
  useCompliancePack,
  useJurisdictions,
  useUserName,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

type Activation = Schemas["JurisdictionActivation"];
type Change = Schemas["ActivationChange"];
type Status = Activation["status"];

const INVALIDATE = ["compliance"];

export const STATUS: Record<
  Status,
  { label: string; tone: Tone; help: string }
> = {
  draft: {
    label: "Draft",
    tone: "neutral",
    help: "Judges no event yet.",
  },
  shadow: {
    label: "Monitoring",
    tone: "warning",
    help: "Every rule runs in shadow: results are recorded, nothing is blocked.",
  },
  live: {
    label: "Live",
    tone: "success",
    help: "Rules run as configured.",
  },
  suspended: {
    label: "Suspended",
    tone: "neutral",
    help: "Judges no event until resumed.",
  },
};

/** What a change does, in words. */
export function changeText(c: Change): string {
  switch (c.action) {
    case "create":
      return "Created as a draft";
    case "fallback":
      return "Make it the default country for events naming no booking entity";
    case "update":
      return "Change licence type, capabilities or day start";
    default:
      return `${STATUS[c.status_from ?? "draft"].label} → ${STATUS[c.status_to ?? "draft"].label}`;
  }
}

const STATE: Record<Change["state"], { label: string; tone: Tone }> = {
  applied: { label: "Applied", tone: "success" },
  pending: { label: "Awaiting approval", tone: "warning" },
  approved: { label: "Approved", tone: "success" },
  rejected: { label: "Rejected", tone: "danger" },
};

/** A country's activations: status, readiness, the moves allowed, changes awaiting approval. */
export function CountryActivations({ country }: { country: string }) {
  const { can } = useSession();
  const acts = useActivations();
  const jurisdictions = useJurisdictions();
  const [wizard, setWizard] = useState(false);
  const [message, setMessage] = useState<string>();
  if (acts.isError) return <ErrorState error={acts.error} className="mb-4" />;
  const mine = (acts.data?.data ?? []).filter(
    (a) => a.jurisdiction === country,
  );
  const name =
    jurisdictions.data?.data.find((j) => j.code === country)?.name ?? country;
  const configure = can("compliance:configure");
  return (
    <section
      aria-label={`${name} activations`}
      className="mb-5 flex flex-col gap-3"
    >
      {message ? <Notice tone="ok">{message}</Notice> : null}
      {mine.map((a) => (
        <ActivationCard key={a.id} a={a} onDone={setMessage} />
      ))}
      {configure ? (
        <div>
          <Button
            variant={mine.length ? "secondary" : "primary"}
            onClick={() => {
              setMessage(undefined);
              setWizard(true);
            }}
          >
            {mine.length
              ? `Activate ${name} for another booking entity`
              : `Activate ${name}`}
          </Button>
        </div>
      ) : mine.length ? null : (
        <p className="text-body text-text-secondary">
          {name} is not switched on. A person with compliance configuration
          rights can activate it.
        </p>
      )}
      {wizard ? (
        <ActivationWizard
          country={country}
          name={name}
          open={wizard}
          onOpenChange={setWizard}
          onDone={setMessage}
        />
      ) : null}
    </section>
  );
}

function ActivationCard({
  a,
  onDone,
}: {
  a: Activation;
  onDone: (message: string) => void;
}) {
  const { can, session } = useSession();
  const userName = useUserName();
  const readiness = useActivationReadiness(
    a.status === "live" ? undefined : a.id,
  );
  const changes = useActivationChanges();
  const [move, setMove] = useState<Status | "fallback">();
  const [rationale, setRationale] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const history = (changes.data?.data ?? []).filter(
    (c) => c.activation_id === a.id,
  );
  const pending = history.find((c) => c.state === "pending");
  const configure = can("compliance:configure");

  const act = useAction(
    async () => {
      const res =
        move === "fallback"
          ? await api().POST(
              "/api/v1/compliance/activations/{activation_id}/fallback",
              {
                params: { path: { activation_id: a.id } },
                body: { rationale },
              },
            )
          : await api().POST(
              "/api/v1/compliance/activations/{activation_id}/status",
              {
                params: { path: { activation_id: a.id } },
                body: { status: move!, rationale },
              },
            );
      unwrap(res);
      return res.response.status === 202
        ? "Proposed. Another person with approval rights must approve it before it applies."
        : "Saved.";
    },
    {
      invalidate: INVALIDATE,
      onSuccess: (m) => {
        onDone(m);
        setMove(undefined);
        setRationale("");
      },
    },
  );
  const decide = useAction(
    async (approve: boolean) =>
      unwrap(
        await api().POST(
          approve
            ? "/api/v1/compliance/activation-changes/{change_id}/approve"
            : "/api/v1/compliance/activation-changes/{change_id}/reject",
          { params: { path: { change_id: pending!.id } }, body: {} },
        ),
      ),
    {
      invalidate: INVALIDATE,
      onSuccess: (c) =>
        onDone(c.state === "approved" ? "Approved and applied." : "Rejected."),
    },
  );

  const moves: { to: Status | "fallback"; label: string }[] = [];
  if (a.status === "draft" || a.status === "suspended")
    moves.push({ to: "shadow", label: "Start monitoring" });
  if (a.status !== "live") moves.push({ to: "live", label: "Go live" });
  if (a.status === "live")
    moves.push({ to: "shadow", label: "Back to monitoring" });
  if (!a.fallback && (a.status === "live" || a.status === "shadow"))
    moves.push({ to: "suspended", label: "Suspend" });
  if (!a.fallback && (a.status === "live" || a.status === "shadow"))
    moves.push({ to: "fallback", label: "Make default country" });
  const blocking = readiness.data?.items.filter((i) => i.blocking) ?? [];
  const status = STATUS[a.status];
  return (
    <Panel>
      <PanelHeader
        title={
          <span className="flex items-center gap-2">
            <span aria-hidden="true">{flag(a.jurisdiction)}</span>
            <span className="mono">{a.booking_entity}</span>
          </span>
        }
        legend={a.pack}
        actions={
          <span className="flex gap-2">
            {a.fallback ? (
              <StatusPill tone="success">Default country</StatusPill>
            ) : null}
            <StatusPill tone={status.tone}>{status.label}</StatusPill>
          </span>
        }
      />
      <div className="flex flex-col gap-4 px-5 pb-5">
        <p className="text-body text-text-secondary">{status.help}</p>
        <KeyValues
          items={[
            ["Licence type", a.licence_type],
            ["Capabilities", a.capabilities.join(", ") || "None"],
            [
              "Business day starts",
              `${String(a.day_starts_at ?? 0).padStart(2, "0")}:00 local time`,
            ],
            ...(a.activated_at
              ? ([
                  [
                    "Live since",
                    `${formatTimestamp(a.activated_at)}, proposed by ${userName(a.activated_by)}, approved by ${userName(a.approved_by)}`,
                  ],
                ] as [string, string][])
              : []),
          ]}
        />
        {a.status !== "live" && readiness.data ? (
          readiness.data.ready && !readiness.data.items.length ? (
            <Notice tone="ok">Ready to go live: nothing is missing.</Notice>
          ) : (
            <Notice tone={readiness.data.ready ? "warn" : "error"}>
              <span className="font-medium">
                {readiness.data.ready
                  ? "Ready to go live, with warnings:"
                  : `${blocking.length} ${blocking.length === 1 ? "thing blocks" : "things block"} going live:`}
              </span>
              <ul className="mt-1 list-disc pl-5">
                {readiness.data.items.map((i) => (
                  <li key={i.message}>
                    {i.blocking ? "Required: " : "Warning: "}
                    {i.message}
                  </li>
                ))}
              </ul>
            </Notice>
          )
        ) : null}
        {pending ? (
          <Notice tone="warn">
            <span className="font-medium">Awaiting approval: </span>
            {changeText(pending)}, proposed by {userName(pending.proposed_by)}{" "}
            on {formatTimestamp(pending.proposed_at)}. “{pending.rationale}”
            {decide.error ? (
              <span className="block">{decide.error}</span>
            ) : null}
            {can("rules:approve") && pending.proposed_by !== session.user.id ? (
              <span className="mt-2 flex gap-2">
                <Button
                  size="sm"
                  variant="primary"
                  disabled={decide.busy}
                  onClick={() => void decide.run(true)}
                >
                  Approve
                </Button>
                <Button
                  size="sm"
                  disabled={decide.busy}
                  onClick={() => void decide.run(false)}
                >
                  Reject
                </Button>
              </span>
            ) : (
              <span className="block">
                Another person with approval rights decides it.
              </span>
            )}
          </Notice>
        ) : null}
        {configure && !pending && !move ? (
          <div className="flex flex-wrap gap-2">
            {moves.map((m) => (
              <Button
                key={m.label}
                size="sm"
                variant={m.to === "live" ? "primary" : "secondary"}
                disabled={m.to === "live" && blocking.length > 0}
                title={
                  m.to === "live" && blocking.length > 0
                    ? "Settle the required items first."
                    : undefined
                }
                onClick={() => setMove(m.to)}
              >
                {m.label}
              </Button>
            ))}
          </div>
        ) : null}
        {move ? (
          <form
            className="flex flex-col gap-3"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              if (rationale.trim()) void act.run();
            }}
          >
            {act.error ? <Notice tone="error">{act.error}</Notice> : null}
            {move === "live" || a.status === "live" || move === "fallback" ? (
              <Notice tone="warn">
                This needs a second person: it applies when someone else with
                approval rights approves it.
              </Notice>
            ) : null}
            <Field
              label="Rationale"
              hint="Kept with the change and shown to the approver."
            >
              {(f) => (
                <Textarea
                  {...f}
                  value={rationale}
                  maxLength={1000}
                  required
                  onChange={(e) => setRationale(e.target.value)}
                />
              )}
            </Field>
            <div className="flex gap-2">
              <Button
                type="submit"
                variant="primary"
                size="sm"
                disabled={act.busy || !rationale.trim()}
              >
                {moves.find((m) => m.to === move)?.label ?? "Save"}
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => setMove(undefined)}
              >
                Cancel
              </Button>
            </div>
          </form>
        ) : null}
        <div>
          <Button
            size="sm"
            variant="ghost"
            aria-expanded={showHistory}
            onClick={() => setShowHistory((v) => !v)}
          >
            {showHistory
              ? "Hide change history"
              : `Change history (${history.length})`}
          </Button>
          {showHistory ? (
            <ol className="mt-2 flex flex-col gap-2">
              {history.map((c) => (
                <li key={c.id} className="text-body">
                  <StatusPill tone={STATE[c.state].tone}>
                    {STATE[c.state].label}
                  </StatusPill>{" "}
                  {changeText(c)}: {formatTimestamp(c.proposed_at)}, by{" "}
                  {userName(c.proposed_by)}
                  {c.decided_by ? `, decided by ${userName(c.decided_by)}` : ""}
                  . <span className="text-text-secondary">“{c.rationale}”</span>
                </li>
              ))}
            </ol>
          ) : null}
        </div>
      </div>
    </Panel>
  );
}

/** Activate a country for a booking entity: entity, licence, capabilities, day start, rationale. */
function ActivationWizard({
  country,
  name,
  open,
  onOpenChange,
  onDone,
}: {
  country: string;
  name: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: (message: string) => void;
}) {
  const pack = useCompliancePack(country);
  const [entity, setEntity] = useState("");
  const [licence, setLicence] = useState("");
  const [caps, setCaps] = useState<string[]>([]);
  const [day, setDay] = useState(0);
  const [rationale, setRationale] = useState("");
  const create = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/activations", {
          body: {
            jurisdiction: country,
            booking_entity: entity.trim(),
            licence_type: licence,
            capabilities: caps,
            day_starts_at: day,
            rationale,
          },
        }),
      ),
    {
      invalidate: INVALIDATE,
      onSuccess: (a) => {
        onDone(
          `${name} activated for ${a.booking_entity} as a draft. Check its readiness, then start monitoring.`,
        );
        onOpenChange(false);
      },
    },
  );
  const valid =
    /^[A-Za-z0-9._:-]{1,64}$/.test(entity.trim()) &&
    licence &&
    rationale.trim();
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`Activate ${name}`}
      description="Starts as a draft that judges no event. Monitoring runs every rule in shadow; going live needs a second person."
      className="max-h-[90vh] max-w-xl overflow-y-auto"
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid) void create.run();
        }}
      >
        {create.error ? <Notice tone="error">{create.error}</Notice> : null}
        <Field
          label="Booking entity"
          hint="The licensed entity your events name in compliance.booking_entity, for example gh-emi."
        >
          {(f) => (
            <Input
              {...f}
              value={entity}
              maxLength={64}
              required
              onChange={(e) => setEntity(e.target.value)}
            />
          )}
        </Field>
        <Field label="Licence type">
          {(f) => (
            <Select
              {...f}
              value={licence}
              required
              onChange={(e) => setLicence(e.target.value)}
            >
              <option value="">Choose…</option>
              {(pack.data?.licence_types ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {pack.data?.capabilities.length ? (
          <fieldset className="flex flex-col gap-2">
            <legend className="text-label text-text-secondary">
              What the entity offers
            </legend>
            {pack.data.capabilities.map((o) => (
              <label key={o.id} className="flex items-center gap-2 text-body">
                <input
                  type="checkbox"
                  checked={caps.includes(o.id)}
                  onChange={(e) =>
                    setCaps((cur) =>
                      e.target.checked
                        ? [...cur, o.id]
                        : cur.filter((c) => c !== o.id),
                    )
                  }
                />
                {o.label}
              </label>
            ))}
          </fieldset>
        ) : null}
        <Field
          label="Business day starts at"
          hint="Daily and monthly limits count from this hour, local time."
        >
          {(f) => (
            <Select
              {...f}
              value={day}
              onChange={(e) => setDay(Number(e.target.value))}
            >
              {Array.from({ length: 24 }, (_, h) => (
                <option key={h} value={h}>
                  {String(h).padStart(2, "0")}:00
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field
          label="Rationale"
          hint="Why this entity is activated; kept with the change."
        >
          {(f) => (
            <Textarea
              {...f}
              value={rationale}
              maxLength={1000}
              required
              onChange={(e) => setRationale(e.target.value)}
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
            disabled={create.busy || !valid}
          >
            Create draft
          </Button>
        </div>
      </form>
    </Modal>
  );
}

/**
 * The preset rules running for the workspace's activated countries (live or monitoring), next
 * to its own rules: the same list developers read from GET /api/v1/compliance/active-rules.
 */
export function ActiveCountryRules() {
  const q = useActiveRules();
  const rules = q.data?.data ?? [];
  const countries = [...new Set(rules.map((r) => r.jurisdiction))];
  return (
    <Panel className="mb-5 overflow-hidden">
      <PanelHeader
        title="Preset rules running"
        legend={
          q.data
            ? `${rules.length} rules in ${countries.length} ${countries.length === 1 ? "country" : "countries"}`
            : undefined
        }
      />
      {q.isError ? (
        <div className="px-5 pb-5">
          <ErrorState error={q.error} />
        </div>
      ) : q.isLoading ? (
        <table className="w-full">
          <tbody>
            <SkeletonRows columns={5} rows={3} />
          </tbody>
        </table>
      ) : !rules.length ? (
        <div className="px-5 pb-5">
          <Empty title="No preset rules are running">
            Activate a country under Preset rules to run its rules.
          </Empty>
        </div>
      ) : (
        <Table aria-label="Preset rules running">
          <thead>
            <Tr>
              <Th>Rule</Th>
              <Th>Country</Th>
              <Th>Booking entity</Th>
              <Th>Mode</Th>
              <Th>Needs</Th>
            </Tr>
          </thead>
          <tbody>
            {rules.map((r) => (
              <Tr key={`${r.booking_entity}:${r.rule}`}>
                <Td>
                  <Link
                    href={
                      `/compliance/rules/${r.rule}?jurisdiction=${r.jurisdiction}` as "/rules"
                    }
                    className="underline-offset-2 hover:underline"
                  >
                    <span className="mono">{r.rule}</span> {r.name}
                  </Link>
                </Td>
                <Td>
                  <span aria-hidden="true">{flag(r.jurisdiction)}</span>{" "}
                  {r.jurisdiction}
                </Td>
                <Td className="mono">{r.booking_entity}</Td>
                <Td>
                  <StatusPill tone={r.mode === "live" ? "success" : "warning"}>
                    {r.mode === "live" ? "Live" : "Monitoring"}
                  </StatusPill>
                </Td>
                <Td>
                  {r.unset_settings.length ? (
                    <span className="text-danger">
                      Set {r.unset_settings.join(", ")}
                    </span>
                  ) : (
                    "Ready"
                  )}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </Panel>
  );
}
