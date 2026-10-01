"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { CopyValue } from "@/components/ui/misc";
import { Confirm } from "@/components/ui/overlay";
import { PageHeader, Panel, PanelHeader } from "@/components/ui/panel";
import { ErrorState, Loading, Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatTimestamp } from "@/lib/format";
import { useSSO, useUserName } from "@/lib/queries";
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
  const [removing, setRemoving] = useState(false);
  const save = useAction(
    async () =>
      unwrap(
        await api().PUT("/api/v1/sso", {
          body: {
            issuer: issuer.trim(),
            client_id: clientID.trim(),
            enabled,
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
