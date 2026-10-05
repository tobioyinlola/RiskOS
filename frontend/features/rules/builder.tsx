"use client";

import Link from "next/link";
import { useId, useMemo, useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { countryOptions, currencyOptions, type Option } from "@/lib/countries";
import {
  formatAmount,
  formatCount,
  formatTimestamp,
  humanize,
} from "@/lib/format";
import { useCatalog, useRules } from "@/lib/queries";
import {
  codeFromName,
  describe,
  fieldFits,
  newClause,
  newGroup,
  operators,
  toCondition,
  type CatalogField,
  type Clause,
  type Draft,
  type Group,
  type Match,
  type Tree,
  type Value,
} from "@/lib/rule-builder";
import { useSession } from "@/lib/session";

import type { RuleChange } from "./approvals";

type Rule = Schemas["Rule"];
type RecentTest = Schemas["RecentRuleTest"];
type Severity = Schemas["Severity"];
type Status = "monitor" | "active" | "off";

const SEVERITIES: Severity[] = ["low", "medium", "high", "critical"];
const EVENT_TYPE = "event.type";

const STATUS: Record<Status, { label: string; help: string }> = {
  monitor: {
    label: "Monitor first",
    help: "See what it catches without changing any score. Recommended for a new rule.",
  },
  active: {
    label: "Active",
    help: "Adds its risk points to the score of every event it matches.",
  },
  off: { label: "Off", help: "Not evaluated." },
};

const statusOf = (r?: Rule): Status =>
  !r
    ? "monitor"
    : !r.enabled
      ? "off"
      : r.mode === "shadow"
        ? "monitor"
        : "active";

/** A clause's value input: words to pick from, yes or no, a number or codes. */
function ValueInput({
  field,
  clause,
  onChange,
  label,
}: {
  field: CatalogField | undefined;
  clause: Clause;
  onChange: (v: Value | null) => void;
  label: string;
}) {
  if (!field)
    return (
      <Input
        aria-label={label}
        disabled
        placeholder="Choose what to check first"
      />
    );
  if (field.type === "boolean")
    return (
      <Select
        aria-label={label}
        value={clause.value === null ? "" : clause.value ? "yes" : "no"}
        onChange={(e) =>
          onChange(e.target.value === "" ? null : e.target.value === "yes")
        }
      >
        <option value="">Choose…</option>
        <option value="yes">Yes</option>
        <option value="no">No</option>
      </Select>
    );
  if (field.type === "number")
    return (
      <span className="flex items-center gap-2">
        <Input
          aria-label={label}
          type="number"
          inputMode="decimal"
          step={
            field.unit === "money"
              ? "0.01"
              : field.unit === "ratio"
                ? "0.1"
                : "1"
          }
          min={0}
          value={typeof clause.value === "number" ? clause.value : ""}
          onChange={(e) =>
            onChange(e.target.value === "" ? null : Number(e.target.value))
          }
          placeholder={field.unit === "money" ? "e.g. 500000" : "e.g. 3"}
        />
        {field.unit === "ratio" ? (
          <span className="text-body-sm whitespace-nowrap text-text-secondary">
            times their usual
          </span>
        ) : null}
      </span>
    );
  const chosen = Array.isArray(clause.value) ? clause.value : [];
  if (field.values?.length)
    return (
      <fieldset className="flex flex-wrap gap-1.5" aria-label={label}>
        {field.values.map((c) => {
          const on = chosen.includes(c.value);
          return (
            <label
              key={c.value}
              className={`inline-flex cursor-pointer items-center gap-1.5 rounded-full border px-2.5 py-1 text-body-sm ${
                on
                  ? "border-ink bg-bg-raised text-text-primary"
                  : "border-border-strong text-text-secondary"
              }`}
            >
              <input
                type="checkbox"
                className="size-3.5 accent-ink"
                checked={on}
                onChange={() =>
                  onChange(
                    on
                      ? chosen.filter((x) => x !== c.value)
                      : [...chosen, c.value],
                  )
                }
              />
              {c.label}
            </label>
          );
        })}
      </fieldset>
    );
  // Countries and currencies: picked by name, kept as ISO codes.
  return (
    <CodePicker
      label={label}
      noun={field.kind === "currency" ? "currency" : "country"}
      options={field.kind === "currency" ? currencyOptions() : countryOptions()}
      value={chosen}
      onChange={(codes) => onChange(codes.length ? codes : null)}
    />
  );
}

/** Pick countries or currencies by typing their name; each choice shows as a removable chip. */
function CodePicker({
  label,
  noun,
  options,
  value,
  onChange,
}: {
  label: string;
  noun: string;
  options: Option[];
  value: string[];
  onChange: (codes: string[]) => void;
}) {
  const id = useId();
  const [text, setText] = useState("");
  const name = (code: string) =>
    options.find((o) => o.code === code)?.name ?? code;
  const add = (code: string) => {
    if (!value.includes(code)) onChange([...value, code]);
    setText("");
  };
  const exact = (t: string) => {
    const q = t.trim().toLowerCase();
    return q
      ? options.find(
          (o) => o.name.toLowerCase() === q || o.code.toLowerCase() === q,
        )
      : undefined;
  };
  return (
    <div className="flex flex-col gap-2">
      {value.length ? (
        <ul className="flex flex-wrap gap-1.5" aria-label={`${label}: chosen`}>
          {value.map((code) => (
            <li
              key={code}
              className="inline-flex items-center gap-1 rounded-full border border-border-strong bg-bg-raised py-0.5 pr-1 pl-2.5 text-body-sm"
            >
              {name(code)}
              <button
                type="button"
                className="inline-flex size-5 items-center justify-center rounded-full text-text-muted hover:bg-bg-sunken hover:text-text-primary"
                aria-label={`Remove ${name(code)}`}
                onClick={() => onChange(value.filter((c) => c !== code))}
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <Input
        aria-label={label}
        list={`${id}-options`}
        value={text}
        placeholder={`Type a ${noun}, then pick it`}
        onChange={(e) => {
          const hit = exact(e.target.value);
          if (hit) add(hit.code);
          else setText(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key !== "Enter") return;
          e.preventDefault();
          const q = text.trim().toLowerCase();
          const hit = q
            ? options.find(
                (o) =>
                  o.name.toLowerCase().startsWith(q) ||
                  o.code.toLowerCase() === q,
              )
            : undefined;
          if (hit) add(hit.code);
        }}
      />
      <datalist id={`${id}-options`}>
        {options
          .filter((o) => !value.includes(o.code))
          .map((o) => (
            <option key={o.code} value={o.name} />
          ))}
      </datalist>
    </div>
  );
}

/** One condition: what to check, how to compare it and to what. */
function ConditionRow({
  clause,
  catalog,
  when,
  index,
  onChange,
  onRemove,
}: {
  clause: Clause;
  catalog: CatalogField[];
  when: string[];
  index: number;
  onChange: (c: Clause) => void;
  onRemove?: () => void;
}) {
  const field = catalog.find((f) => f.name === clause.field);
  const groups = useMemo(() => {
    const out = new Map<string, CatalogField[]>();
    for (const f of catalog) {
      if (f.name === EVENT_TYPE) continue;
      out.set(f.group, [...(out.get(f.group) ?? []), f]);
    }
    return [...out];
  }, [catalog]);
  const misfit = field && !fieldFits(field, when);
  const n = index + 1;
  return (
    <div className="flex flex-col gap-1.5">
      <div className="grid grid-cols-1 items-start gap-2 md:grid-cols-[minmax(0,1.4fr)_minmax(0,0.8fr)_minmax(0,1.4fr)_auto]">
        <Select
          aria-label={`Condition ${n}: what to check`}
          value={clause.field}
          onChange={(e) => {
            const f = catalog.find((x) => x.name === e.target.value);
            onChange({
              ...clause,
              field: e.target.value,
              op: operators(f)[0]?.op ?? "eq",
              value: null,
            });
          }}
        >
          <option value="">Choose what to check…</option>
          {groups.map(([group, fields]) => (
            <optgroup key={group} label={group}>
              {fields.map((f) => (
                <option key={f.name} value={f.name}>
                  {f.label}
                  {fieldFits(f, when) ? "" : " (not on these events)"}
                </option>
              ))}
            </optgroup>
          ))}
        </Select>
        <Select
          aria-label={`Condition ${n}: comparison`}
          value={clause.op}
          disabled={!field}
          onChange={(e) =>
            onChange({ ...clause, op: e.target.value as Clause["op"] })
          }
        >
          {operators(field).map((o) => (
            <option key={o.op} value={o.op}>
              {o.label}
            </option>
          ))}
        </Select>
        <ValueInput
          field={field}
          clause={clause}
          label={`Condition ${n}: value`}
          onChange={(value) => onChange({ ...clause, value })}
        />
        {onRemove ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onRemove}
            aria-label={`Remove condition ${n}`}
          >
            Remove
          </Button>
        ) : (
          <span />
        )}
      </div>
      {field?.unit === "money" ? (
        <p className="text-label text-text-muted">
          In the transaction&apos;s own currency, in whole units (naira, not
          kobo). Add a currency condition to limit it to one currency.
        </p>
      ) : null}
      {misfit ? (
        <p className="text-label text-status-warning">
          The events you chose never carry this, so this condition would never
          be true.
        </p>
      ) : null}
    </div>
  );
}

function Joiner({ match }: { match: Match }) {
  return (
    <p className="text-label font-medium tracking-wide text-text-muted uppercase">
      {match === "all" ? "and" : "or"}
    </p>
  );
}

function MatchSelect({
  value,
  onChange,
  label,
}: {
  value: Match;
  onChange: (m: Match) => void;
  label: string;
}) {
  return (
    <Select
      aria-label={label}
      className="h-control-sm w-auto text-body-sm"
      value={value}
      onChange={(e) => onChange(e.target.value as Match)}
    >
      <option value="all">all</option>
      <option value="any">any</option>
    </Select>
  );
}

/**
 * Create or change a rule without code: WHEN (events), IF (conditions joined by and/or, in
 * groups), THEN (risk points and severity), and how it starts. The rule is said in words
 * beside the form; nothing technical is shown.
 */
export function RuleBuilder({
  rule,
  initial,
  onSaved,
}: {
  rule?: Rule;
  initial: Draft;
  onSaved: (r: Rule | RuleChange) => void;
}) {
  const { environment } = useSession();
  const catalogQ = useCatalog();
  const rulesQ = useRules();
  const catalog = useMemo(() => catalogQ.data?.fields ?? [], [catalogQ.data]);
  const eventTypes = catalog.find((f) => f.name === EVENT_TYPE)?.values ?? [];
  const [name, setName] = useState(rule?.name ?? "");
  const [description, setDescription] = useState(rule?.description ?? "");
  const [draft, setDraft] = useState<Draft>(initial);
  const [weight, setWeight] = useState(rule?.weight ?? 20);
  const [severity, setSeverity] = useState<Severity>(
    rule?.severity ?? "medium",
  );
  const [status, setStatus] = useState<Status>(statusOf(rule));
  const [touched, setTouched] = useState(false);
  const built = toCondition(draft, catalog);
  const problem = "problem" in built ? built.problem : undefined;

  const save = useAction(
    async () => {
      if (!("tree" in built)) throw new Error(built.problem);
      const common = {
        name: name.trim(),
        description: description.trim(),
        severity,
        weight,
        mode: (status === "active"
          ? "active"
          : "shadow") as Schemas["RuleMode"],
        enabled: status !== "off",
        condition: built.tree as Record<string, unknown>,
      };
      if (rule)
        return unwrap(
          await api().PATCH("/api/v1/rules/{rule_id}", {
            params: { path: { rule_id: rule.id } },
            body: common,
          }),
        );
      const taken = new Set([
        ...(rulesQ.data?.data ?? []).map((r) => r.code),
        ...(catalogQ.data?.signals ?? []).map((s) => s.code),
      ]);
      return unwrap(
        await api().POST("/api/v1/rules", {
          body: { ...common, code: codeFromName(name, taken) },
        }),
      );
    },
    {
      invalidate: ["rules", "rule", "rule-versions", "rule-changes"],
      onSuccess: onSaved,
    },
  );

  const setItems = (items: Draft["items"]) =>
    setDraft((d) => ({ ...d, items }));
  const updateItem = (i: number, it: Draft["items"][number]) =>
    setItems(draft.items.map((x, j) => (j === i ? it : x)));
  const removeItem = (i: number) =>
    setItems(draft.items.filter((_, j) => j !== i));
  const toggleWhen = (t: string) =>
    setDraft((d) => ({
      ...d,
      when: d.when.includes(t) ? d.when.filter((x) => x !== t) : [...d.when, t],
    }));
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setTouched(true);
    if (!problem && name.trim()) void save.run();
  };
  const sentence = describe(draft, catalog, { weight, severity });
  // The number of the first condition of each item, for labels ("Condition 3: value").
  const firstNo = draft.items.map((_, i) =>
    draft.items
      .slice(0, i)
      .reduce((n, it) => n + (it.kind === "clause" ? 1 : it.clauses.length), 0),
  );

  return (
    <form
      onSubmit={onSubmit}
      className="grid grid-cols-1 items-start gap-5 xl:grid-cols-[minmax(0,1fr)_340px]"
    >
      <div className="flex min-w-0 flex-col gap-5">
        {save.error ? <Notice tone="error">{save.error}</Notice> : null}
        <Panel>
          <PanelHeader title="Name" />
          <PanelBody className="flex flex-col gap-4">
            <Field
              label="Rule name"
              error={
                touched && !name.trim() ? "Give the rule a name." : undefined
              }
            >
              {(p) => (
                <Input
                  {...p}
                  value={name}
                  maxLength={100}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="e.g. Large transfer to a new beneficiary"
                />
              )}
            </Field>
            <Field label="Why this rule exists (optional)">
              {(p) => (
                <Textarea
                  {...p}
                  value={description}
                  maxLength={1000}
                  onChange={(e) => setDescription(e.target.value)}
                />
              )}
            </Field>
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader title="When" legend="the events this rule looks at" />
          <PanelBody>
            <fieldset
              className="flex flex-wrap gap-2"
              aria-label="Events this rule looks at"
            >
              {eventTypes.map((t) => {
                const on = draft.when.includes(t.value);
                return (
                  <label
                    key={t.value}
                    className={`inline-flex cursor-pointer items-center gap-2 rounded-full border px-3 py-1.5 text-body-sm ${
                      on
                        ? "border-ink bg-bg-raised text-text-primary"
                        : "border-border-strong text-text-secondary"
                    }`}
                  >
                    <input
                      type="checkbox"
                      className="size-4 accent-ink"
                      checked={on}
                      onChange={() => toggleWhen(t.value)}
                    />
                    {t.label}
                  </label>
                );
              })}
            </fieldset>
            <p className="mt-2 text-label text-text-muted">
              {draft.when.length === 0
                ? "No event chosen: the rule looks at every event."
                : "Only these events are checked."}
            </p>
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader title="If" legend="the conditions that must be true" />
          <PanelBody className="flex flex-col gap-3">
            <p className="flex flex-wrap items-center gap-2 text-body-sm text-text-secondary">
              The rule matches when
              <MatchSelect
                label="How the conditions combine"
                value={draft.match}
                onChange={(match) => setDraft((d) => ({ ...d, match }))}
              />
              of these are true:
            </p>
            {draft.items.map((it, i) => (
              <div key={it.id} className="flex flex-col gap-3">
                {i > 0 ? <Joiner match={draft.match} /> : null}
                {it.kind === "clause" ? (
                  <ConditionRow
                    clause={it}
                    catalog={catalog}
                    when={draft.when}
                    index={firstNo[i]!}
                    onChange={(c) => updateItem(i, c)}
                    onRemove={
                      draft.items.length > 1 ? () => removeItem(i) : undefined
                    }
                  />
                ) : (
                  <GroupBox
                    group={it}
                    catalog={catalog}
                    when={draft.when}
                    firstIndex={firstNo[i]!}
                    onChange={(g) => updateItem(i, g)}
                    onRemove={() => removeItem(i)}
                  />
                )}
              </div>
            ))}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                size="sm"
                onClick={() => setItems([...draft.items, newClause()])}
              >
                Add condition
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setItems([...draft.items, newGroup()])}
              >
                Add a group
              </Button>
            </div>
            {touched && problem ? (
              <Notice tone="error">{problem}</Notice>
            ) : null}
          </PanelBody>
        </Panel>

        <Panel>
          <PanelHeader title="Then" legend="what happens when it matches" />
          <PanelBody className="flex flex-col gap-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field
                label="Risk points to add"
                hint="Added to the event's risk score (0–100); your risk settings decide what each score leads to."
              >
                {(p) => (
                  <span className="flex items-center gap-3">
                    <input
                      {...p}
                      type="range"
                      min={1}
                      max={100}
                      value={weight}
                      onChange={(e) => setWeight(Number(e.target.value))}
                      className="w-full accent-ink"
                    />
                    <span className="w-10 text-right text-body font-medium tabular-nums">
                      {weight}
                    </span>
                  </span>
                )}
              </Field>
              <Field label="How serious it is">
                {(p) => (
                  <Select
                    {...p}
                    value={severity}
                    onChange={(e) => setSeverity(e.target.value as Severity)}
                  >
                    {SEVERITIES.map((s) => (
                      <option key={s} value={s}>
                        {humanize(s)}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
            <fieldset className="flex flex-col gap-2">
              <legend className="mb-1 text-label text-text-primary">
                Status
              </legend>
              {(Object.keys(STATUS) as Status[])
                .filter((s) => rule || s !== "off")
                .map((s) => (
                  <label
                    key={s}
                    className={`flex cursor-pointer gap-3 rounded-md border px-3 py-2.5 ${
                      status === s
                        ? "border-ink bg-bg-raised"
                        : "border-border-subtle"
                    }`}
                  >
                    <input
                      type="radio"
                      name="status"
                      className="mt-1 size-4 accent-ink"
                      checked={status === s}
                      onChange={() => setStatus(s)}
                    />
                    <span>
                      <span className="block text-body-sm font-medium">
                        {STATUS[s].label}
                      </span>
                      <span className="block text-label text-text-muted">
                        {STATUS[s].help}
                      </span>
                    </span>
                  </label>
                ))}
            </fieldset>
          </PanelBody>
        </Panel>
      </div>

      <div className="flex flex-col gap-4 xl:sticky xl:top-4">
        <Panel>
          <PanelHeader title="This rule says" />
          <PanelBody className="flex flex-col gap-4">
            <p className="text-body" aria-live="polite">
              {sentence}
            </p>
            <p className="text-label text-text-muted">
              {STATUS[status].label}: {STATUS[status].help}
            </p>
            <TryRecent
              tree={"tree" in built ? built.tree : null}
              catalog={catalog}
            />
            {environment === "production" ? (
              <Notice>
                Production: saving sends the rule for approval. It applies when
                another person with approval rights approves it.
              </Notice>
            ) : null}
            <Button
              type="submit"
              variant="primary"
              disabled={save.busy || !!rule?.pending_change_id}
            >
              {environment === "production"
                ? "Send for approval"
                : rule
                  ? "Save changes"
                  : "Create rule"}
            </Button>
          </PanelBody>
        </Panel>
      </div>
    </form>
  );
}

/** A group of conditions with its own and/or. */
function GroupBox({
  group,
  catalog,
  when,
  firstIndex,
  onChange,
  onRemove,
}: {
  group: Group;
  catalog: CatalogField[];
  when: string[];
  firstIndex: number;
  onChange: (g: Group) => void;
  onRemove: () => void;
}) {
  const set = (clauses: Clause[]) => onChange({ ...group, clauses });
  return (
    <div className="flex flex-col gap-3 rounded-md border border-border-subtle bg-bg-sunken p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="flex flex-wrap items-center gap-2 text-body-sm text-text-secondary">
          A group that is true when
          <MatchSelect
            label="How this group's conditions combine"
            value={group.match}
            onChange={(match) => onChange({ ...group, match })}
          />
          of these are true:
        </p>
        <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
          Remove group
        </Button>
      </div>
      {group.clauses.map((c, i) => (
        <div key={c.id} className="flex flex-col gap-3">
          {i > 0 ? <Joiner match={group.match} /> : null}
          <ConditionRow
            clause={c}
            catalog={catalog}
            when={when}
            index={firstIndex + i}
            onChange={(x) =>
              set(group.clauses.map((y, j) => (j === i ? x : y)))
            }
            onRemove={
              group.clauses.length > 1
                ? () => set(group.clauses.filter((_, j) => j !== i))
                : undefined
            }
          />
        </div>
      ))}
      <Button
        type="button"
        size="sm"
        className="self-start"
        onClick={() => set([...group.clauses, newClause()])}
      >
        Add condition to group
      </Button>
    </div>
  );
}

/** Run the rule as it stands on recent events: what it would have matched, nothing stored. */
function TryRecent({
  tree,
  catalog,
}: {
  tree: Tree | null;
  catalog: CatalogField[];
}) {
  const [days, setDays] = useState(7);
  const [result, setResult] = useState<{
    key: string;
    days: number;
    r: RecentTest;
  }>();
  const key = JSON.stringify(tree);
  const types = catalog.find((f) => f.name === EVENT_TYPE)?.values ?? [];
  const typeLabel = (t: string) => types.find((x) => x.value === t)?.label ?? t;
  const run = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/rules/test-recent", {
          body: { condition: tree as Record<string, unknown>, days },
        }),
      ),
    { onSuccess: (r) => setResult({ key, days, r }) },
  );
  const shown = result && result.key === key ? result : undefined;
  const pct =
    shown && shown.r.checked ? (shown.r.matched / shown.r.checked) * 100 : 0;
  return (
    <div className="flex flex-col gap-3 border-t border-border-subtle pt-4">
      <p className="text-label font-medium text-text-primary">
        Try it on recent events
      </p>
      <div className="flex items-center gap-2">
        <Select
          aria-label="Period to try"
          className="h-control-sm w-auto text-body-sm"
          value={days}
          onChange={(e) => setDays(Number(e.target.value))}
        >
          <option value={1}>Last 24 hours</option>
          <option value={7}>Last 7 days</option>
          <option value={30}>Last 30 days</option>
        </Select>
        <Button
          type="button"
          size="sm"
          disabled={!tree || run.busy}
          onClick={() => void run.run()}
        >
          Try it
        </Button>
      </div>
      {!tree ? (
        <p className="text-label text-text-muted">
          Finish the conditions to try the rule.
        </p>
      ) : null}
      {run.error ? <Notice tone="error">{run.error}</Notice> : null}
      {shown ? (
        <div className="flex flex-col gap-2" role="status">
          <p className="text-body-sm">
            Would have matched <strong>{formatCount(shown.r.matched)}</strong>{" "}
            of {formatCount(shown.r.checked)} events in the{" "}
            {shown.days === 1 ? "last 24 hours" : `last ${shown.days} days`}
            {shown.r.checked
              ? ` (${pct < 0.1 && pct > 0 ? "<0.1" : pct.toFixed(1)}%)`
              : ""}
            .
          </p>
          {shown.r.truncated ? (
            <p className="text-label text-text-muted">
              Only the newest 5,000 events were checked.
            </p>
          ) : null}
          {shown.r.sample.length ? (
            <ul className="flex flex-col gap-1 text-label">
              {shown.r.sample.slice(0, 5).map((m) => (
                <li key={m.evaluation_id}>
                  <Link
                    href={`/events?event=${m.event_id}` as "/events"}
                    className="text-text-secondary hover:text-accent"
                  >
                    {formatTimestamp(m.at)} · {typeLabel(m.event_type)}
                    {m.amount !== null && m.currency
                      ? ` · ${formatAmount(m.amount, m.currency)}`
                      : ""}{" "}
                    · {humanize(m.decision)}
                  </Link>
                </li>
              ))}
            </ul>
          ) : null}
          <p className="text-label text-text-muted">
            Uses what was known when each event happened; nothing is stored.
          </p>
        </div>
      ) : null}
    </div>
  );
}
