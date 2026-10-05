"use client";

import { useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { KeyValues, Tag } from "@/components/ui/misc";
import { Drawer } from "@/components/ui/overlay";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { Notice } from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import {
  useRule,
  useRuleChange,
  useRuleChanges,
  useUserName,
  useCatalog,
} from "@/lib/queries";
import { ruleSentence } from "@/lib/rule-builder";
import { useSession } from "@/lib/session";

type Rule = Schemas["Rule"];
export type RuleChange = Schemas["RuleChange"];

/** A save in production returns the proposed change (202) instead of the rule. */
export function isChange(x: Rule | RuleChange): x is RuleChange {
  return "proposed" in x;
}

const INVALIDATE = ["rules", "rule", "rule-versions", "rule-changes"];

const KIND_LABEL: Record<RuleChange["kind"], string> = {
  create: "New rule",
  update: "Change",
  rollback: "Rollback",
};

/** Pending proposals, above the rules table. Renders nothing when there are none. */
export function PendingChanges() {
  const q = useRuleChanges("pending");
  const userName = useUserName();
  const [open, setOpen] = useState<string | null>(null);
  const changes = q.data?.data ?? [];
  if (changes.length === 0) return null;
  return (
    <Panel className="mb-4 overflow-hidden">
      <PanelHeader
        title="Awaiting approval"
        legend={`${changes.length} pending`}
      />
      <Table aria-label="Rule changes awaiting approval">
        <thead>
          <tr>
            <Th>Change</Th>
            <Th>Rule</Th>
            <Th>Proposed by</Th>
            <Th>When</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {changes.map((c) => (
            <Tr key={c.id}>
              <Td>
                <Tag>{KIND_LABEL[c.kind]}</Tag>
              </Td>
              <Td>
                <span className="block text-body-sm">{c.proposed.name}</span>
                <span className="mono text-label text-text-muted">
                  {c.code}
                </span>
              </Td>
              <Td className="text-body-sm text-text-secondary">
                {userName(c.proposed_by)}
              </Td>
              <DataCell
                className="text-text-muted"
                title={formatTimestamp(c.proposed_at)}
              >
                {formatAge(c.proposed_at)}
              </DataCell>
              <Td className="text-right">
                <Button size="sm" onClick={() => setOpen(c.id)}>
                  Review
                </Button>
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      <ChangeDrawer id={open} onClose={() => setOpen(null)} />
    </Panel>
  );
}

/** The pending change of one rule, on its page. */
export function PendingChangeNotice({ rule }: { rule: Rule }) {
  const [open, setOpen] = useState(false);
  if (!rule.pending_change_id) return null;
  return (
    <div className="mb-4">
      <Notice tone="warn">
        <span>
          A change to this rule is awaiting approval. The rule keeps its current
          version until another person approves it; edits are locked until
          then.{" "}
        </span>
        <button
          type="button"
          className="text-accent underline-offset-2 hover:underline"
          onClick={() => setOpen(true)}
        >
          Review the change
        </button>
      </Notice>
      <ChangeDrawer
        id={open ? rule.pending_change_id : null}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}

function ChangeDrawer({
  id,
  onClose,
}: {
  id: string | null;
  onClose: () => void;
}) {
  const q = useRuleChange(id);
  const c = q.data;
  return (
    <Drawer
      open={!!id}
      onOpenChange={(o) => (o ? undefined : onClose())}
      title={c ? `${KIND_LABEL[c.kind]}: ${c.proposed.name}` : "Rule change"}
      subtitle={c ? <span className="mono">{c.code}</span> : null}
    >
      {c ? <ChangeReview change={c} onDecided={onClose} /> : null}
    </Drawer>
  );
}

function ChangeReview({
  change: c,
  onDecided,
}: {
  change: RuleChange;
  onDecided: () => void;
}) {
  const catalog = useCatalog();
  const { session, can } = useSession();
  const userName = useUserName();
  const current = useRule(c.kind === "create" ? undefined : c.rule_id);
  const [reason, setReason] = useState("");
  const mine = c.proposed_by === session.user.id;
  const canApprove = can("rules:approve") && !mine;
  const canReject = can("rules:approve") || mine;

  const approve = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/rule-changes/{change_id}/approve", {
          params: { path: { change_id: c.id } },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: onDecided },
  );
  const reject = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/rule-changes/{change_id}/reject", {
          params: { path: { change_id: c.id } },
          body: reason.trim() ? { reason: reason.trim() } : {},
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: onDecided },
  );

  const now = current.data;
  const p = c.proposed;
  const rows: [string, ReactNode, ReactNode][] = [
    ["Name", now?.name, p.name],
    ["Description", now?.description || "—", p.description || "—"],
    ["Severity", now && humanize(now.severity), humanize(p.severity)],
    ["Weight", now && `+${now.weight}`, `+${p.weight}`],
    ["Mode", now && humanize(now.mode), humanize(p.mode)],
    ["Enabled", now && (now.enabled ? "Yes" : "No"), p.enabled ? "Yes" : "No"],
  ];
  const conditionChanged =
    !now || JSON.stringify(now.condition) !== JSON.stringify(p.condition);
  const fields = catalog.data?.fields ?? [];

  return (
    <div className="flex flex-col gap-5">
      <KeyValues
        items={[
          ["Proposed by", userName(c.proposed_by)],
          ["When", formatTimestamp(c.proposed_at)],
          [
            "Applies to",
            c.kind === "create"
              ? "A new rule"
              : `v${c.base_version} (becomes v${(c.base_version ?? 0) + 1})`,
          ],
          ...(p.restored_from
            ? ([["Restores", `v${p.restored_from}`]] as [string, string][])
            : []),
        ]}
      />

      <Table aria-label="Proposed content">
        <thead>
          <tr>
            <Th>Field</Th>
            {c.kind !== "create" ? <Th>Now</Th> : null}
            <Th>Proposed</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([label, before, after]) => {
            const changed =
              c.kind !== "create" && String(before) !== String(after);
            return (
              <Tr key={label} selected={changed}>
                <Td className="text-label text-text-muted">{label}</Td>
                {c.kind !== "create" ? (
                  <Td className="text-body-sm text-text-secondary">{before}</Td>
                ) : null}
                <Td className="text-body-sm">{after}</Td>
              </Tr>
            );
          })}
        </tbody>
      </Table>

      <div className="flex flex-col gap-2">
        <p className="text-label text-text-muted">
          What it says
          {conditionChanged && c.kind !== "create" ? " (changed)" : ""}
        </p>
        <p className="text-body">{ruleSentence(p, fields)}</p>
      </div>

      {approve.error ? <Notice tone="error">{approve.error}</Notice> : null}
      {reject.error ? <Notice tone="error">{reject.error}</Notice> : null}
      {mine ? (
        <Notice>
          You proposed this change. Another person with approval rights must
          approve it; you can withdraw it.
        </Notice>
      ) : !can("rules:approve") ? (
        <Notice>
          Approving rule changes needs the risk manager or administrator role.
        </Notice>
      ) : null}

      {canReject ? (
        <Field label="Reason (optional)">
          {(f) => (
            <Input
              {...f}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={500}
            />
          )}
        </Field>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {canApprove ? (
          <Button
            variant="primary"
            onClick={() => void approve.run()}
            disabled={approve.busy || reject.busy}
          >
            Approve and apply
          </Button>
        ) : null}
        {canReject ? (
          <Button
            onClick={() => void reject.run()}
            disabled={approve.busy || reject.busy}
          >
            {mine ? "Withdraw" : "Reject"}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
