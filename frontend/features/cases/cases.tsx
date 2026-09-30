"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input, Select, Textarea } from "@/components/instrument/field";
import { KeyValues, Tag } from "@/components/instrument/misc";
import { Modal } from "@/components/instrument/overlay";
import {
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/instrument/panel";
import { RiskBadge } from "@/components/instrument/risk-badge";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/instrument/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/instrument/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import { useCase, useCases, useTeam, useUserName } from "@/lib/queries";
import { BANDS, type RiskBand } from "@/lib/risk";
import { useSession } from "@/lib/session";

import { Timeline } from "../alerts/alerts";

const STATUS_LABEL: Record<Schemas["CaseStatus"], string> = {
  open: "Open",
  investigating: "Investigating",
  resolved: "Resolved",
};

const RESOLUTIONS: {
  value: Schemas["CaseResolution"];
  label: string;
  help: string;
}[] = [
  {
    value: "confirmed_fraud",
    label: "Confirmed fraud",
    help: "The activity was fraudulent.",
  },
  {
    value: "false_positive",
    label: "False positive",
    help: "The alerts were wrong; they close as false positives.",
  },
  {
    value: "legitimate",
    label: "Legitimate",
    help: "Unusual but genuine activity.",
  },
  {
    value: "inconclusive",
    label: "Inconclusive",
    help: "Not enough evidence either way.",
  },
];

export function CaseList() {
  const [status, setStatus] = useState<Schemas["CaseStatus"] | "all">("open");
  const q = useCases(status === "all" ? undefined : status);
  const { can } = useSession();
  const userName = useUserName();
  const [creating, setCreating] = useState(false);
  const cases = useMemo(
    () => (q.data?.pages ?? []).flatMap((p) => p.data),
    [q.data],
  );
  return (
    <>
      <PageHeader
        title="Cases"
        description="Investigations: alerts, notes, assignment and a resolution."
        actions={
          can("cases:write") ? (
            <Button variant="primary" onClick={() => setCreating(true)}>
              Open case
            </Button>
          ) : null
        }
      />
      <div role="tablist" aria-label="Status" className="mb-3 flex gap-1">
        {(["open", "investigating", "resolved", "all"] as const).map((s) => (
          <button
            key={s}
            role="tab"
            aria-selected={status === s}
            onClick={() => setStatus(s)}
            className={`h-control-sm rounded-md px-3 text-body-sm ${status === s ? "bg-signal-wash text-ink" : "text-ink-2 hover:bg-bg-raised"}`}
          >
            {s === "all" ? "All" : STATUS_LABEL[s]}
          </button>
        ))}
      </div>
      <Panel className="overflow-hidden">
        <Table aria-label="Cases">
          <thead>
            <tr>
              <Th>Priority</Th>
              <Th>Case</Th>
              <Th>Status</Th>
              <Th>Assignee</Th>
              <Th>Opened</Th>
              <Th>Updated</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {cases.map((c) => (
              <Tr key={c.id}>
                <Td>
                  <RiskBadge band={c.priority as RiskBand} showScore={false} />
                </Td>
                <Td>
                  <Link
                    href={`/cases/${c.id}` as "/cases"}
                    className="block text-body-sm hover:text-signal-ink"
                  >
                    {c.title}
                  </Link>
                  <span className="mono text-legend text-ink-3">{c.id}</span>
                </Td>
                <Td>
                  <Tag>{STATUS_LABEL[c.status]}</Tag>
                </Td>
                <Td className="text-body-sm text-ink-2">
                  {userName(c.assignee)}
                </Td>
                <DataCell className="text-ink-3">
                  {formatAge(c.created_at)}
                </DataCell>
                <DataCell className="text-ink-3">
                  {formatAge(c.updated_at)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && cases.length === 0 ? (
          <Empty title="No cases here">
            Open one from an alert, or start one here.
          </Empty>
        ) : null}
        {q.hasNextPage ? (
          <div className="border-t border-line-subtle p-3">
            <Button size="sm" onClick={() => void q.fetchNextPage()}>
              Load more
            </Button>
          </div>
        ) : null}
      </Panel>
      <CreateCase open={creating} onClose={() => setCreating(false)} />
    </>
  );
}

function CreateCase({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const create = useAction(
    async (body: Schemas["CreateCaseRequest"]) =>
      unwrap(await api().POST("/api/v1/cases", { body })),
    {
      invalidate: ["cases"],
      onSuccess: (c) => {
        onClose();
        router.push(`/cases/${c.id}` as "/cases");
      },
    },
  );
  return (
    <Modal
      open={open}
      onOpenChange={(o) => !o && onClose()}
      title="Open a case"
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          const f = new FormData(e.currentTarget);
          void create.run({
            title: String(f.get("title")),
            description: String(f.get("description") ?? "") || undefined,
            priority: (String(f.get("priority")) || undefined) as
              Schemas["RiskLevel"] | undefined,
          });
        }}
      >
        {create.error ? <Notice tone="error">{create.error}</Notice> : null}
        <Field label="Title">
          {(p) => (
            <Input {...p} name="title" required maxLength={200} autoFocus />
          )}
        </Field>
        <Field label="Description">
          {(p) => <Textarea {...p} name="description" maxLength={5000} />}
        </Field>
        <Field label="Priority">
          {(p) => (
            <Select {...p} name="priority" defaultValue="elevated">
              {BANDS.map((b) => (
                <option key={b.band} value={b.band}>
                  {b.label}
                </option>
              ))}
            </Select>
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

export function CaseWorkspace({ id }: { id: string }) {
  const q = useCase(id);
  const { can, session } = useSession();
  const team = useTeam();
  const userName = useUserName();
  const [resolving, setResolving] = useState(false);
  const [note, setNote] = useState("");
  const update = useAction(
    async (body: Schemas["UpdateCaseRequest"]) =>
      unwrap(
        await api().PATCH("/api/v1/cases/{case_id}", {
          params: { path: { case_id: id } },
          body,
        }),
      ),
    { invalidate: ["case", "cases", "alerts", "alert"] },
  );
  const addNote = useAction(
    async (body: string) =>
      unwrap(
        await api().POST("/api/v1/cases/{case_id}/notes", {
          params: { path: { case_id: id } },
          body: { body },
        }),
      ),
    { invalidate: ["case"], onSuccess: () => setNote("") },
  );
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  const d = q.data!;
  const c = d.case;
  const writable = can("cases:write") && c.status !== "resolved";
  return (
    <>
      <PageHeader
        title={c.title}
        description={<span className="mono text-data text-ink-3">{c.id}</span>}
        actions={
          writable ? (
            <>
              {c.status === "open" ? (
                <Button
                  onClick={() => void update.run({ status: "investigating" })}
                  disabled={update.busy}
                >
                  Start investigating
                </Button>
              ) : null}
              <Button variant="primary" onClick={() => setResolving(true)}>
                Resolve case
              </Button>
            </>
          ) : null
        }
      />
      {update.error ? <Notice tone="error">{update.error}</Notice> : null}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_360px]">
        <div className="flex flex-col gap-4">
          {c.description ? (
            <Panel>
              <PanelBody className="text-body whitespace-pre-wrap">
                {c.description}
              </PanelBody>
            </Panel>
          ) : null}
          <Panel className="overflow-hidden">
            <PanelHeader title="Alerts" legend={`${d.alerts.length}`} />
            {d.alerts.length ? (
              <Table>
                <thead>
                  <tr>
                    <Th>Risk</Th>
                    <Th>Alert</Th>
                    <Th>Subject</Th>
                    <Th>Status</Th>
                  </tr>
                </thead>
                <tbody>
                  {d.alerts.map((a) => (
                    <Tr key={a.id}>
                      <Td>
                        <RiskBadge
                          band={a.severity as RiskBand}
                          score={a.max_score}
                        />
                      </Td>
                      <Td>
                        <Link
                          href={`/alerts?status=all&alert=${a.id}` as "/alerts"}
                          className="text-body-sm hover:text-signal-ink"
                        >
                          {a.title}
                        </Link>
                      </Td>
                      <DataCell>{a.subject.ref}</DataCell>
                      <Td>
                        <Tag>{humanize(a.status)}</Tag>
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </Table>
            ) : (
              <Empty title="No alerts in this case" />
            )}
          </Panel>
          <Panel>
            <PanelHeader title="Notes" legend="append-only" />
            <PanelBody className="flex flex-col gap-3">
              {d.notes.map((n) => (
                <article
                  key={n.id}
                  className="border-l-2 border-line-control pl-3"
                >
                  <p className="text-body whitespace-pre-wrap">{n.body}</p>
                  <p className="legend mt-1">
                    {userName(n.author)} · {formatTimestamp(n.created_at)}
                  </p>
                </article>
              ))}
              {d.notes.length === 0 ? (
                <p className="text-body-sm text-ink-3">No notes yet.</p>
              ) : null}
              {can("cases:write") ? (
                <form
                  className="flex flex-col gap-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (note.trim()) void addNote.run(note);
                  }}
                >
                  {addNote.error ? (
                    <Notice tone="error">{addNote.error}</Notice>
                  ) : null}
                  <Field
                    label="Add a note"
                    hint="Notes cannot be edited or deleted. Never paste card numbers or passwords."
                  >
                    {(p) => (
                      <Textarea
                        {...p}
                        value={note}
                        onChange={(e) => setNote(e.target.value)}
                        maxLength={10000}
                      />
                    )}
                  </Field>
                  <Button
                    type="submit"
                    size="sm"
                    className="self-start"
                    disabled={addNote.busy || !note.trim()}
                  >
                    Add note
                  </Button>
                </form>
              ) : null}
            </PanelBody>
          </Panel>
        </div>
        <div className="flex flex-col gap-4">
          <Panel>
            <PanelBody className="flex flex-col gap-4">
              <div className="flex items-center gap-2">
                <RiskBadge band={c.priority as RiskBand} showScore={false} />
                <Tag>{STATUS_LABEL[c.status]}</Tag>
              </div>
              <KeyValues
                items={[
                  [
                    "Opened",
                    <span key="o" className="mono">
                      {formatTimestamp(c.created_at)}
                    </span>,
                  ],
                  ["Opened by", userName(c.created_by)],
                  ...(c.resolution
                    ? ([
                        ["Resolution", humanize(c.resolution)],
                        [
                          "Resolved",
                          <span key="r" className="mono">
                            {c.resolved_at
                              ? formatTimestamp(c.resolved_at)
                              : ""}
                          </span>,
                        ],
                      ] as [string, React.ReactNode][])
                    : []),
                ]}
              />
              {c.resolution_note ? (
                <p className="text-body-sm whitespace-pre-wrap text-ink-2">
                  {c.resolution_note}
                </p>
              ) : null}
              {writable ? (
                <>
                  <Field label="Assignee">
                    {(p) => (
                      <Select
                        {...p}
                        value={c.assignee ?? ""}
                        onChange={(e) =>
                          void update.run({ assignee: e.target.value })
                        }
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
                  <Field label="Priority">
                    {(p) => (
                      <Select
                        {...p}
                        value={c.priority}
                        onChange={(e) =>
                          void update.run({
                            priority: e.target.value as Schemas["RiskLevel"],
                          })
                        }
                      >
                        {BANDS.map((b) => (
                          <option key={b.band} value={b.band}>
                            {b.label}
                          </option>
                        ))}
                      </Select>
                    )}
                  </Field>
                </>
              ) : (
                <KeyValues items={[["Assignee", userName(c.assignee)]]} />
              )}
            </PanelBody>
          </Panel>
          <Panel>
            <PanelBody>
              <Timeline entries={d.activity} />
            </PanelBody>
          </Panel>
        </div>
      </div>
      <Modal
        open={resolving}
        onOpenChange={setResolving}
        title="Resolve case"
        description="Resolving closes the case and its alerts. It cannot be reopened; notes can still be added."
      >
        <form
          className="flex flex-col gap-4"
          onSubmit={async (e) => {
            e.preventDefault();
            const f = new FormData(e.currentTarget);
            const res = await update.run({
              status: "resolved",
              resolution: String(
                f.get("resolution"),
              ) as Schemas["CaseResolution"],
              resolution_note: String(f.get("note") ?? "") || undefined,
            });
            if (res) setResolving(false);
          }}
        >
          {update.error ? <Notice tone="error">{update.error}</Notice> : null}
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-label text-ink-2">Resolution</legend>
            {RESOLUTIONS.map((r, i) => (
              <label key={r.value} className="flex gap-2 text-body">
                <input
                  type="radio"
                  name="resolution"
                  value={r.value}
                  defaultChecked={i === 0}
                  className="mt-1 accent-[var(--signal-ink)]"
                />
                <span>
                  {r.label}
                  <span className="block text-body-sm text-ink-3">
                    {r.help}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
          <Field label="Resolution note">
            {(p) => <Textarea {...p} name="note" maxLength={5000} />}
          </Field>
          <div className="flex justify-end gap-2">
            <Button onClick={() => setResolving(false)}>Cancel</Button>
            <Button type="submit" variant="primary" disabled={update.busy}>
              Resolve
            </Button>
          </div>
        </form>
      </Modal>
    </>
  );
}
