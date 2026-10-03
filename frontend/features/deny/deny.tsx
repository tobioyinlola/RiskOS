"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Tag } from "@/components/ui/misc";
import { Confirm, Modal } from "@/components/ui/overlay";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatCount, formatTimestamp } from "@/lib/format";
import { useDenyList, useUserName } from "@/lib/queries";
import { useSession } from "@/lib/session";

import { EntityLink } from "../entities/link";
import { Ban } from "@/components/ui/icons";

type Kind = Schemas["DenyKind"];
type Entry = Schemas["DenyEntry"];

export const DENY_KIND_LABEL: Record<Kind, string> = {
  user: "User",
  account: "Account",
  device: "Device",
  ip: "IP address",
  beneficiary: "Beneficiary",
  country: "Country",
};

const PLACEHOLDER: Record<Kind, string> = {
  user: "usr_4812",
  account: "acc_77",
  device: "dev_9a1",
  ip: "203.0.113.7",
  beneficiary: "ben_552",
  country: "Two letters, e.g. KP",
};

const EXPIRY: { value: string; label: string; hours?: number }[] = [
  { value: "never", label: "Never expires" },
  { value: "24h", label: "24 hours", hours: 24 },
  { value: "7d", label: "7 days", hours: 24 * 7 },
  { value: "30d", label: "30 days", hours: 24 * 30 },
];

const INVALIDATE = ["deny-list"];

/** The form to add an identifier; used by the page and by entity profiles. */
export function AddDenyForm({
  kind: initialKind = "user",
  value: initialValue = "",
  lockKind = false,
  onDone,
}: {
  kind?: Kind;
  value?: string;
  lockKind?: boolean;
  onDone?: (e: Entry) => void;
}) {
  const [kind, setKind] = useState<Kind>(initialKind);
  const [value, setValue] = useState(initialValue);
  const [reason, setReason] = useState("");
  const [expiry, setExpiry] = useState("never");
  const add = useAction(
    async () => {
      const hours = EXPIRY.find((e) => e.value === expiry)?.hours;
      return unwrap(
        await api().POST("/api/v1/deny-list", {
          body: {
            kind,
            value: value.trim(),
            ...(reason.trim() ? { reason: reason.trim() } : {}),
            ...(hours
              ? {
                  expires_at: new Date(
                    Date.now() + hours * 3_600_000,
                  ).toISOString(),
                }
              : {}),
          },
        }),
      );
    },
    {
      invalidate: INVALIDATE,
      onSuccess: (e) => {
        setValue("");
        setReason("");
        onDone?.(e);
      },
    },
  );
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (value.trim()) void add.run();
  };
  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3">
      {add.error ? <Notice tone="error">{add.error}</Notice> : null}
      <div className="flex flex-wrap items-end gap-3">
        <Field label="Kind" className="w-44">
          {(p) => (
            <Select
              {...p}
              value={kind}
              disabled={lockKind}
              onChange={(e) => setKind(e.target.value as Kind)}
            >
              {(Object.keys(DENY_KIND_LABEL) as Kind[]).map((k) => (
                <option key={k} value={k}>
                  {DENY_KIND_LABEL[k]}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <Field label="Identifier" className="min-w-56 flex-1">
          {(p) => (
            <Input
              {...p}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder={PLACEHOLDER[kind]}
              className="mono"
              maxLength={128}
              required
            />
          )}
        </Field>
        <Field label="Expires" className="w-40">
          {(p) => (
            <Select
              {...p}
              value={expiry}
              onChange={(e) => setExpiry(e.target.value)}
            >
              {EXPIRY.map((e) => (
                <option key={e.value} value={e.value}>
                  {e.label}
                </option>
              ))}
            </Select>
          )}
        </Field>
      </div>
      <Field
        label="Reason (optional)"
        hint="For your team: why this identifier is blocked."
      >
        {(p) => (
          <Input
            {...p}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            maxLength={200}
          />
        )}
      </Field>
      <Button
        type="submit"
        variant="primary"
        className="self-start"
        disabled={add.busy || !value.trim()}
        icon={<Ban />}
      >
        Add to deny list
      </Button>
    </form>
  );
}

/** The deny list: identifiers whose events are blocked whatever their score. */
export function DenyList() {
  const { can } = useSession();
  const [kind, setKind] = useState<Kind | "">("");
  const q = useDenyList(kind || undefined);
  const userName = useUserName();
  const [removing, setRemoving] = useState<Entry>();
  const writable = can("risk:configure");
  const remove = useAction(
    async (id: string) =>
      unwrap(
        await api().DELETE("/api/v1/deny-list/{entry_id}", {
          params: { path: { entry_id: id } },
        }),
      ),
    { invalidate: INVALIDATE, onSuccess: () => setRemoving(undefined) },
  );
  const now = q.dataUpdatedAt;
  const entries = q.data?.data ?? [];
  return (
    <>
      <PageHeader
        title="Deny lists"
        description="Identifiers you never want to transact with. An event whose user, account, device, IP address, beneficiary or country is listed is blocked, whatever its score."
      />
      {writable ? (
        <Panel className="mb-5">
          <PanelHeader
            title="Add an identifier"
            legend="takes effect within seconds"
          />
          <div className="p-4">
            <AddDenyForm />
          </div>
        </Panel>
      ) : null}
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <Select
          aria-label="Kind"
          className="h-control-sm w-44 text-body-sm"
          value={kind}
          onChange={(e) => setKind(e.target.value as Kind | "")}
        >
          <option value="">All kinds</option>
          {(Object.keys(DENY_KIND_LABEL) as Kind[]).map((k) => (
            <option key={k} value={k}>
              {DENY_KIND_LABEL[k]}
            </option>
          ))}
        </Select>
        {q.data ? (
          <span className="text-label text-text-muted">
            {formatCount(q.data.total)} of {formatCount(q.data.limit)} entries
            used
          </span>
        ) : null}
      </div>
      {remove.error ? <Notice tone="error">{remove.error}</Notice> : null}
      <Panel className="overflow-hidden">
        <Table aria-label="Deny list">
          <thead>
            <tr>
              <Th>Kind</Th>
              <Th>Identifier</Th>
              <Th>Reason</Th>
              <Th>Added</Th>
              <Th>Expires</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {entries.map((e) => {
              const expired =
                e.expires_at !== null &&
                new Date(e.expires_at).getTime() <= now;
              const entityKind =
                e.kind === "user" || e.kind === "account" || e.kind === "device"
                  ? e.kind
                  : null;
              return (
                <Tr key={e.id}>
                  <Td>
                    <Tag>{DENY_KIND_LABEL[e.kind]}</Tag>
                  </Td>
                  <Td>
                    {entityKind ? (
                      <EntityLink kind={entityKind} refId={e.value} />
                    ) : (
                      <span className="mono text-data">{e.value}</span>
                    )}
                  </Td>
                  <Td
                    className="max-w-80 truncate text-body-sm text-text-secondary"
                    title={e.reason}
                  >
                    {e.reason || "—"}
                  </Td>
                  <Td className="text-body-sm text-text-secondary">
                    <span title={formatTimestamp(e.created_at)}>
                      {formatAge(e.created_at)} ago
                    </span>
                    <span className="block text-label text-text-muted">
                      {userName(e.created_by)}
                    </span>
                  </Td>
                  <DataCell className={expired ? "text-text-muted" : ""}>
                    {e.expires_at
                      ? expired
                        ? "Expired"
                        : formatTimestamp(e.expires_at)
                      : "Never"}
                  </DataCell>
                  <Td className="text-right">
                    {writable ? (
                      <Button
                        size="sm"
                        variant="danger"
                        onClick={() => setRemoving(e)}
                      >
                        Remove
                      </Button>
                    ) : null}
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && entries.length === 0 ? (
          <Empty title="Nothing on the deny list">
            Add identifiers you have confirmed as fraudulent, such as a mule
            account&apos;s beneficiary ID or an emulator&apos;s device ID.
          </Empty>
        ) : null}
      </Panel>
      <Confirm
        open={!!removing}
        onOpenChange={(o) => (o ? undefined : setRemoving(undefined))}
        title={`Remove ${removing ? DENY_KIND_LABEL[removing.kind].toLowerCase() : ""} ${removing?.value ?? ""}?`}
        description="Events carrying it will be scored and decided normally again, within seconds."
        confirmLabel="Remove from deny list"
        busy={remove.busy}
        onConfirm={async () => {
          if (!removing) return;
          await remove.run(removing.id);
        }}
      />
    </>
  );
}

/** The entity profile's action: add this user, account or device to the deny list. */
export function DenyEntityButton({
  kind,
  refId,
}: {
  kind: "user" | "account" | "device";
  refId: string;
}) {
  const { can } = useSession();
  const [open, setOpen] = useState(false);
  const [added, setAdded] = useState(false);
  if (!can("risk:configure")) return null;
  return (
    <>
      <Button
        variant="danger"
        icon={<Ban />}
        onClick={() => setOpen(true)}
        disabled={added}
      >
        {added ? "On the deny list" : "Add to deny list"}
      </Button>
      <Modal
        open={open}
        onOpenChange={setOpen}
        title={`Block ${kind} ${refId}?`}
        description="Every event carrying this identifier will be blocked, whatever its score, within seconds."
      >
        <AddDenyForm
          kind={kind}
          value={refId}
          lockKind
          onDone={() => {
            setAdded(true);
            setOpen(false);
          }}
        />
      </Modal>
    </>
  );
}
