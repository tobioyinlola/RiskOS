"use client";

import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { CodeBlock, KeyValues, Status, Tag } from "@/components/ui/misc";
import { Confirm, Drawer, Modal } from "@/components/ui/overlay";
import {
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/ui/panel";
import {
  Empty,
  ErrorState,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatTimestamp, humanize } from "@/lib/format";
import { useAuditLogs, useInvites, useTeam, useUserName } from "@/lib/queries";
import { useSession } from "@/lib/session";
import { createPasskey, passkeysSupported } from "@/lib/webauthn";
import { useReauth } from "@/components/shell/reauth";

import { Enrollment, RecoveryCodes } from "../auth/mfa";

const ROLES: { value: Schemas["Role"]; label: string; help: string }[] = [
  {
    value: "customer_admin",
    label: "Administrator",
    help: "Everything, including team and keys.",
  },
  {
    value: "risk_manager",
    label: "Risk manager",
    help: "Rules, risk settings, alerts and cases.",
  },
  { value: "analyst", label: "Analyst", help: "Events, alerts and cases." },
  {
    value: "developer",
    label: "Developer",
    help: "API keys, webhooks and events.",
  },
];
const roleLabel = (r: string) =>
  ROLES.find((x) => x.value === r)?.label ?? humanize(r);

export function AuditLog() {
  const q = useAuditLogs();
  const userName = useUserName();
  const [open, setOpen] = useState<Schemas["AuditLogEntry"]>();
  const rows = useMemo(
    () => (q.data?.pages ?? []).flatMap((p) => p.data),
    [q.data],
  );
  return (
    <>
      <PageHeader
        title="Audit log"
        description="Every material action, hash-chained per tenant so tampering is detectable. Tenant-level entries appear in both environments."
      />
      <Panel className="overflow-hidden">
        <Table aria-label="Audit log">
          <thead>
            <tr>
              <Th className="text-right">#</Th>
              <Th>When</Th>
              <Th>Action</Th>
              <Th>Actor</Th>
              <Th>Resource</Th>
              <Th>Outcome</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={6} /> : null}
            {rows.map((e) => (
              <Tr
                key={e.id}
                density="compact"
                className="cursor-pointer"
                selected={open?.id === e.id}
                onClick={() => setOpen(e)}
              >
                <DataCell className="text-right text-text-muted">
                  {e.sequence}
                </DataCell>
                <DataCell className="text-text-secondary">
                  {formatTimestamp(e.occurred_at)}
                </DataCell>
                <DataCell>{e.action}</DataCell>
                <Td className="text-body-sm">
                  {e.actor.type === "user" ? (
                    userName(e.actor.id)
                  ) : (
                    <span className="mono text-data">{e.actor.id}</span>
                  )}
                </Td>
                <DataCell className="text-text-secondary">
                  {e.resource.id}
                </DataCell>
                <Td>
                  <Status tone={e.outcome === "success" ? "ok" : "error"}>
                    {humanize(e.outcome)}
                  </Status>
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.hasNextPage ? (
          <div className="border-t border-border-subtle p-3">
            <Button size="sm" onClick={() => void q.fetchNextPage()}>
              Load more
            </Button>
          </div>
        ) : null}
      </Panel>
      <Drawer
        open={!!open}
        onOpenChange={(o) => !o && setOpen(undefined)}
        title={open?.action ?? ""}
        subtitle={open ? <span className="mono">{open.id}</span> : undefined}
      >
        {open ? (
          <div className="flex flex-col gap-5">
            <KeyValues
              items={[
                [
                  "Sequence",
                  <span key="s" className="mono">
                    {open.sequence}
                  </span>,
                ],
                [
                  "When",
                  <span key="w" className="mono">
                    {formatTimestamp(open.occurred_at)}
                  </span>,
                ],
                [
                  "Actor",
                  <span key="a" className="mono">
                    {open.actor.id}
                  </span>,
                ],
                [
                  "Resource",
                  <span
                    key="r"
                    className="mono"
                  >{`${open.resource.type} ${open.resource.id}`}</span>,
                ],
                ["Environment", open.environment ?? "Tenant"],
                [
                  "Request",
                  <span key="q" className="mono">
                    {open.request_id}
                  </span>,
                ],
                [
                  "Hash",
                  <span key="h" className="mono break-all">
                    {open.hash}
                  </span>,
                ],
              ]}
            />
            <CodeBlock value={open.metadata} />
          </div>
        ) : null}
      </Drawer>
    </>
  );
}

export function Team() {
  const { can, session } = useSession();
  const members = useTeam();
  const invites = useInvites();
  const [inviting, setInviting] = useState(false);
  const [link, setLink] = useState<string>();
  const update = useAction(
    async (id: string, body: Schemas["UpdateTeamMemberRequest"]) =>
      unwrap(
        await api().PATCH("/api/v1/team/members/{user_id}", {
          params: { path: { user_id: id } },
          body,
        }),
      ),
    { invalidate: ["team"] },
  );
  const invite = useAction(
    async (body: Schemas["CreateInviteRequest"]) =>
      unwrap(await api().POST("/api/v1/team/invites", { body })),
    {
      invalidate: ["invites"],
      onSuccess: (r) => setLink(r.invite_url),
    },
  );
  const revoke = useAction(
    async (id: string) =>
      unwrap(
        await api().POST("/api/v1/team/invites/{invite_id}/revoke", {
          params: { path: { invite_id: id } },
        }),
      ),
    { invalidate: ["invites"] },
  );
  const writable = can("team:write");
  return (
    <>
      <PageHeader
        title="Team"
        description="People who sign in to RiskOS. Every account uses a second factor."
        actions={
          writable ? (
            <Button variant="primary" onClick={() => setInviting(true)}>
              Invite
            </Button>
          ) : null
        }
      />
      {update.error ? <Notice tone="error">{update.error}</Notice> : null}
      <Panel className="mb-4 overflow-hidden">
        <PanelHeader
          title="Members"
          legend={`${members.data?.data.length ?? 0}`}
        />
        <Table aria-label="Members">
          <thead>
            <tr>
              <Th>Person</Th>
              <Th>Role</Th>
              <Th>Status</Th>
              <Th>Second factor</Th>
              <Th>Last sign-in</Th>
            </tr>
          </thead>
          <tbody>
            {members.isLoading ? <SkeletonRows columns={5} rows={3} /> : null}
            {(members.data?.data ?? []).map((m) => {
              const self = m.id === session.user.id;
              return (
                <Tr key={m.id}>
                  <Td className="text-body-sm">
                    {m.name}{" "}
                    {self ? (
                      <span className="text-text-muted">(you)</span>
                    ) : null}
                    <span className="mono block text-label text-text-muted">
                      {m.email}
                    </span>
                  </Td>
                  <Td>
                    {writable && !self ? (
                      <Select
                        aria-label={`Role of ${m.name}`}
                        className="h-control-sm w-44"
                        value={m.role}
                        onChange={(e) =>
                          void update.run(m.id, {
                            role: e.target.value as Schemas["Role"],
                          })
                        }
                      >
                        {ROLES.map((r) => (
                          <option key={r.value} value={r.value}>
                            {r.label}
                          </option>
                        ))}
                      </Select>
                    ) : (
                      <span className="text-body-sm">{roleLabel(m.role)}</span>
                    )}
                  </Td>
                  <Td>
                    {writable && !self ? (
                      <Button
                        size="sm"
                        variant={m.status === "active" ? "danger" : "secondary"}
                        onClick={() =>
                          void update.run(m.id, {
                            status:
                              m.status === "active" ? "disabled" : "active",
                          })
                        }
                      >
                        {m.status === "active" ? "Disable" : "Enable"}
                      </Button>
                    ) : (
                      <Status tone={m.status === "active" ? "ok" : "off"}>
                        {humanize(m.status)}
                      </Status>
                    )}
                  </Td>
                  <Td>
                    <Status tone={m.mfa_enabled ? "ok" : "warn"}>
                      {m.mfa_enabled ? "Enrolled" : "Not yet"}
                    </Status>
                  </Td>
                  <DataCell className="text-text-muted">
                    {m.last_login_at ? formatAge(m.last_login_at) : "never"}
                  </DataCell>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {members.isError ? (
          <ErrorState error={members.error} className="m-4" />
        ) : null}
      </Panel>
      <Panel className="overflow-hidden">
        <PanelHeader title="Invites" legend="single use, 72 hours" />
        {revoke.error ? <Notice tone="error">{revoke.error}</Notice> : null}
        <Table aria-label="Invites">
          <thead>
            <tr>
              <Th>Email</Th>
              <Th>Role</Th>
              <Th>Status</Th>
              <Th>Expires</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {(invites.data?.data ?? []).map((i) => (
              <Tr key={i.id} density="compact">
                <DataCell>{i.email}</DataCell>
                <Td className="text-body-sm">{roleLabel(i.role)}</Td>
                <Td>
                  <Tag>{humanize(i.status)}</Tag>
                </Td>
                <DataCell className="text-text-muted">
                  {formatTimestamp(i.expires_at)}
                </DataCell>
                <Td className="text-right">
                  {writable && i.status === "pending" ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => void revoke.run(i.id)}
                    >
                      Revoke
                    </Button>
                  ) : null}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
        {invites.isSuccess && (invites.data?.data.length ?? 0) === 0 ? (
          <Empty title="No invites" />
        ) : null}
      </Panel>
      <Modal
        open={inviting}
        onOpenChange={(o) => {
          if (!o) {
            setInviting(false);
            setLink(undefined);
          }
        }}
        title="Invite a person"
      >
        {link ? (
          <div className="flex flex-col gap-3">
            <Notice tone="warn">
              Shown once. Send this link to the person directly; RiskOS does not
              send email yet.
            </Notice>
            <CodeBlock value={link} />
            <div className="flex gap-2">
              <Button
                size="sm"
                onClick={() => void navigator.clipboard.writeText(link)}
              >
                Copy link
              </Button>
              <Button
                size="sm"
                variant="primary"
                onClick={() => {
                  setInviting(false);
                  setLink(undefined);
                }}
              >
                Done
              </Button>
            </div>
          </div>
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              void invite.run({
                email: String(f.get("email")),
                role: String(f.get("role")) as Schemas["Role"],
              });
            }}
          >
            {invite.error ? <Notice tone="error">{invite.error}</Notice> : null}
            <Field label="Email">
              {(p) => (
                <Input {...p} name="email" type="email" required autoFocus />
              )}
            </Field>
            <fieldset className="flex flex-col gap-2">
              <legend className="mb-1 text-label text-text-secondary">
                Role
              </legend>
              {ROLES.map((r) => (
                <label key={r.value} className="flex gap-2 text-body">
                  <input
                    type="radio"
                    name="role"
                    value={r.value}
                    defaultChecked={r.value === "analyst"}
                    className="mt-1 accent-accent"
                  />
                  <span>
                    {r.label}
                    <span className="block text-body-sm text-text-muted">
                      {r.help}
                    </span>
                  </span>
                </label>
              ))}
            </fieldset>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setInviting(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={invite.busy}>
                Create invite link
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </>
  );
}

export function AccountSecurity() {
  const { session, refresh } = useSession();
  const reauth = useReauth();
  const [codes, setCodes] = useState<string[]>();
  const [addingTotp, setAddingTotp] = useState(false);
  const [error, setError] = useState<string>();
  const [removing, setRemoving] = useState<{ id: string; name: string }>();
  const remove = useAction(
    async (id: string) =>
      unwrap(
        await api().DELETE("/api/v1/auth/mfa/passkeys/{passkey_id}", {
          params: { path: { passkey_id: id } },
        }),
      ),
    { invalidate: ["session"] },
  );
  const regenerate = useAction(
    async () => unwrap(await api().POST("/api/v1/auth/mfa/recovery-codes")),
    {
      invalidate: ["session"],
      onSuccess: (r) => setCodes(r.recovery_codes),
    },
  );
  const addPasskey = async () => {
    setError(undefined);
    try {
      await reauth(async () => {
        const options = unwrap(
          await api().POST("/api/v1/auth/mfa/passkeys/register/options"),
        );
        const credential = await createPasskey(options);
        return unwrap(
          await api().POST("/api/v1/auth/mfa/passkeys/register", {
            body: { name: "Passkey", credential },
          }),
        );
      });
      await refresh();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "The passkey could not be added.",
      );
    }
  };
  return (
    <>
      <PageHeader
        title="Account security"
        description={`${session.user.name} · ${session.user.email}`}
      />
      <div className="flex max-w-3xl flex-col gap-4">
        {error || remove.error || regenerate.error ? (
          <Notice tone="error">
            {error ?? remove.error ?? regenerate.error}
          </Notice>
        ) : null}
        <Panel className="overflow-hidden">
          <PanelHeader
            title="Passkeys"
            legend="preferred"
            actions={
              passkeysSupported() ? (
                <Button size="sm" onClick={() => void addPasskey()}>
                  Add passkey
                </Button>
              ) : null
            }
          />
          <Table>
            <tbody>
              {session.mfa.passkeys.map((k) => (
                <Tr key={k.id}>
                  <Td className="text-body-sm">{k.name}</Td>
                  <DataCell className="text-text-muted">
                    added {formatAge(k.created_at)} ago
                  </DataCell>
                  <DataCell className="text-text-muted">
                    {k.last_used_at
                      ? `used ${formatAge(k.last_used_at)} ago`
                      : "not used yet"}
                  </DataCell>
                  <Td className="text-right">
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setRemoving({ id: k.id, name: k.name })}
                      disabled={remove.busy}
                    >
                      Remove
                    </Button>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
          {session.mfa.passkeys.length === 0 ? (
            <Empty title="No passkeys">
              Sign in with your fingerprint, face or device PIN.
            </Empty>
          ) : null}
          <Confirm
            open={!!removing}
            onOpenChange={(o) => (o ? undefined : setRemoving(undefined))}
            title={`Remove the passkey “${removing?.name ?? ""}”?`}
            description="You will no longer be able to sign in with it. Your other second factors keep working."
            confirmLabel="Remove passkey"
            busy={remove.busy}
            onConfirm={async () => {
              if (!removing) return;
              const r = await remove.run(removing.id);
              if (r !== undefined) setRemoving(undefined);
            }}
          />
        </Panel>
        <Panel>
          <PanelHeader title="Authenticator app" />
          <PanelBody className="flex flex-col gap-3">
            <Status tone={session.mfa.totp ? "ok" : "off"}>
              {session.mfa.totp ? "Enrolled" : "Not enrolled"}
            </Status>
            {!session.mfa.totp ? (
              addingTotp ? (
                <Enrollment
                  onEnrolled={(c) => {
                    setAddingTotp(false);
                    if (c.length) setCodes(c);
                    void refresh();
                  }}
                />
              ) : (
                <Button
                  size="sm"
                  className="self-start"
                  onClick={() => void reauth(async () => setAddingTotp(true))}
                >
                  Set up an authenticator app
                </Button>
              )
            ) : null}
          </PanelBody>
        </Panel>
        <Panel>
          <PanelHeader
            title="Recovery codes"
            legend={`${session.mfa.recovery_codes_remaining} left`}
          />
          <PanelBody className="flex flex-col gap-3">
            {codes ? (
              <RecoveryCodes
                codes={codes}
                onContinue={() => setCodes(undefined)}
              />
            ) : (
              <>
                <p className="text-body-sm text-text-secondary">
                  Replacing them makes the old codes stop working.
                </p>
                <Button
                  size="sm"
                  className="self-start"
                  onClick={() => void regenerate.run()}
                  disabled={regenerate.busy}
                >
                  Replace recovery codes
                </Button>
              </>
            )}
          </PanelBody>
        </Panel>
      </div>
    </>
  );
}
