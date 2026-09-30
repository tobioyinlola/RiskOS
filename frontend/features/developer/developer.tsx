"use client";

import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/instrument/button";
import { Field, Input } from "@/components/instrument/field";
import {
  CodeBlock,
  KeyValues,
  Status,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Tag,
} from "@/components/instrument/misc";
import { Drawer, Modal } from "@/components/instrument/overlay";
import {
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/instrument/panel";
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
import {
  useApiKeys,
  useDeliveries,
  useDelivery,
  useWebhook,
  useWebhooks,
} from "@/lib/queries";
import { useSession } from "@/lib/session";

const WEBHOOK_EVENTS: Schemas["WebhookEventType"][] = [
  "alert.created",
  "alert.updated",
  "case.created",
  "case.resolved",
];

/** A secret shown once, with a clear warning and a copy button. */
function SecretOnce({
  label,
  secret,
  onDone,
}: {
  label: string;
  secret: string;
  onDone: () => void;
}) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex flex-col gap-3">
      <Notice tone="warn">
        Shown once. Store it in a secrets manager now; RiskOS cannot show it
        again.
      </Notice>
      <p className="legend">{label}</p>
      <CodeBlock value={secret} />
      <div className="flex gap-2">
        <Button
          size="sm"
          onClick={() => {
            void navigator.clipboard.writeText(secret);
            setCopied(true);
          }}
        >
          {copied ? "Copied" : "Copy"}
        </Button>
        <Button size="sm" variant="primary" onClick={onDone}>
          Done
        </Button>
      </div>
    </div>
  );
}

export function Developer() {
  const { can, environment } = useSession();
  return (
    <>
      <PageHeader
        title="Developer"
        description={`API keys and webhooks of the ${environment} environment. Test and production keep separate keys and endpoints.`}
      />
      <Tabs defaultValue={can("api_keys:read") ? "keys" : "webhooks"}>
        <TabList className="mb-4">
          {can("api_keys:read") ? <Tab value="keys">API keys</Tab> : null}
          {can("webhooks:read") ? <Tab value="webhooks">Webhooks</Tab> : null}
        </TabList>
        {can("api_keys:read") ? (
          <TabPanel value="keys">
            <ApiKeys />
          </TabPanel>
        ) : null}
        {can("webhooks:read") ? (
          <TabPanel value="webhooks">
            <Webhooks />
          </TabPanel>
        ) : null}
      </Tabs>
    </>
  );
}

function ApiKeys() {
  const q = useApiKeys();
  const { can, session } = useSession();
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string>();
  const [confirm, setConfirm] = useState<{
    key: Schemas["ApiKey"];
    action: "rotate" | "revoke";
  }>();
  const create = useAction(
    async (body: Schemas["CreateApiKeyRequest"]) =>
      unwrap(await api().POST("/api/v1/api-keys", { body })),
    {
      invalidate: ["api-keys"],
      onSuccess: (r) => setSecret(r.secret),
    },
  );
  const rotate = useAction(
    async (id: string) =>
      unwrap(
        await api().POST("/api/v1/api-keys/{key_id}/rotate", {
          params: { path: { key_id: id } },
          body: { overlap_seconds: 86400 },
        }),
      ),
    { invalidate: ["api-keys"], onSuccess: (r) => setSecret(r.secret) },
  );
  const revoke = useAction(
    async (id: string) =>
      unwrap(
        await api().POST("/api/v1/api-keys/{key_id}/revoke", {
          params: { path: { key_id: id } },
        }),
      ),
    { invalidate: ["api-keys"] },
  );
  const keys = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="API keys"
        legend="machine access"
        actions={
          can("api_keys:write") ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => setCreating(true)}
            >
              Create key
            </Button>
          ) : null
        }
      />
      {revoke.error || rotate.error ? (
        <Notice tone="error">{revoke.error ?? rotate.error}</Notice>
      ) : null}
      <Table aria-label="API keys">
        <thead>
          <tr>
            <Th>Key</Th>
            <Th>Prefix</Th>
            <Th>Scopes</Th>
            <Th>Status</Th>
            <Th>Created</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={6} rows={3} /> : null}
          {keys.map((k) => (
            <Tr key={k.id}>
              <Td className="text-body-sm">
                {k.name}
                <span className="mono block text-legend text-ink-3">
                  {k.id}
                </span>
              </Td>
              <DataCell>{k.prefix}</DataCell>
              <Td
                className="mono text-legend text-ink-2"
                title={k.scopes.join(", ")}
              >
                {k.scopes.length} scopes
              </Td>
              <Td>
                <Status tone={k.status === "active" ? "ok" : "off"}>
                  {humanize(k.status)}
                </Status>
              </Td>
              <DataCell className="text-ink-3">
                {formatAge(k.created_at)}
              </DataCell>
              <Td className="text-right whitespace-nowrap">
                {can("api_keys:write") && k.status === "active" ? (
                  <span className="inline-flex gap-2">
                    <Button
                      size="sm"
                      onClick={() => setConfirm({ key: k, action: "rotate" })}
                    >
                      Rotate
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => setConfirm({ key: k, action: "revoke" })}
                    >
                      Revoke
                    </Button>
                  </span>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      <Modal
        open={creating}
        onOpenChange={(o) => {
          if (!o) {
            setCreating(false);
            setSecret(undefined);
          }
        }}
        title="Create API key"
      >
        {secret ? (
          <SecretOnce
            label="API key"
            secret={secret}
            onDone={() => {
              setCreating(false);
              setSecret(undefined);
            }}
          />
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              void create.run({
                name: String(f.get("name")),
                scopes: f.getAll("scopes").map(String) as Schemas["Scope"][],
              });
            }}
          >
            {create.error ? <Notice tone="error">{create.error}</Notice> : null}
            <Field
              label="Name"
              hint="What uses this key, e.g. payments-service."
            >
              {(p) => (
                <Input {...p} name="name" required maxLength={100} autoFocus />
              )}
            </Field>
            <fieldset>
              <legend className="mb-2 text-label text-ink-2">
                Scopes (a key can only hold scopes you hold)
              </legend>
              <div className="grid max-h-56 grid-cols-2 gap-1 overflow-y-auto">
                {session.permissions.map((p) => (
                  <label
                    key={p}
                    className="flex items-center gap-2 text-body-sm"
                  >
                    <input
                      type="checkbox"
                      name="scopes"
                      value={p}
                      defaultChecked={p === "events:write"}
                      className="accent-[var(--signal-ink)]"
                    />
                    <span className="mono text-data">{p}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setCreating(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={create.busy}>
                Create key
              </Button>
            </div>
          </form>
        )}
      </Modal>
      <Modal
        open={!!confirm}
        onOpenChange={(o) => {
          if (!o) {
            setConfirm(undefined);
            setSecret(undefined);
          }
        }}
        title={
          confirm?.action === "revoke"
            ? `Revoke ${confirm.key.name}?`
            : `Rotate ${confirm?.key.name ?? ""}?`
        }
        description={
          confirm?.action === "revoke"
            ? "Every request with this key fails within 5 seconds. This cannot be undone."
            : "A new key replaces it; the old one keeps working for 24 hours so you can deploy the new one."
        }
      >
        {secret && confirm?.action === "rotate" ? (
          <SecretOnce
            label="New API key"
            secret={secret}
            onDone={() => {
              setConfirm(undefined);
              setSecret(undefined);
            }}
          />
        ) : (
          <div className="flex justify-end gap-2">
            <Button onClick={() => setConfirm(undefined)}>Cancel</Button>
            {confirm?.action === "revoke" ? (
              <Button
                variant="danger"
                disabled={revoke.busy}
                onClick={async () => {
                  const r = await revoke.run(confirm.key.id);
                  if (r) setConfirm(undefined);
                }}
              >
                Revoke {confirm.key.name}
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={rotate.busy}
                onClick={() => confirm && void rotate.run(confirm.key.id)}
              >
                Rotate
              </Button>
            )}
          </div>
        )}
      </Modal>
    </Panel>
  );
}

function Webhooks() {
  const q = useWebhooks();
  const { can } = useSession();
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string>();
  const create = useAction(
    async (body: Schemas["CreateWebhookRequest"]) =>
      unwrap(await api().POST("/api/v1/webhooks", { body })),
    {
      invalidate: ["webhooks"],
      onSuccess: (r) => setSecret(r.secret),
    },
  );
  const hooks = q.data?.data ?? [];
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title="Webhook endpoints"
        legend="signed, retried for an hour"
        actions={
          can("webhooks:write") ? (
            <Button
              variant="primary"
              size="sm"
              onClick={() => setCreating(true)}
            >
              Add endpoint
            </Button>
          ) : null
        }
      />
      <Table aria-label="Webhook endpoints">
        <thead>
          <tr>
            <Th>Endpoint</Th>
            <Th>Events</Th>
            <Th>Status</Th>
            <Th>Created</Th>
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={4} rows={2} /> : null}
          {hooks.map((w) => (
            <Tr key={w.id}>
              <Td>
                <Link
                  href={`/developer/webhooks/${w.id}` as "/developer"}
                  className="mono block text-data hover:text-signal-ink"
                >
                  {w.url}
                </Link>
                {w.description ? (
                  <span className="text-body-sm text-ink-3">
                    {w.description}
                  </span>
                ) : null}
              </Td>
              <Td className="mono text-legend text-ink-2">
                {w.event_types.join(" ")}
              </Td>
              <Td>
                {w.status === "active" ? (
                  <Status tone={w.failing_since ? "warn" : "ok"}>
                    {w.failing_since ? "Failing" : "Active"}
                  </Status>
                ) : (
                  <Status tone="error">Disabled</Status>
                )}
              </Td>
              <DataCell className="text-ink-3">
                {formatAge(w.created_at)}
              </DataCell>
            </Tr>
          ))}
        </tbody>
      </Table>
      {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
      {q.isSuccess && hooks.length === 0 ? (
        <Empty title="No endpoints">
          Receive alert and case events in your systems, signed with
          HMAC-SHA256.
        </Empty>
      ) : null}
      <Modal
        open={creating}
        onOpenChange={(o) => {
          if (!o) {
            setCreating(false);
            setSecret(undefined);
          }
        }}
        title="Add webhook endpoint"
      >
        {secret ? (
          <SecretOnce
            label="Signing secret"
            secret={secret}
            onDone={() => {
              setCreating(false);
              setSecret(undefined);
            }}
          />
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              const f = new FormData(e.currentTarget);
              void create.run({
                url: String(f.get("url")),
                description: String(f.get("description") ?? "") || undefined,
                event_types: f
                  .getAll("events")
                  .map(String) as Schemas["WebhookEventType"][],
              });
            }}
          >
            {create.error ? <Notice tone="error">{create.error}</Notice> : null}
            <Field
              label="URL"
              hint="https only; private and internal addresses are refused."
            >
              {(p) => (
                <Input
                  {...p}
                  name="url"
                  type="url"
                  required
                  placeholder="https://hooks.example.com/riskos"
                  className="mono"
                  autoFocus
                />
              )}
            </Field>
            <Field label="Description">
              {(p) => <Input {...p} name="description" maxLength={500} />}
            </Field>
            <fieldset>
              <legend className="mb-2 text-label text-ink-2">Events</legend>
              {WEBHOOK_EVENTS.map((ev) => (
                <label
                  key={ev}
                  className="flex items-center gap-2 py-0.5 text-body-sm"
                >
                  <input
                    type="checkbox"
                    name="events"
                    value={ev}
                    defaultChecked
                    className="accent-[var(--signal-ink)]"
                  />
                  <span className="mono text-data">{ev}</span>
                </label>
              ))}
            </fieldset>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setCreating(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={create.busy}>
                Add endpoint
              </Button>
            </div>
          </form>
        )}
      </Modal>
    </Panel>
  );
}

export function WebhookDetail({ id }: { id: string }) {
  const q = useWebhook(id);
  const deliveries = useDeliveries(id);
  const { can } = useSession();
  const [openDelivery, setOpenDelivery] = useState<string>();
  const [secret, setSecret] = useState<string>();
  const [rotating, setRotating] = useState(false);
  const update = useAction(
    async (body: Schemas["UpdateWebhookRequest"]) =>
      unwrap(
        await api().PATCH("/api/v1/webhooks/{webhook_id}", {
          params: { path: { webhook_id: id } },
          body,
        }),
      ),
    { invalidate: ["webhook", "webhooks"] },
  );
  const test = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/webhooks/{webhook_id}/test", {
          params: { path: { webhook_id: id } },
        }),
      ),
    {
      invalidate: ["deliveries"],
    },
  );
  const rotate = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/webhooks/{webhook_id}/rotate-secret", {
          params: { path: { webhook_id: id } },
          body: { overlap_hours: 24 },
        }),
      ),
    { invalidate: ["webhook"], onSuccess: (r) => setSecret(r.secret) },
  );
  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  const w = q.data!;
  const rows = (deliveries.data?.pages ?? []).flatMap((p) => p.data);
  const writable = can("webhooks:write");
  return (
    <>
      <PageHeader
        title="Webhook endpoint"
        description={<span className="mono text-data">{w.url}</span>}
        actions={
          writable ? (
            <>
              <Button
                onClick={() => void test.run()}
                disabled={test.busy || w.status !== "active"}
              >
                Send test event
              </Button>
              <Button onClick={() => setRotating(true)}>Rotate secret</Button>
              {w.status === "active" ? (
                <Button
                  variant="danger"
                  onClick={() => void update.run({ status: "disabled" })}
                  disabled={update.busy}
                >
                  Disable
                </Button>
              ) : (
                <Button
                  variant="primary"
                  onClick={() => void update.run({ status: "active" })}
                  disabled={update.busy}
                >
                  Enable
                </Button>
              )}
            </>
          ) : null
        }
      />
      <div className="mb-4 flex flex-col gap-2">
        {update.error || test.error ? (
          <Notice tone="error">{update.error ?? test.error}</Notice>
        ) : null}
        {w.disabled_reason ? (
          <Notice tone="error">Disabled: {w.disabled_reason}.</Notice>
        ) : null}
        {w.failing_since && w.status === "active" ? (
          <Notice tone="warn">
            Every delivery has failed since {formatTimestamp(w.failing_since)}.
            After 3 days the endpoint is disabled.
          </Notice>
        ) : null}
      </div>
      <Panel className="mb-4">
        <PanelBody>
          <KeyValues
            items={[
              [
                "Endpoint",
                <span key="i" className="mono">
                  {w.id}
                </span>,
              ],
              [
                "Events",
                <span key="e" className="mono">
                  {w.event_types.join(", ")}
                </span>,
              ],
              [
                "Status",
                <Status
                  key="s"
                  tone={
                    w.status === "active"
                      ? w.failing_since
                        ? "warn"
                        : "ok"
                      : "error"
                  }
                >
                  {humanize(w.status)}
                </Status>,
              ],
              ...(w.previous_secret_valid_until
                ? ([
                    [
                      "Previous secret",
                      <span key="p" className="mono">
                        signs until{" "}
                        {formatTimestamp(w.previous_secret_valid_until)}
                      </span>,
                    ],
                  ] as [string, React.ReactNode][])
                : []),
            ]}
          />
        </PanelBody>
      </Panel>
      <Panel className="overflow-hidden">
        <PanelHeader title="Deliveries" legend="every attempt is logged" />
        <Table aria-label="Deliveries">
          <thead>
            <tr>
              <Th>Event</Th>
              <Th>Status</Th>
              <Th className="text-right">Attempts</Th>
              <Th>Last response</Th>
              <Th>Created</Th>
            </tr>
          </thead>
          <tbody>
            {deliveries.isLoading ? (
              <SkeletonRows columns={5} rows={4} />
            ) : null}
            {rows.map((d) => (
              <Tr
                key={d.id}
                selected={d.id === openDelivery}
                className="cursor-pointer"
                onClick={() => setOpenDelivery(d.id)}
                density="compact"
              >
                <Td>
                  <span className="mono block text-data">{d.event_type}</span>
                  <span className="mono text-legend text-ink-3">
                    {d.event_id}
                  </span>
                </Td>
                <Td>
                  <Status
                    tone={
                      d.status === "succeeded"
                        ? "ok"
                        : d.status === "failed"
                          ? "error"
                          : "warn"
                    }
                  >
                    {humanize(d.status)}
                  </Status>
                </Td>
                <DataCell className="text-right">{d.attempts}</DataCell>
                <DataCell className="text-ink-2">
                  {d.last_status_code ?? d.last_error ?? "—"}
                </DataCell>
                <DataCell className="text-ink-3">
                  {formatAge(d.created_at)}
                </DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {deliveries.isSuccess && rows.length === 0 ? (
          <Empty title="No deliveries yet">
            Send a test event to check your endpoint.
          </Empty>
        ) : null}
      </Panel>
      <DeliveryDrawer
        webhookId={id}
        id={openDelivery}
        onClose={() => setOpenDelivery(undefined)}
        writable={writable}
      />
      <Modal
        open={rotating}
        onOpenChange={(o) => {
          if (!o) {
            setRotating(false);
            setSecret(undefined);
          }
        }}
        title="Rotate signing secret"
        description="For 24 hours deliveries carry signatures from both secrets, so you can switch your receiver without dropping events."
      >
        {secret ? (
          <SecretOnce
            label="New signing secret"
            secret={secret}
            onDone={() => {
              setRotating(false);
              setSecret(undefined);
            }}
          />
        ) : (
          <div className="flex flex-col gap-3">
            {rotate.error ? <Notice tone="error">{rotate.error}</Notice> : null}
            <div className="flex justify-end gap-2">
              <Button onClick={() => setRotating(false)}>Cancel</Button>
              <Button
                variant="primary"
                onClick={() => void rotate.run()}
                disabled={rotate.busy}
              >
                Rotate
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </>
  );
}

function DeliveryDrawer({
  webhookId,
  id,
  onClose,
  writable,
}: {
  webhookId: string;
  id?: string;
  onClose: () => void;
  writable: boolean;
}) {
  const q = useDelivery(webhookId, id);
  const redeliver = useAction(
    async () =>
      unwrap(
        await api().POST(
          "/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}/redeliver",
          {
            params: { path: { webhook_id: webhookId, delivery_id: id! } },
          },
        ),
      ),
    { invalidate: ["deliveries", "delivery"] },
  );
  const d = q.data;
  return (
    <Drawer
      open={!!id}
      onOpenChange={(o) => !o && onClose()}
      title={d ? d.delivery.event_type : "Delivery"}
      subtitle={d ? <span className="mono">{d.delivery.id}</span> : undefined}
      footer={
        d && writable && d.delivery.status === "failed" ? (
          <Button
            variant="primary"
            size="sm"
            onClick={() => void redeliver.run()}
            disabled={redeliver.busy}
          >
            Redeliver
          </Button>
        ) : null
      }
    >
      {q.isLoading ? <Loading /> : null}
      {d ? (
        <div className="flex flex-col gap-5">
          {redeliver.error ? (
            <Notice tone="error">{redeliver.error}</Notice>
          ) : null}
          <KeyValues
            items={[
              [
                "Event ID",
                <span key="e" className="mono">
                  {d.delivery.event_id}
                </span>,
              ],
              [
                "Status",
                <Status
                  key="s"
                  tone={
                    d.delivery.status === "succeeded"
                      ? "ok"
                      : d.delivery.status === "failed"
                        ? "error"
                        : "warn"
                  }
                >
                  {humanize(d.delivery.status)}
                </Status>,
              ],
              [
                "Next attempt",
                <span key="n" className="mono">
                  {d.delivery.next_attempt_at
                    ? formatTimestamp(d.delivery.next_attempt_at)
                    : "—"}
                </span>,
              ],
            ]}
          />
          <section>
            <h3 className="mb-2 text-heading">Attempts</h3>
            <Table>
              <thead>
                <tr>
                  <Th>#</Th>
                  <Th>Started</Th>
                  <Th className="text-right">Duration</Th>
                  <Th>Result</Th>
                </tr>
              </thead>
              <tbody>
                {d.attempts.map((a) => (
                  <Tr key={`${a.attempt}-${a.started_at}`} density="compact">
                    <DataCell>{a.attempt}</DataCell>
                    <DataCell className="text-ink-2">
                      {formatTimestamp(a.started_at)}
                    </DataCell>
                    <DataCell className="text-right">
                      {a.duration_ms} ms
                    </DataCell>
                    <DataCell>
                      {a.error ?? `HTTP ${a.status_code ?? ""}`}
                    </DataCell>
                  </Tr>
                ))}
              </tbody>
            </Table>
          </section>
          <section>
            <h3 className="mb-2 text-heading">Payload</h3>
            <CodeBlock value={d.payload} />
          </section>
          <Tag>Signed with HMAC-SHA256 over id, timestamp and body</Tag>
        </div>
      ) : null}
    </Drawer>
  );
}
