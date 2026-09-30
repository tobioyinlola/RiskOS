"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input, Select } from "@/components/instrument/field";
import { KeyValues, Tag } from "@/components/instrument/misc";
import { Drawer, Modal } from "@/components/instrument/overlay";
import { PageHeader, Panel } from "@/components/instrument/panel";
import { ReasonList } from "@/components/instrument/reason-list";
import { RiskBadge } from "@/components/instrument/risk-badge";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/instrument/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/instrument/table";
import { TraceLine } from "@/components/instrument/trace-line";
import { isTyping, usePageShortcuts } from "@/components/shell/shortcuts";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import {
  useAlert,
  useAlerts,
  useSignalCategories,
  useTeam,
  useUserName,
  type AlertFilter,
} from "@/lib/queries";
import {
  BANDS,
  DECISION_LABEL,
  type Decision,
  type RiskBand,
} from "@/lib/risk";
import { useSession } from "@/lib/session";

type Alert = Schemas["Alert"];

const STATUS_TABS: { value: AlertFilter["status"] | "all"; label: string }[] = [
  { value: "open", label: "Open" },
  { value: "investigating", label: "Investigating" },
  { value: "escalated", label: "In a case" },
  { value: "resolved", label: "Resolved" },
  { value: "false_positive", label: "False positive" },
  { value: "all", label: "All" },
];

const STATUS_LABEL: Record<Alert["status"], string> = {
  open: "Open",
  investigating: "Investigating",
  escalated: "In a case",
  false_positive: "False positive",
  resolved: "Resolved",
};

const bandRank = (b: string) => BANDS.findIndex((x) => x.band === b);

/** Queue order (docs/design-system.md, AlertRow): most severe first, then oldest first. */
export function sortQueue(alerts: Alert[]): Alert[] {
  return [...alerts].sort(
    (a, b) =>
      bandRank(b.severity) - bandRank(a.severity) ||
      b.max_score - a.max_score ||
      a.last_seen_at.localeCompare(b.last_seen_at),
  );
}

export function AlertQueue() {
  const router = useRouter();
  const params = useSearchParams();
  const status = (params.get("status") ?? "open") as
    AlertFilter["status"] | "all";
  const kind = (params.get("kind") ?? undefined) as AlertFilter["kind"];
  const openId = params.get("alert") ?? undefined;
  const filter: AlertFilter = {
    status: status === "all" ? undefined : status,
    kind,
  };
  const q = useAlerts(filter);
  const userName = useUserName();
  const alerts = useMemo(
    () => sortQueue((q.data?.pages ?? []).flatMap((p) => p.data)),
    [q.data],
  );
  const [cursor, setCursor] = useState(0);
  const [caseFor, setCaseFor] = useState<Alert>();

  const setParam = useCallback(
    (name: string, value: string | undefined) => {
      const next = new URLSearchParams(params.toString());
      if (value === undefined) next.delete(name);
      else next.set(name, value);
      router.replace(`/alerts?${next.toString()}` as "/alerts", {
        scroll: false,
      });
    },
    [params, router],
  );

  const open = useCallback(
    (a: Alert | undefined) => setParam("alert", a?.id),
    [setParam],
  );
  const selected = alerts[cursor];
  const triage = useTriage();

  usePageShortcuts([
    { keys: "j", label: "Next alert" },
    { keys: "k", label: "Previous alert" },
    { keys: "Enter", label: "Open the alert" },
    { keys: "i", label: "Mark investigating" },
    { keys: "f", label: "Mark false positive" },
    { keys: "x", label: "Resolve" },
    { keys: "c", label: "Open a case with the alert" },
  ]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e) || e.metaKey || e.ctrlKey || e.altKey || caseFor) return;
      const target = openId ? alerts.find((a) => a.id === openId) : selected;
      switch (e.key) {
        case "j":
          setCursor((c) => Math.min(c + 1, Math.max(0, alerts.length - 1)));
          break;
        case "k":
          setCursor((c) => Math.max(c - 1, 0));
          break;
        case "Enter":
          if (!openId && selected) open(selected);
          break;
        case "i":
          if (target) void triage.run(target.id, { status: "investigating" });
          break;
        case "f":
          if (target) void triage.run(target.id, { status: "false_positive" });
          break;
        case "x":
          if (target) void triage.run(target.id, { status: "resolved" });
          break;
        case "c":
          if (target) setCaseFor(target);
          break;
        default:
          return;
      }
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [alerts, selected, openId, open, triage, caseFor]);

  return (
    <>
      <PageHeader
        title="Alerts"
        description="Risky decisions and degraded evaluations, grouped per subject within an hour. Most severe first."
      />
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div
          role="tablist"
          aria-label="Status"
          className="flex flex-wrap gap-1"
        >
          {STATUS_TABS.map((t) => (
            <button
              key={t.value}
              role="tab"
              aria-selected={status === t.value}
              onClick={() => {
                setCursor(0);
                setParam("status", t.value);
              }}
              className={`h-control-sm rounded-md px-3 text-body-sm ${status === t.value ? "bg-signal-wash text-ink" : "text-ink-2 hover:bg-bg-raised"}`}
            >
              {t.label}
            </button>
          ))}
        </div>
        <Select
          aria-label="Kind"
          className="w-44"
          value={kind ?? ""}
          onChange={(e) => setParam("kind", e.target.value || undefined)}
        >
          <option value="">All kinds</option>
          <option value="risk">Risk</option>
          <option value="security">Security</option>
        </Select>
      </div>
      {triage.error ? <Notice tone="error">{triage.error}</Notice> : null}
      <Panel className="overflow-hidden">
        <Table aria-label="Alert queue">
          <thead>
            <tr>
              <Th>Risk</Th>
              <Th>Alert</Th>
              <Th>Subject</Th>
              <Th className="text-right">Evaluations</Th>
              <Th>Assignee</Th>
              <Th>Status</Th>
              <Th>Last seen</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={7} /> : null}
            {alerts.map((a, i) => (
              <Tr
                key={a.id}
                selected={i === cursor || a.id === openId}
                onClick={() => {
                  setCursor(i);
                  open(a);
                }}
                className="cursor-pointer"
              >
                <Td>
                  <RiskBadge
                    band={a.severity as RiskBand}
                    score={a.max_score}
                  />
                </Td>
                <Td>
                  <span className="block text-body-sm">{a.title}</span>
                  <span className="mono text-legend text-ink-3">{a.id}</span>
                </Td>
                <DataCell>
                  {a.subject.type === "user" ? (
                    a.subject.ref
                  ) : (
                    <span className="text-ink-2">
                      {humanize(a.subject.type)}
                    </span>
                  )}
                </DataCell>
                <DataCell className="text-right">{a.evaluation_count}</DataCell>
                <Td className="text-body-sm text-ink-2">
                  {userName(a.assignee)}
                </Td>
                <Td>
                  <Tag>{STATUS_LABEL[a.status]}</Tag>
                </Td>
                <DataCell className="text-ink-3">
                  {formatAge(a.last_seen_at)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && alerts.length === 0 ? (
          <Empty title="No alerts here">
            Alerts appear when a decision reaches the alert threshold in risk
            settings.
          </Empty>
        ) : null}
        {q.hasNextPage ? (
          <div className="border-t border-line-subtle p-3">
            <Button
              size="sm"
              onClick={() => void q.fetchNextPage()}
              disabled={q.isFetchingNextPage}
            >
              Load more
            </Button>
          </div>
        ) : null}
      </Panel>
      <AlertDrawer
        id={openId}
        onClose={() => open(undefined)}
        onOpenCase={setCaseFor}
      />
      <NewCaseModal alert={caseFor} onClose={() => setCaseFor(undefined)} />
    </>
  );
}

function useTriage() {
  return useAction(
    async (id: string, body: Schemas["UpdateAlertRequest"]) =>
      unwrap(
        await api().PATCH("/api/v1/alerts/{alert_id}", {
          params: { path: { alert_id: id } },
          body,
        }),
      ),
    { invalidate: ["alerts", "alert"] },
  );
}

function AlertDrawer({
  id,
  onClose,
  onOpenCase,
}: {
  id: string | undefined;
  onClose: () => void;
  onOpenCase: (a: Alert) => void;
}) {
  const q = useAlert(id);
  const { session, can } = useSession();
  const team = useTeam();
  const userName = useUserName();
  const category = useSignalCategories();
  const triage = useTriage();
  const [note, setNote] = useState("");
  const d = q.data;
  const a = d?.alert;
  const open =
    a !== undefined && (a.status === "open" || a.status === "investigating");
  const latest = d?.evaluations[0];
  const scores = [...(d?.evaluations ?? [])].reverse().map((e) => e.risk_score);

  const act = async (body: Schemas["UpdateAlertRequest"]) => {
    if (!a) return;
    const res = await triage.run(
      a.id,
      note.trim() ? { ...body, note: note.trim() } : body,
    );
    if (res) setNote("");
  };

  return (
    <Drawer
      open={!!id}
      onOpenChange={(o) => !o && onClose()}
      title={a ? a.title : "Alert"}
      subtitle={a ? <span className="mono">{a.id}</span> : undefined}
      footer={
        a && can("alerts:write") ? (
          <>
            {open ? (
              <>
                {a.status === "open" ? (
                  <Button
                    size="sm"
                    shortcut="i"
                    onClick={() => void act({ status: "investigating" })}
                    disabled={triage.busy}
                  >
                    Investigate
                  </Button>
                ) : null}
                <Button
                  size="sm"
                  shortcut="f"
                  onClick={() => void act({ status: "false_positive" })}
                  disabled={triage.busy}
                >
                  False positive
                </Button>
                <Button
                  size="sm"
                  shortcut="x"
                  onClick={() => void act({ status: "resolved" })}
                  disabled={triage.busy}
                >
                  Resolve
                </Button>
                {can("cases:write") ? (
                  <Button
                    size="sm"
                    variant="primary"
                    shortcut="c"
                    onClick={() => onOpenCase(a)}
                  >
                    Open case
                  </Button>
                ) : null}
              </>
            ) : a.case_id ? (
              <Button size="sm" asChild>
                <Link href={`/cases/${a.case_id}` as "/cases"}>
                  Go to the case
                </Link>
              </Button>
            ) : (
              <span className="text-body-sm text-ink-3">
                This alert is closed.
              </span>
            )}
          </>
        ) : null
      }
    >
      {q.isLoading ? <Loading /> : null}
      {q.isError ? <ErrorState error={q.error} /> : null}
      {a && d ? (
        <div className="flex flex-col gap-6">
          <div className="flex flex-wrap items-center gap-2">
            <RiskBadge band={a.severity as RiskBand} score={a.max_score} />
            <Tag>{STATUS_LABEL[a.status]}</Tag>
            <Tag>{a.kind === "risk" ? "Risk" : "Security"}</Tag>
          </div>
          {triage.error ? <Notice tone="error">{triage.error}</Notice> : null}
          <KeyValues
            items={[
              [
                "Subject",
                <span key="s" className="mono">
                  {a.subject.type === "user"
                    ? a.subject.ref
                    : humanize(a.subject.ref)}
                </span>,
              ],
              [
                "Evaluations",
                <span key="e" className="mono">
                  {a.evaluation_count}
                </span>,
              ],
              [
                "First seen",
                <span key="f" className="mono">
                  {formatTimestamp(a.first_seen_at)}
                </span>,
              ],
              [
                "Last seen",
                <span key="l" className="mono">
                  {formatTimestamp(a.last_seen_at)}
                </span>,
              ],
              ["Assignee", userName(a.assignee)],
              ...(a.case_id
                ? ([
                    [
                      "Case",
                      <Link
                        key="c"
                        className="mono text-signal-ink"
                        href={`/cases/${a.case_id}` as "/cases"}
                      >
                        {a.case_id}
                      </Link>,
                    ],
                  ] as [string, React.ReactNode][])
                : []),
            ]}
          />
          {open && can("alerts:write") ? (
            <div className="flex flex-col gap-3">
              <Field label="Assignee">
                {(p) => (
                  <Select
                    {...p}
                    value={a.assignee ?? ""}
                    onChange={(e) => void act({ assignee: e.target.value })}
                    disabled={triage.busy}
                  >
                    <option value="">Unassigned</option>
                    <option value={session.user.id}>
                      {session.user.name} (you)
                    </option>
                    {(team.data?.data ?? [])
                      .filter(
                        (m) =>
                          m.id !== session.user.id && m.status === "active",
                      )
                      .map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.name}
                        </option>
                      ))}
                  </Select>
                )}
              </Field>
              <Field
                label="Note for the timeline"
                hint="Added with your next action."
              >
                {(p) => (
                  <Input
                    {...p}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                    maxLength={2000}
                  />
                )}
              </Field>
            </div>
          ) : null}
          {scores.length > 1 ? (
            <div>
              <p className="legend mb-1">Score per evaluation</p>
              <TraceLine
                points={scores}
                width={420}
                height={40}
                label={`Scores of ${scores.length} evaluations`}
              />
            </div>
          ) : null}
          {latest ? (
            <ReasonList
              title="Why the latest score"
              reasons={latest.signals.map((s) => ({
                label: s.label,
                category: category(s.code),
                contribution: s.weight,
              }))}
            />
          ) : null}
          {d.evaluations.length ? (
            <section>
              <h3 className="mb-2 text-heading">Evaluations</h3>
              <ul>
                {d.evaluations.map((e) => (
                  <li
                    key={e.evaluation_id}
                    className="flex items-center justify-between gap-2 border-t border-line-subtle py-2"
                  >
                    <span className="flex items-center gap-2">
                      <RiskBadge
                        band={e.risk_level as RiskBand}
                        score={e.risk_score}
                      />
                      <span className="text-body-sm">
                        {DECISION_LABEL[e.decision as Decision]}
                      </span>
                    </span>
                    <Link
                      href={`/events?event=${e.event_id}` as "/events"}
                      className="mono text-data text-signal-ink"
                    >
                      {e.event_id}
                    </Link>
                    <span className="mono text-data text-ink-3">
                      {formatAge(e.evaluated_at)}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
          <Timeline entries={d.activity} />
        </div>
      ) : null}
    </Drawer>
  );
}

export function Timeline({ entries }: { entries: Schemas["ActivityEntry"][] }) {
  const userName = useUserName();
  return (
    <section>
      <h3 className="mb-2 text-heading">Timeline</h3>
      <ol>
        {entries.map((e) => (
          <li
            key={e.id}
            className="grid grid-cols-[1fr_auto] gap-2 border-t border-line-subtle py-2"
          >
            <div>
              <p className="text-body-sm">{describeActivity(e)}</p>
              <p className="legend">
                {e.actor.startsWith("usr_") ? userName(e.actor) : e.actor}
              </p>
            </div>
            <span
              className="mono text-data text-ink-3"
              title={formatTimestamp(e.created_at)}
            >
              {formatAge(e.created_at)}
            </span>
          </li>
        ))}
      </ol>
    </section>
  );
}

function describeActivity(e: Schemas["ActivityEntry"]): string {
  const d = e.detail as Record<string, unknown>;
  const parts: string[] = [humanize(e.action)];
  if (typeof d.status_after === "string")
    parts.push(`→ ${humanize(d.status_after)}`);
  if (typeof d.note === "string") parts.push(`“${d.note}”`);
  if (typeof d.risk_score === "number") parts.push(`score ${d.risk_score}`);
  return parts.join(" · ");
}

export function NewCaseModal({
  alert,
  onClose,
}: {
  alert: Alert | undefined;
  onClose: () => void;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const create = useAction(
    async (body: Schemas["CreateCaseRequest"]) =>
      unwrap(await api().POST("/api/v1/cases", { body })),
    {
      invalidate: ["alerts", "alert", "cases"],
      onSuccess: (c) => {
        onClose();
        void qc.invalidateQueries();
        router.push(`/cases/${c.id}` as "/cases");
      },
    },
  );
  return (
    <Modal
      open={!!alert}
      onOpenChange={(o) => !o && onClose()}
      title="Open a case"
      description="The alert moves into the case; resolving the case closes it."
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void create.run({
            title: String(f.get("title")),
            alert_ids: alert ? [alert.id] : undefined,
          });
        }}
        className="flex flex-col gap-4"
      >
        {create.error ? <Notice tone="error">{create.error}</Notice> : null}
        <Field label="Title">
          {(p) => (
            <Input
              {...p}
              name="title"
              required
              maxLength={200}
              defaultValue={alert ? `${alert.title}: ${alert.subject.ref}` : ""}
              autoFocus
            />
          )}
        </Field>
        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={create.busy}>
            Open case
          </Button>
        </div>
      </form>
    </Modal>
  );
}
