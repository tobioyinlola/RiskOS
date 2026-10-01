"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import {
  CodeBlock,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Tag,
} from "@/components/ui/misc";
import {
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import {
  useCatalog,
  useRule,
  useRules,
  useRuleVersions,
  useUserName,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import {
  isChange,
  PendingChangeNotice,
  PendingChanges,
  type RuleChange,
} from "./approvals";

type Rule = Schemas["Rule"];

const SEVERITIES: Schemas["Severity"][] = ["low", "medium", "high", "critical"];

export function RuleList() {
  const q = useRules();
  const { can, environment } = useSession();
  const rules = q.data?.data ?? [];
  return (
    <>
      <PageHeader
        title="Rules"
        description={
          environment === "production"
            ? "Your own signals: typed conditions that add weight to a score. In production every change needs a second person's approval."
            : "Your own signals: typed conditions that add weight to a score. Versioned; every change can be rolled back."
        }
        actions={
          can("rules:write") ? (
            <Button variant="primary" asChild>
              <Link href="/rules/new">Create rule</Link>
            </Button>
          ) : null
        }
      />
      <PendingChanges />
      <Panel className="overflow-hidden">
        <Table aria-label="Rules">
          <thead>
            <tr>
              <Th>Rule</Th>
              <Th>Mode</Th>
              <Th>Severity</Th>
              <Th className="text-right">Weight</Th>
              <Th className="text-right">Version</Th>
              <Th>Changed</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {rules.map((r) => (
              <Tr key={r.id}>
                <Td>
                  <Link
                    href={`/rules/${r.id}` as "/rules"}
                    className="block text-body-sm hover:text-accent"
                  >
                    {r.name}
                  </Link>
                  <span className="mono text-label text-text-muted">
                    {r.code}
                  </span>
                </Td>
                <Td>
                  <Tag>
                    {!r.enabled
                      ? "Disabled"
                      : r.mode === "shadow"
                        ? "Shadow"
                        : "Active"}
                  </Tag>
                  {r.pending_change_id ? (
                    <Tag className="ml-1">Awaiting approval</Tag>
                  ) : null}
                </Td>
                <Td className="text-body-sm">{humanize(r.severity)}</Td>
                <DataCell className="text-right">+{r.weight}</DataCell>
                <DataCell className="text-right">v{r.version}</DataCell>
                <DataCell className="text-text-muted">
                  {formatAge(r.updated_at)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rules.length === 0 ? (
          <Empty title="No rules yet">
            Built-in signals already score every event. Add a rule for what only
            you know; start it in shadow mode to see what it would do.
          </Empty>
        ) : null}
      </Panel>
    </>
  );
}

const EXAMPLE = `{
  "all": [
    { "field": "transaction.amount", "op": "gte", "value": 1000000 },
    { "field": "feature.new_beneficiary", "op": "eq", "value": true }
  ]
}`;

interface Draft {
  code: string;
  name: string;
  description: string;
  severity: Schemas["Severity"];
  weight: number;
  mode: Schemas["RuleMode"];
  enabled: boolean;
  condition: string;
}

function toDraft(r?: Rule): Draft {
  return {
    code: r?.code ?? "",
    name: r?.name ?? "",
    description: r?.description ?? "",
    severity: r?.severity ?? "medium",
    weight: r?.weight ?? 20,
    mode: r?.mode ?? "shadow",
    enabled: r?.enabled ?? true,
    condition: r ? JSON.stringify(r.condition, null, 2) : EXAMPLE,
  };
}

function parseCondition(text: string): {
  value?: Record<string, unknown>;
  error?: string;
} {
  try {
    const v = JSON.parse(text) as unknown;
    if (typeof v !== "object" || v === null || Array.isArray(v))
      return { error: "The condition must be a JSON object." };
    return { value: v as Record<string, unknown> };
  } catch (e) {
    return {
      error: `The condition is not valid JSON: ${(e as Error).message}`,
    };
  }
}

function FieldCatalog() {
  const q = useCatalog();
  return (
    <Panel>
      <PanelHeader title="Fields" legend="catalog" />
      <PanelBody className="max-h-[520px] overflow-y-auto">
        <p className="mb-3 text-body-sm text-text-secondary">
          A condition is <code className="mono">all</code>,{" "}
          <code className="mono">any</code>, <code className="mono">not</code>{" "}
          or a comparison. A field the event does not have never matches.
        </p>
        <ul>
          {(q.data?.fields ?? []).map((f) => (
            <li key={f.name} className="border-t border-border-subtle py-1.5">
              <code className="mono text-data">{f.name}</code>
              <span className="ml-2 text-label text-text-muted">{f.type}</span>
              <p className="mono text-label text-text-muted">
                {f.operators.join(" ")}
              </p>
            </li>
          ))}
        </ul>
      </PanelBody>
    </Panel>
  );
}

function RuleForm({
  rule,
  onSaved,
}: {
  rule?: Rule;
  onSaved: (r: Rule | RuleChange) => void;
}) {
  const { environment } = useSession();
  const [d, setD] = useState<Draft>(() => toDraft(rule));
  const [conditionError, setConditionError] = useState<string>();
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) =>
    setD((x) => ({ ...x, [k]: v }));
  const save = useAction(
    async () => {
      const cond = parseCondition(d.condition);
      if (!cond.value) throw new Error(cond.error);
      const common = {
        name: d.name,
        description: d.description,
        severity: d.severity,
        weight: d.weight,
        mode: d.mode,
        enabled: d.enabled,
        condition: cond.value,
      };
      return rule
        ? unwrap(
            await api().PATCH("/api/v1/rules/{rule_id}", {
              params: { path: { rule_id: rule.id } },
              body: common,
            }),
          )
        : unwrap(
            await api().POST("/api/v1/rules", {
              body: { ...common, code: d.code },
            }),
          );
    },
    {
      invalidate: ["rules", "rule", "rule-versions", "rule-changes"],
      onSuccess: onSaved,
    },
  );
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        const cond = parseCondition(d.condition);
        setConditionError(cond.error);
        if (cond.value) void save.run();
      }}
    >
      {save.error ? <Notice tone="error">{save.error}</Notice> : null}
      {!rule ? (
        <Field
          label="Code"
          hint="The signal code it emits: lowercase letters, digits and _. Cannot change later."
        >
          {(p) => (
            <Input
              {...p}
              value={d.code}
              onChange={(e) => set("code", e.target.value)}
              required
              pattern="^[a-z][a-z0-9_]{2,63}$"
              className="mono"
            />
          )}
        </Field>
      ) : null}
      <Field label="Name">
        {(p) => (
          <Input
            {...p}
            value={d.name}
            onChange={(e) => set("name", e.target.value)}
            required
            maxLength={100}
          />
        )}
      </Field>
      <Field label="Description">
        {(p) => (
          <Textarea
            {...p}
            value={d.description}
            onChange={(e) => set("description", e.target.value)}
            maxLength={1000}
          />
        )}
      </Field>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <Field label="Weight" hint="Points added (0–100).">
          {(p) => (
            <Input
              {...p}
              type="number"
              min={0}
              max={100}
              value={d.weight}
              onChange={(e) => set("weight", Number(e.target.value))}
              className="mono"
            />
          )}
        </Field>
        <Field label="Severity">
          {(p) => (
            <Select
              {...p}
              value={d.severity}
              onChange={(e) =>
                set("severity", e.target.value as Schemas["Severity"])
              }
            >
              {SEVERITIES.map((s) => (
                <option key={s} value={s}>
                  {humanize(s)}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Mode" hint="Shadow: recorded, not scored.">
          {(p) => (
            <Select
              {...p}
              value={d.mode}
              onChange={(e) =>
                set("mode", e.target.value as Schemas["RuleMode"])
              }
            >
              <option value="shadow">Shadow</option>
              <option value="active">Active</option>
            </Select>
          )}
        </Field>
        <Field label="Status">
          {(p) => (
            <Select
              {...p}
              value={d.enabled ? "on" : "off"}
              onChange={(e) => set("enabled", e.target.value === "on")}
            >
              <option value="on">Enabled</option>
              <option value="off">Disabled</option>
            </Select>
          )}
        </Field>
      </div>
      <Field label="Condition" error={conditionError}>
        {(p) => (
          <Textarea
            {...p}
            value={d.condition}
            onChange={(e) => set("condition", e.target.value)}
            className="mono min-h-64 text-data"
            spellCheck={false}
          />
        )}
      </Field>
      {environment === "production" ? (
        <Notice>
          Production: saving proposes the change. It applies when another person
          with approval rights approves it.
        </Notice>
      ) : null}
      <Button
        type="submit"
        variant="primary"
        className="self-start"
        disabled={save.busy || !!rule?.pending_change_id}
      >
        {environment === "production"
          ? "Propose for approval"
          : rule
            ? "Save as a new version"
            : "Create rule"}
      </Button>
    </form>
  );
}

export function NewRule() {
  const router = useRouter();
  return (
    <>
      <PageHeader
        title="Create rule"
        description="New rules start in shadow mode: evaluated and recorded, but not scored until you make them active."
      />
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_340px]">
        <Panel>
          <PanelBody>
            <RuleForm
              onSaved={(r) =>
                router.push(
                  isChange(r) ? "/rules" : (`/rules/${r.id}` as "/rules"),
                )
              }
            />
          </PanelBody>
        </Panel>
        <FieldCatalog />
      </div>
    </>
  );
}

export function RuleDetail({ id }: { id: string }) {
  const q = useRule(id);
  const { can } = useSession();
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  const r = q.data!;
  return (
    <>
      <PageHeader
        title={r.name}
        description={
          <span className="mono text-data text-text-muted">
            {r.code} · v{r.version}
          </span>
        }
      />
      <PendingChangeNotice rule={r} />
      <Tabs defaultValue={can("rules:write") ? "edit" : "test"}>
        <TabList className="mb-4">
          {can("rules:write") ? <Tab value="edit">Edit</Tab> : null}
          <Tab value="test">Test</Tab>
          <Tab value="versions">Versions</Tab>
        </TabList>
        {can("rules:write") ? (
          <TabPanel value="edit">
            <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_340px]">
              <Panel>
                <PanelBody>
                  <RuleForm
                    key={r.version}
                    rule={r}
                    onSaved={() => undefined}
                  />
                </PanelBody>
              </Panel>
              <FieldCatalog />
            </div>
          </TabPanel>
        ) : null}
        <TabPanel value="test">
          <RuleTester rule={r} />
        </TabPanel>
        <TabPanel value="versions">
          <RuleVersions rule={r} />
        </TabPanel>
      </Tabs>
    </>
  );
}

const SAMPLE_EVENT = `{
  "event_id": "rule-test-1",
  "type": "transaction",
  "occurred_at": "${new Date().toISOString()}",
  "actor": { "user_id": "u_123", "account_id": "acc_123", "device_id": "dev_123" },
  "context": { "ip": "203.0.113.7", "country": "NG" },
  "data": {
    "transaction_id": "t_1", "amount": 2500000, "currency": "NGN",
    "direction": "debit", "beneficiary_id": "ben_9", "channel": "bank_transfer"
  }
}`;

function RuleTester({ rule }: { rule: Rule }) {
  const [event, setEvent] = useState(SAMPLE_EVENT);
  const [draft, setDraft] = useState("");
  const [result, setResult] = useState<Schemas["TestRuleResult"]>();
  const test = useAction(
    async () => {
      const ev = JSON.parse(event) as Schemas["Event"];
      const body: Schemas["TestRuleRequest"] = { event: ev };
      if (draft.trim())
        body.condition = JSON.parse(draft) as Record<string, unknown>;
      return unwrap(
        await api().POST("/api/v1/rules/{rule_id}/test", {
          params: { path: { rule_id: rule.id } },
          body,
        }),
      );
    },
    { onSuccess: setResult },
  );
  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <Panel>
        <PanelHeader title="Test against an event" legend="nothing is stored" />
        <PanelBody className="flex flex-col gap-4">
          {test.error ? <Notice tone="error">{test.error}</Notice> : null}
          <Field label="Event (JSON, as sent to POST /api/v1/events)">
            {(p) => (
              <Textarea
                {...p}
                value={event}
                onChange={(e) => setEvent(e.target.value)}
                className="mono min-h-72 text-data"
                spellCheck={false}
              />
            )}
          </Field>
          <Field
            label="Draft condition (optional)"
            hint="Test a change before saving it. Leave empty to test the current version."
          >
            {(p) => (
              <Textarea
                {...p}
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                className="mono text-data"
                spellCheck={false}
              />
            )}
          </Field>
          <Button
            variant="primary"
            className="self-start"
            onClick={() => void test.run()}
            disabled={test.busy}
          >
            Run test
          </Button>
        </PanelBody>
      </Panel>
      <Panel>
        <PanelHeader title="Result" />
        <PanelBody>
          {result ? (
            <div className="flex flex-col gap-3">
              <p className="text-heading-2">
                {result.matched ? "Matches" : "Does not match"}
              </p>
              <p className="text-label text-text-muted">
                {result.draft
                  ? "Draft condition"
                  : `Version ${result.version ?? ""}`}{" "}
                · facts the condition read
              </p>
              <CodeBlock value={result.facts} />
            </div>
          ) : (
            <p className="text-body-sm text-text-muted">
              Run a test to see whether the rule matches and which facts it
              read.
            </p>
          )}
        </PanelBody>
      </Panel>
    </div>
  );
}

function RuleVersions({ rule }: { rule: Rule }) {
  const q = useRuleVersions(rule.id);
  const { can } = useSession();
  const userName = useUserName();
  const rollback = useAction(
    async (version: number) =>
      unwrap(
        await api().POST("/api/v1/rules/{rule_id}/rollback", {
          params: { path: { rule_id: rule.id } },
          body: { version },
        }),
      ),
    { invalidate: ["rule", "rules", "rule-versions", "rule-changes"] },
  );
  return (
    <Panel className="overflow-hidden">
      {rollback.error ? <Notice tone="error">{rollback.error}</Notice> : null}
      <Table aria-label="Versions">
        <thead>
          <tr>
            <Th>Version</Th>
            <Th>Mode</Th>
            <Th className="text-right">Weight</Th>
            <Th>By</Th>
            <Th>When</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={6} rows={3} /> : null}
          {[...(q.data?.data ?? [])].reverse().map((v) => (
            <Tr
              key={v.version}
              selected={v.version === q.data?.current_version}
            >
              <DataCell>
                v{v.version}
                {v.restored_from ? (
                  <span className="ml-2 text-text-muted">
                    restores v{v.restored_from}
                  </span>
                ) : null}
              </DataCell>
              <Td>
                <Tag>{!v.enabled ? "Disabled" : humanize(v.mode)}</Tag>
              </Td>
              <DataCell className="text-right">+{v.weight}</DataCell>
              <Td className="text-body-sm text-text-secondary">
                {userName(v.created_by)}
              </Td>
              <DataCell
                className="text-text-muted"
                title={formatTimestamp(v.created_at)}
              >
                {formatAge(v.created_at)}
              </DataCell>
              <Td className="text-right">
                {can("rules:write") &&
                !rule.pending_change_id &&
                v.version !== q.data?.current_version ? (
                  <Button
                    size="sm"
                    onClick={() => void rollback.run(v.version)}
                    disabled={rollback.busy}
                  >
                    Restore
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
    </Panel>
  );
}
