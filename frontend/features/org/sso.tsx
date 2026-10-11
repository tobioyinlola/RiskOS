"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { CopyValue } from "@/components/ui/misc";
import { Confirm } from "@/components/ui/overlay";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { ErrorState, Loading, Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatTimestamp } from "@/lib/format";
import { useSSO, useSSODomains, useUserName } from "@/lib/queries";
import { useSession } from "@/lib/session";

type Connection = Schemas["SSOConnection"];

/** Single sign-on: the tenant's OpenID Connect provider. */
export function SingleSignOn() {
  const { can } = useSession();
  const q = useSSO();
  const writable = can("team:write");
  return (
    <>
      <PageHeader
        title="Single sign-on"
        description="Let people sign in with your identity provider (OpenID Connect). It signs in people who already have a RiskOS account, and RiskOS still asks for its own second factor."
      />
      {q.isError ? <ErrorState error={q.error} /> : null}
      {q.isLoading ? <Loading /> : null}
      {q.data ? (
        <div className="flex max-w-3xl flex-col gap-5">
          <Panel>
            <PanelHeader title="Register RiskOS at your provider" />
            <div className="flex flex-col gap-3 p-4 text-body-sm text-text-secondary">
              <p>
                Create a web application (authorization code flow with PKCE) and
                allow this redirect URI. RiskOS asks for the{" "}
                <span className="mono">openid</span> and{" "}
                <span className="mono">email</span> scopes.
              </p>
              <CopyValue value={q.data.redirect_uri} />
              <p>
                The first time someone signs in with single sign-on, the
                provider must confirm the email of their RiskOS account (
                <span className="mono">email_verified</span>). RiskOS then
                remembers their identity at the provider.
              </p>
            </div>
          </Panel>
          <ConnectionForm
            key={q.data.connection?.updated_at ?? "new"}
            connection={q.data.connection}
            writable={writable}
          />
          {q.data.connection ? <Domains writable={writable} /> : null}
        </div>
      ) : null}
    </>
  );
}

function ConnectionForm({
  connection,
  writable,
}: {
  connection: Connection | null;
  writable: boolean;
}) {
  const userName = useUserName();
  const [issuer, setIssuer] = useState(connection?.issuer ?? "");
  const [clientID, setClientID] = useState(connection?.client_id ?? "");
  const [secret, setSecret] = useState("");
  const [enabled, setEnabled] = useState(connection?.enabled ?? true);
  const [required, setRequired] = useState(connection?.required ?? false);
  const [jit, setJIT] = useState(connection?.jit_enabled ?? false);
  const [jitRole, setJITRole] = useState<Connection["jit_role"]>(
    connection?.jit_role ?? "auditor",
  );
  const [removing, setRemoving] = useState(false);
  // Requiring it is a step of its own, on a connection that has been saved and used.
  const changed =
    secret !== "" ||
    issuer.trim() !== (connection?.issuer ?? "") ||
    clientID.trim() !== (connection?.client_id ?? "");
  const canRequire =
    writable &&
    !!connection &&
    enabled &&
    (!changed || connection.required) &&
    (issuer.trim() === connection.issuer || !connection.required);
  const save = useAction(
    async () =>
      unwrap(
        await api().PUT("/api/v1/sso", {
          body: {
            issuer: issuer.trim(),
            client_id: clientID.trim(),
            enabled,
            required: required && enabled,
            jit_enabled: jit && enabled,
            jit_role: jitRole,
            ...(secret ? { client_secret: secret } : {}),
          },
        }),
      ),
    { invalidate: ["sso"], onSuccess: () => setSecret("") },
  );
  const remove = useAction(
    async () => unwrap(await api().DELETE("/api/v1/sso")),
    { invalidate: ["sso"], onSuccess: () => setRemoving(false) },
  );
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    void save.run();
  };
  return (
    <Panel>
      <PanelHeader
        title="Identity provider"
        legend={
          connection
            ? connection.enabled
              ? "on"
              : "set up, switched off"
            : "not set up"
        }
      />
      <form onSubmit={onSubmit} className="flex flex-col gap-4 p-4">
        {save.error ? <Notice tone="error">{save.error}</Notice> : null}
        {remove.error ? <Notice tone="error">{remove.error}</Notice> : null}
        <Field
          label="Issuer"
          hint="The provider's issuer URL; RiskOS reads its OpenID configuration when you save."
        >
          {(p) => (
            <Input
              {...p}
              value={issuer}
              onChange={(e) => setIssuer(e.target.value)}
              placeholder="https://login.example.com"
              className="mono"
              type="url"
              required
              disabled={!writable}
            />
          )}
        </Field>
        <Field label="Client ID">
          {(p) => (
            <Input
              {...p}
              value={clientID}
              onChange={(e) => setClientID(e.target.value)}
              className="mono"
              maxLength={256}
              required
              disabled={!writable}
            />
          )}
        </Field>
        <Field
          label="Client secret"
          hint={
            connection
              ? "Stored encrypted and never shown. Leave empty to keep it."
              : "Stored encrypted and never shown."
          }
        >
          {(p) => (
            <Input
              {...p}
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              type="password"
              autoComplete="off"
              maxLength={1024}
              required={!connection}
              disabled={!writable}
            />
          )}
        </Field>
        <label className="flex items-center gap-2 text-body">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            disabled={!writable}
            className="accent-accent"
          />
          Allow sign-in with this provider
        </label>
        <div className="flex flex-col gap-1">
          <label className="flex items-center gap-2 text-body">
            <input
              type="checkbox"
              checked={required && enabled}
              onChange={(e) => setRequired(e.target.checked)}
              disabled={!canRequire && !required}
              className="accent-accent"
            />
            Require single sign-on
          </label>
          <p className="text-label text-text-muted">
            Password and Google sign-in are refused for everyone in this
            workspace. You must have signed in through your provider yourself
            first, and the connection is saved before it is required. If the
            provider becomes unavailable, RiskOS support can switch the
            requirement off.
          </p>
        </div>
        <div className="flex flex-col gap-2">
          <label className="flex items-center gap-2 text-body">
            <input
              type="checkbox"
              checked={jit && enabled}
              onChange={(e) => setJIT(e.target.checked)}
              disabled={!writable || !connection || !enabled}
              className="accent-accent"
            />
            Create accounts at first sign-in
          </label>
          <p className="text-label text-text-muted">
            People whose email is at a verified domain below can sign in without
            an invite; the account is created then, with the role chosen here
            and no password. Verify a domain first.
          </p>
          <Field label="Role of accounts created this way">
            {(p) => (
              <Select
                {...p}
                value={jitRole}
                onChange={(e) =>
                  setJITRole(e.target.value as Connection["jit_role"])
                }
                disabled={!writable || !connection}
              >
                <option value="auditor">Auditor (read-only)</option>
                <option value="analyst">Analyst</option>
                <option value="risk_manager">Risk manager</option>
                <option value="compliance_officer">Compliance officer</option>
                <option value="developer">Developer</option>
              </Select>
            )}
          </Field>
        </div>
        {connection ? (
          <p className="text-label text-text-muted">
            Last changed {formatTimestamp(connection.updated_at)} by{" "}
            {userName(connection.updated_by)}
          </p>
        ) : null}
        {writable ? (
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="primary" disabled={save.busy}>
              {connection ? "Save changes" : "Set up single sign-on"}
            </Button>
            {connection ? (
              <Button variant="danger" onClick={() => setRemoving(true)}>
                Remove
              </Button>
            ) : null}
          </div>
        ) : null}
      </form>
      <Confirm
        open={removing}
        onOpenChange={setRemoving}
        title="Remove single sign-on?"
        description="People sign in with their password again. RiskOS forgets their identities at the provider."
        confirmLabel="Remove single sign-on"
        busy={remove.busy}
        onConfirm={async () => {
          await remove.run();
        }}
      />
    </Panel>
  );
}

/** The email domains the workspace proves it controls, for creating accounts at first sign-in. */
function Domains({ writable }: { writable: boolean }) {
  const q = useSSODomains();
  const [name, setName] = useState("");
  const [removing, setRemoving] = useState<string>();
  const add = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/sso/domains", {
          body: { domain: name.trim() },
        }),
      ),
    { invalidate: ["sso-domains"], onSuccess: () => setName("") },
  );
  const verify = useAction(
    async (domain: string) =>
      unwrap(
        await api().POST("/api/v1/sso/domains/{domain}/verify", {
          params: { path: { domain } },
        }),
      ),
    { invalidate: ["sso-domains"] },
  );
  const remove = useAction(
    async (domain: string) =>
      unwrap(
        await api().DELETE("/api/v1/sso/domains/{domain}", {
          params: { path: { domain } },
        }),
      ),
    { invalidate: ["sso-domains"], onSuccess: () => setRemoving(undefined) },
  );
  return (
    <Panel>
      <PanelHeader title="Email domains" />
      <div className="flex flex-col gap-4 p-4">
        <p className="text-body-sm text-text-secondary">
          Claim a domain, publish the TXT record shown, then verify it. Only a
          verified domain lets people in without an invite, and only one
          workspace can hold a domain.
        </p>
        {add.error ? <Notice tone="error">{add.error}</Notice> : null}
        {verify.error ? <Notice tone="error">{verify.error}</Notice> : null}
        {remove.error ? <Notice tone="error">{remove.error}</Notice> : null}
        {q.isError ? <ErrorState error={q.error} /> : null}
        {q.isLoading ? <Loading /> : null}
        {q.data && q.data.data.length === 0 ? (
          <p className="text-body-sm text-text-muted">No domains yet.</p>
        ) : null}
        {q.data?.data.map((d) => (
          <div
            key={d.domain}
            className="flex flex-col gap-2 rounded-md border border-border-subtle p-3"
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="mono text-body">{d.domain}</span>
              <span className="text-label text-text-muted">
                {d.verified
                  ? `verified ${formatTimestamp(d.verified_at ?? d.created_at)}`
                  : "waiting for the DNS record"}
              </span>
            </div>
            {d.verified ? null : (
              <>
                <p className="text-label text-text-muted">
                  Add a TXT record named
                </p>
                <CopyValue value={d.txt_host} />
                <p className="text-label text-text-muted">with the value</p>
                <CopyValue value={d.txt_value} />
              </>
            )}
            {writable ? (
              <div className="flex flex-wrap gap-2">
                {d.verified ? null : (
                  <Button
                    variant="secondary"
                    disabled={verify.busy}
                    onClick={() => void verify.run(d.domain)}
                  >
                    Verify {d.domain}
                  </Button>
                )}
                <Button variant="danger" onClick={() => setRemoving(d.domain)}>
                  Release {d.domain}
                </Button>
              </div>
            ) : null}
          </div>
        ))}
        {writable ? (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void add.run();
            }}
            className="flex flex-wrap items-end gap-2"
          >
            <Field label="Domain" className="min-w-64 flex-1">
              {(p) => (
                <Input
                  {...p}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="example.com"
                  className="mono"
                  maxLength={253}
                  required
                />
              )}
            </Field>
            <Button type="submit" variant="primary" disabled={add.busy}>
              Claim domain
            </Button>
          </form>
        ) : null}
      </div>
      <Confirm
        open={removing !== undefined}
        onOpenChange={(open) => {
          if (!open) setRemoving(undefined);
        }}
        title={`Release ${removing ?? "this domain"}?`}
        description="Nobody new is let in through it. Accounts already created stay."
        confirmLabel="Release domain"
        busy={remove.busy}
        onConfirm={async () => {
          if (removing) await remove.run(removing);
        }}
      />
    </Panel>
  );
}
