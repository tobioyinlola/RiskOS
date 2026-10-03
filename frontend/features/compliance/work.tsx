"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/field";
import { Tag } from "@/components/ui/misc";
import { Modal } from "@/components/ui/overlay";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
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
import { formatDue, isReport, TASK_LABEL } from "@/lib/compliance";
import { formatAmount, formatTimestamp } from "@/lib/format";
import {
  useComplianceTasks,
  useRestrictedSignals,
  useUserName,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

import { RuleLink } from "./library";

type Task = Schemas["ComplianceTask"];
type Status = Schemas["ComplianceTaskStatus"];
type Escalation = Schemas["ScreeningEscalation"];

/** What each escalation means, by the rule whose deadline passed. */
const ESCALATION: Record<
  Escalation["rule"],
  { one: string; many: string; tone: "warn" | "error" }
> = {
  "GOV-11": {
    one: "urgent alert nobody has picked up within the fraud response target",
    many: "urgent alerts nobody has picked up within the fraud response target",
    tone: "error",
  },
  "REG-14": {
    one: "alert is past its screening deadline",
    many: "alerts are past their screening deadline",
    tone: "warn",
  },
};

const INVALIDATE = ["compliance"];

const STATUS: Record<Status, { label: string; tone: Tone }> = {
  open: { label: "Open", tone: "warning" },
  approved: { label: "Approved", tone: "info" },
  filed: { label: "Filed", tone: "success" },
  done: { label: "Done", tone: "success" },
  dismissed: { label: "Dismissed", tone: "neutral" },
};

const CATEGORIES = [
  "terrorism_financing",
  "kidnapping_ransom",
  "corruption",
  "fraud",
  "drug_trafficking",
  "general",
  "other",
] as const;

const text = (v: unknown) => (typeof v === "string" ? v : "");

/** Compliance → Work queue: what regulatory controls require, with legal deadlines. */
export function WorkQueue() {
  const { can } = useSession();
  const [status, setStatus] = useState<Status | "">("open");
  const q = useComplianceTasks(status || undefined);
  const [open, setOpen] = useState<Task>();
  const sensitive = can("compliance:sensitive");
  const tasks = q.data?.data ?? [];
  // Countdowns are as of when the queue was read.
  const now = new Date(q.dataUpdatedAt);
  return (
    <>
      <PageHeader
        title="Work queue"
        description="Reports and actions your regulatory controls require, with their legal deadlines. Reports are approved by a compliance officer and filed by a person: RiskOS never submits them."
      />
      {(["GOV-11", "REG-14"] as const).map((rule) => (
        <Escalations
          key={rule}
          rule={rule}
          list={(q.data?.escalations ?? []).filter((e) => e.rule === rule)}
          now={now}
        />
      ))}
      <div className="mb-3 flex items-center gap-2">
        <Select
          aria-label="Status"
          className="h-control-sm w-48 text-body-sm"
          value={status}
          onChange={(e) => setStatus(e.target.value as Status | "")}
        >
          <option value="">All tasks</option>
          {(Object.keys(STATUS) as Status[]).map((s) => (
            <option key={s} value={s}>
              {STATUS[s].label}
            </option>
          ))}
        </Select>
      </div>
      <Panel className="mb-5 overflow-hidden">
        <Table aria-label="Compliance tasks">
          <thead>
            <tr>
              <Th>Task</Th>
              <Th>Customer</Th>
              <Th>Deadline</Th>
              <Th>Status</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={5} /> : null}
            {tasks.map((t) => (
              <Tr key={t.id}>
                <Td className="py-2.5">
                  <span className="font-medium">
                    {TASK_LABEL[t.kind] ?? t.kind}
                  </span>
                  <span className="block text-label text-text-muted">
                    <RuleLink id={t.rule} />
                    {t.sensitive ? " · compliance only" : ""}
                  </span>
                </Td>
                <Td className="mono text-data">
                  {t.subject_ref ?? "–"}
                  {t.account_ref ? (
                    <span className="block text-label text-text-muted">
                      account {t.account_ref}
                    </span>
                  ) : null}
                </Td>
                <Td
                  className={
                    t.overdue
                      ? "text-body-sm font-medium text-status-danger"
                      : "text-body-sm text-text-secondary"
                  }
                >
                  {t.due_at
                    ? t.status === "open" || t.status === "approved"
                      ? formatDue(t.due_at, now)
                      : formatTimestamp(t.due_at)
                    : "No deadline set"}
                </Td>
                <Td>
                  <StatusPill tone={STATUS[t.status].tone}>
                    {STATUS[t.status].label}
                  </StatusPill>
                </Td>
                <Td className="text-right">
                  <Button size="sm" onClick={() => setOpen(t)}>
                    Open
                  </Button>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && tasks.length === 0 ? (
          <Empty title="Nothing to do">
            Tasks appear when a regulatory control requires something of your
            institution.
          </Empty>
        ) : null}
      </Panel>
      {q.data?.unreported_fraud.length ? (
        <UnreportedFraud list={q.data.unreported_fraud} onPrepared={setOpen} />
      ) : null}
      {sensitive ? <RestrictedSignals /> : null}
      {open ? (
        <TaskDialog
          key={open.id}
          task={open}
          onClose={() => setOpen(undefined)}
        />
      ) : null}
    </>
  );
}

/** Alerts past one rule's deadline, at most five named, each linking to the alert. */
function Escalations({
  rule,
  list,
  now,
}: {
  rule: Escalation["rule"];
  list: Escalation[];
  now: Date;
}) {
  if (!list.length) return null;
  const e = ESCALATION[rule];
  return (
    <div className="mb-5">
      <Notice tone={e.tone}>
        <span className="font-medium">
          {list.length === 1 ? `1 ${e.one}` : `${list.length} ${e.many}`} (
          {rule}):{" "}
        </span>
        {list.slice(0, 5).map((a, i) => (
          <span key={a.alert_id}>
            {i ? "; " : ""}
            <Link
              className="underline"
              href={`/alerts?status=all&alert=${a.alert_id}` as "/alerts"}
            >
              {a.title}
            </Link>{" "}
            ({a.severity}, {formatDue(a.due_at, now)})
          </span>
        ))}
        {list.length > 5 ? `; and ${list.length - 5} more` : ""}
      </Notice>
    </div>
  );
}

/** Confirmed-fraud cases without an industry fraud report (GOV-12). */
function UnreportedFraud({
  list,
  onPrepared,
}: {
  list: Schemas["UnreportedFraudCase"][];
  onPrepared: (t: Task) => void;
}) {
  const { can } = useSession();
  const prepare = useAction(
    async (caseId: string) =>
      unwrap(
        await api().POST("/api/v1/compliance/fraud-reports", {
          body: { case_id: caseId },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: onPrepared },
  );
  return (
    <Panel className="mb-5 overflow-hidden">
      <PanelHeader
        title="Confirmed fraud not yet reported"
        legend="GOV-12: report confirmed fraud to the industry channel (NIBSS)"
      />
      {prepare.error ? (
        <div className="px-5 pb-3">
          <Notice tone="error">{prepare.error}</Notice>
        </div>
      ) : null}
      <Table aria-label="Confirmed fraud not yet reported">
        <thead>
          <tr>
            <Th>Case</Th>
            <Th>Confirmed</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {list.map((c) => (
            <Tr key={c.case_id}>
              <Td className="py-2.5">
                <Link
                  className="font-medium underline"
                  href={`/cases/${c.case_id}` as "/cases"}
                >
                  {c.title}
                </Link>
              </Td>
              <Td className="text-body-sm text-text-secondary">
                {formatTimestamp(c.resolved_at)}
              </Td>
              <Td className="text-right">
                {can("compliance:write") ? (
                  <Button
                    size="sm"
                    disabled={prepare.busy}
                    aria-label={`Prepare a report for ${c.title}`}
                    onClick={() => void prepare.run(c.case_id)}
                  >
                    Prepare report
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

function ReportView({ task }: { task: Task }) {
  const r = task.report;
  const amount =
    typeof r.amount === "number" && typeof r.currency === "string"
      ? formatAmount(r.amount, r.currency)
      : null;
  const indicators = Array.isArray(r.indicators) ? r.indicators : [];
  return (
    <dl className="grid grid-cols-[140px_1fr] gap-x-3 gap-y-1 text-body-sm">
      {amount ? (
        <>
          <dt className="text-text-muted">Amount</dt>
          <dd className="mono">{amount}</dd>
        </>
      ) : null}
      {text(r.category) ? (
        <>
          <dt className="text-text-muted">Category</dt>
          <dd>{text(r.category).replaceAll("_", " ")}</dd>
        </>
      ) : null}
      {text(r.predicate_offence) ? (
        <>
          <dt className="text-text-muted">Predicate offence</dt>
          <dd>{text(r.predicate_offence)}</dd>
        </>
      ) : null}
      {indicators.length ? (
        <>
          <dt className="text-text-muted">Indicators</dt>
          <dd>{indicators.join("; ")}</dd>
        </>
      ) : null}
      {text(r.narrative) ? (
        <>
          <dt className="text-text-muted">Narrative</dt>
          <dd className="whitespace-pre-wrap">{text(r.narrative)}</dd>
        </>
      ) : null}
    </dl>
  );
}

/** One task: edit a draft report, approve, file, export, or close. */
function TaskDialog({
  task: initial,
  onClose,
}: {
  task: Task;
  onClose: () => void;
}) {
  // The task as the last action returned it: an approved report leaves the "open" list.
  const [task, setTask] = useState(initial);
  const { can } = useSession();
  const userName = useUserName();
  const [category, setCategory] = useState(text(task.report.category));
  const [offence, setOffence] = useState(text(task.report.predicate_offence));
  const [indicators, setIndicators] = useState(
    (Array.isArray(task.report.indicators) ? task.report.indicators : []).join(
      "\n",
    ),
  );
  const [narrative, setNarrative] = useState(text(task.report.narrative));
  const [filingRef, setFilingRef] = useState("");
  const [note, setNote] = useState("");
  const report = isReport(task.kind);
  const draft = report && task.status === "open";
  const live = task.status === "open" || task.status === "approved";
  const officer = can("compliance:sensitive");
  const writable = can("compliance:write") && (!task.sensitive || officer);
  const path = { params: { path: { task_id: task.id } } };

  const save = useAction(
    async () =>
      unwrap(
        await api().PATCH("/api/v1/compliance/tasks/{task_id}/report", {
          ...path,
          body: {
            ...(category
              ? { category: category as (typeof CATEGORIES)[number] }
              : {}),
            predicate_offence: offence,
            narrative,
            indicators: indicators
              .split("\n")
              .map((i) => i.trim())
              .filter(Boolean),
          },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: setTask },
  );
  const approve = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/tasks/{task_id}/approve", path),
      ),
    { invalidate: INVALIDATE, onSuccess: setTask },
  );
  const file = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/tasks/{task_id}/file", {
          ...path,
          body: { filing_ref: filingRef.trim() },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: setTask },
  );
  const close = useAction(
    async (status: "done" | "dismissed") =>
      unwrap(
        await api().POST("/api/v1/compliance/tasks/{task_id}/close", {
          ...path,
          body: { status, ...(note.trim() ? { note: note.trim() } : {}) },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: onClose },
  );
  const exportReport = useAction(
    async () => {
      const t = unwrap(
        await api().GET("/api/v1/compliance/tasks/{task_id}/export", path),
      );
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(t, null, 2)], { type: "application/json" }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = `${task.kind}-${task.id}.json`;
      a.click();
      URL.revokeObjectURL(url);
      return t;
    },
    { invalidate: INVALIDATE, onSuccess: setTask },
  );
  const error =
    save.error ??
    approve.error ??
    file.error ??
    close.error ??
    exportReport.error;
  const onSave = (e: FormEvent) => {
    e.preventDefault();
    void save.run();
  };
  return (
    <Modal
      open
      onOpenChange={(o) => (o ? undefined : onClose())}
      title={TASK_LABEL[task.kind] ?? task.kind}
      description={`Required by ${task.rule}${task.subject_ref ? ` for customer ${task.subject_ref}` : ""}. ${STATUS[task.status].label}${task.approved_by ? `, approved by ${userName(task.approved_by)}` : ""}${task.filing_ref ? `, filed as ${task.filing_ref}` : ""}.`}
      className="max-h-[90vh] max-w-xl overflow-y-auto"
    >
      <div className="flex flex-col gap-4">
        {error ? <Notice tone="error">{error}</Notice> : null}
        {report && !draft ? <ReportView task={task} /> : null}
        {draft && writable ? (
          <form onSubmit={onSave} className="flex flex-col gap-3">
            {task.kind === "report.STR" ? (
              <>
                <Field label="Category">
                  {(f) => (
                    <Select
                      {...f}
                      value={category}
                      onChange={(e) => setCategory(e.target.value)}
                    >
                      <option value="">Choose…</option>
                      {CATEGORIES.map((c) => (
                        <option key={c} value={c}>
                          {c.replaceAll("_", " ")}
                        </option>
                      ))}
                    </Select>
                  )}
                </Field>
                <Field label="Predicate offence">
                  {(f) => (
                    <Input
                      {...f}
                      value={offence}
                      maxLength={4000}
                      onChange={(e) => setOffence(e.target.value)}
                    />
                  )}
                </Field>
                <Field label="Indicators" hint="One per line.">
                  {(f) => (
                    <Textarea
                      {...f}
                      value={indicators}
                      onChange={(e) => setIndicators(e.target.value)}
                    />
                  )}
                </Field>
              </>
            ) : (
              <ReportView task={task} />
            )}
            <Field label="Narrative">
              {(f) => (
                <Textarea
                  {...f}
                  value={narrative}
                  maxLength={4000}
                  onChange={(e) => setNarrative(e.target.value)}
                />
              )}
            </Field>
            <div className="flex flex-wrap justify-end gap-2">
              <Button type="submit" disabled={save.busy}>
                Save draft
              </Button>
              {officer ? (
                <Button
                  type="button"
                  variant="primary"
                  disabled={approve.busy}
                  onClick={() => void approve.run()}
                >
                  Approve for filing
                </Button>
              ) : null}
            </div>
            {!officer ? (
              <p className="text-body-sm text-text-muted">
                A compliance officer approves reports before they are filed.
              </p>
            ) : null}
          </form>
        ) : null}
        {report && task.status === "approved" && writable ? (
          <div className="flex flex-col gap-3 rounded-md border border-border-subtle p-3">
            <p className="text-body-sm text-text-secondary">
              {task.kind === "report.industry_fraud"
                ? "Report it to the industry fraud channel (NIBSS), then record the reference it gave."
                : "Enter the report in the regulator\u2019s portal (goAML for the NFIU), then record the reference it gave."}
            </p>
            <div className="flex flex-wrap items-end gap-2">
              <Button
                type="button"
                disabled={exportReport.busy}
                onClick={() => void exportReport.run()}
              >
                Export
              </Button>
              {officer ? (
                <>
                  <Field label="Filing reference" className="min-w-48 flex-1">
                    {(f) => (
                      <Input
                        {...f}
                        value={filingRef}
                        maxLength={128}
                        onChange={(e) => setFilingRef(e.target.value)}
                      />
                    )}
                  </Field>
                  <Button
                    type="button"
                    variant="primary"
                    disabled={file.busy || !filingRef.trim()}
                    onClick={() => void file.run()}
                  >
                    Record filing
                  </Button>
                </>
              ) : null}
            </div>
          </div>
        ) : null}
        {live && writable ? (
          <div className="flex flex-col gap-2">
            <Field
              label="Note"
              hint={
                report
                  ? "Needed to dismiss the report."
                  : "Optional when done; needed to dismiss."
              }
            >
              {(f) => (
                <Textarea
                  {...f}
                  value={note}
                  maxLength={1000}
                  onChange={(e) => setNote(e.target.value)}
                />
              )}
            </Field>
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="danger"
                disabled={close.busy || !note.trim()}
                onClick={() => void close.run("dismissed")}
              >
                Dismiss
              </Button>
              {!report ? (
                <Button
                  type="button"
                  variant="primary"
                  disabled={close.busy}
                  onClick={() => void close.run("done")}
                >
                  Mark done
                </Button>
              ) : null}
            </div>
          </div>
        ) : null}
        {!live && task.close_note ? (
          <p className="text-body-sm text-text-secondary">
            Note: {task.close_note}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}

/** Compliance-only signals, for compliance officers (GOV-09). */
function RestrictedSignals() {
  const q = useRestrictedSignals(true);
  const rows = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Compliance-only signals"
        legend="last 7 days, visible to compliance officers only"
      />
      <p className="max-w-prose px-5 pb-3 text-body-sm text-text-secondary">
        Everywhere else these appear as &ldquo;Compliance review&rdquo;, so
        nothing tips off the customer or staff outside compliance.
      </p>
      <Table aria-label="Compliance-only signals">
        <thead>
          <tr>
            <Th>Rule</Th>
            <Th>Event</Th>
            <Th>When</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={3} rows={2} /> : null}
          {rows.flatMap((e) =>
            e.signals.map((s) => (
              <Tr key={`${e.evaluation_id}-${s.rule}`}>
                <Td className="py-2.5">
                  <RuleLink id={s.rule} /> {s.name}{" "}
                  {s.mode === "shadow" ? <Tag>shadow</Tag> : null}
                </Td>
                <Td className="mono text-data">{e.event_id ?? "–"}</Td>
                <Td className="text-body-sm text-text-secondary">
                  {formatTimestamp(e.created_at)}
                </Td>
              </Tr>
            )),
          )}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && rows.length === 0 ? (
        <Empty title="None in the last 7 days" />
      ) : null}
    </Panel>
  );
}
