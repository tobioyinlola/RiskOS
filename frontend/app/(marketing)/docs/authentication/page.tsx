import type { Metadata } from "next";

import {
  C,
  Callout,
  Code,
  DocLink,
  DocPage,
  Fields,
  H2,
  H3,
  Next,
  OL,
  P,
  UL,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE as API } from "@/lib/docs";

export const metadata: Metadata = { title: "Authentication and keys" };

export default function Page() {
  return (
    <DocPage
      title="Authentication and keys"
      lead="Your backend authenticates every request with an API key. The key decides which workspace and environment the request acts in and what it may do; nothing in a request can change that."
    >
      <H2>Sending the key</H2>
      <P>
        Send the key in the <C>Authorization</C> header of every request under{" "}
        <C>/api/v1</C>:
      </P>
      <Code
        label="Authorization header"
        code={`Authorization: Bearer rsk_test_...`}
      />
      <UL>
        <li>
          <C>rsk_test_…</C> keys act in your <strong>test</strong> environment,{" "}
          <C>rsk_live_…</C> keys in <strong>production</strong>. The
          environments share nothing: events, rules, webhooks, settings and
          alerts are separate.
        </li>
        <li>
          A missing, malformed, unknown, expired or revoked key gets{" "}
          <C>401 UNAUTHENTICATED</C>, which deliberately never says which.
        </li>
        <li>
          A valid key without the scope an operation needs gets{" "}
          <C>403 FORBIDDEN</C>. A suspended workspace gets{" "}
          <C>403 TENANT_SUSPENDED</C>.
        </li>
        <li>
          Repeated authentication failures from one address are rate limited (30
          per minute by default), so leaked or guessed keys cannot be tried at
          scale.
        </li>
      </UL>
      <P>
        <C>GET /api/v1/whoami</C> tells you what a key is: its workspace,
        environment, ID and scopes. It needs no scope, which makes it a good
        health check for your configuration.
      </P>

      <H2>Scopes</H2>
      <P>
        A key holds only the scopes you give it. Give each system the fewest it
        needs: a payments service that sends events needs <C>events:write</C>{" "}
        and nothing else.
      </P>
      <Fields
        label="Scopes"
        head={["Scope", "Allows", "API keys"]}
        rows={[
          [
            "events:write",
            "Send events and receive decisions (POST /api/v1/events)",
            "Yes",
          ],
          [
            "events:read",
            "List and read events, entity profiles and analytics",
            "Yes",
          ],
          [
            "risk:evaluate",
            "Dry-run evaluations that store no event (POST /api/v1/risk/evaluate)",
            "Yes",
          ],
          [
            "risk:configure",
            "Read and change risk settings: signal weights, thresholds, fail policies; manage the deny list",
            "Yes",
          ],
          [
            "rules:read",
            "Read rules, their versions, statistics and proposed changes; test rules",
            "Yes",
          ],
          [
            "rules:write",
            "Create, change and roll back rules (in production, as proposals)",
            "Yes",
          ],
          [
            "rules:approve",
            "Approve or reject proposed production rule changes",
            "No: people only",
          ],
          ["alerts:read / alerts:write", "Read alerts / triage them", "Yes"],
          [
            "cases:read / cases:write",
            "Read cases / open, update, resolve and annotate them",
            "Yes",
          ],
          [
            "webhooks:read / webhooks:write",
            "Read webhook endpoints and deliveries / manage them",
            "Yes",
          ],
          [
            "api_keys:read / api_keys:write",
            "List keys / create, rotate and revoke keys",
            "Yes",
          ],
          ["audit_logs:read", "Read the workspace's audit log", "Yes"],
          [
            "team:read / team:write",
            "Read console users and invites / manage them",
            "Yes",
          ],
          [
            "compliance:read",
            "Read the rule pack, its rules, deadlines and the compliance work queue",
            "Yes",
          ],
          [
            "compliance:engineering",
            "Read the engineering detail of pack rules",
            "Yes",
          ],
          [
            "compliance:write",
            "Set the compliance profile; edit reports and tasks",
            "No: people only",
          ],
          [
            "compliance:configure",
            "Turn pack rules on or off and change their settings",
            "No: people only",
          ],
          [
            "compliance:sensitive",
            "See suspicious transaction reports and compliance-only signals",
            "No: compliance officers only",
          ],
        ]}
      />
      <Callout>
        Some permissions are never given to keys on purpose: approving a
        production rule change, setting the compliance profile, configuring
        regulatory controls and seeing suspicious transaction reports are a
        person&apos;s decisions, recorded against their name.
      </Callout>

      <H2>Managing keys</H2>
      <P>
        Create keys in the console (<strong>Developer → API keys</strong>) or
        through the API with a key that holds <C>api_keys:write</C>. A new key
        can only receive scopes the creating key or person holds.
      </P>
      <H3>Create</H3>
      <Code
        label="Create a key"
        code={`curl ${API}/api/v1/api-keys \\
  -H "Authorization: Bearer $RISKOS_ADMIN_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"name": "payments backend", "scopes": ["events:write"], "expires_at": "2027-10-01T00:00:00Z"}'`}
      />
      <P>
        The response holds <C>api_key</C> (the key&apos;s ID, name, non-secret{" "}
        <C>prefix</C>, environment, scopes and status) and <C>secret</C>, the
        full key. <strong>The secret is returned once</strong> and cannot be
        retrieved again; RiskOS only stores a keyed hash of it.{" "}
        <C>expires_at</C> is optional.
      </P>
      <H3>Rotate</H3>
      <P>
        Rotate a key regularly, and at once if it may have leaked.{" "}
        <C>POST /api/v1/api-keys/{"{key_id}"}/rotate</C> creates a replacement
        with the same name and scopes and returns its secret once. The old key
        keeps working for <C>overlap_seconds</C> (default 86,400, at most
        604,800: 7 days) so you can deploy the new one without downtime; then it
        expires.
      </P>
      <Code
        label="Rotate a key"
        code={`curl -X POST ${API}/api/v1/api-keys/key_06gg.../rotate \\
  -H "Authorization: Bearer $RISKOS_ADMIN_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"overlap_seconds": 3600}'`}
      />
      <H3>Revoke</H3>
      <P>
        <C>POST /api/v1/api-keys/{"{key_id}"}/revoke</C> stops a key within
        seconds on every RiskOS instance. Revoking an already revoked key
        returns it unchanged.
      </P>

      <H2>Keeping keys safe</H2>
      <OL>
        <li>
          Call RiskOS from your backend only. Never put a key in a mobile app, a
          browser or anything a customer can inspect.
        </li>
        <li>
          Keep keys in a secrets manager, not in source code, images or logs.
        </li>
        <li>
          Use separate keys per system and per environment, so you can rotate or
          revoke one without touching the others.
        </li>
        <li>
          If a key may have leaked: rotate it with a short overlap (or revoke
          it), then check the audit log (<C>GET /api/v1/audit-logs</C>) for what
          it did.
        </li>
      </OL>
      <P>
        The console uses separate, cookie-based sessions with a mandatory second
        factor; those endpoints (the{" "}
        <DocLink href="/docs/reference/session">session</DocLink> group) are not
        for API keys.
      </P>
      <Next href="/docs/events" label="Sending events" />
    </DocPage>
  );
}
