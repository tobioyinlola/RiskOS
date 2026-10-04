import type { Metadata } from "next";

import {
  C,
  Callout,
  DocLink,
  DocPage,
  Fields,
  H2,
  Next,
  OL,
  P,
  UL,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE } from "@/lib/docs";

export const metadata: Metadata = { title: "Introduction" };

export default function Page() {
  return (
    <DocPage
      title="RiskOS developer documentation"
      lead="RiskOS scores every payment, login and account change your platform sends it, explains the score in named signals, applies the regulatory controls your jurisdiction requires, and routes risky activity to your analysts. These pages take you from a first API call to a production integration."
    >
      <H2>How an integration works</H2>
      <OL>
        <li>
          Your backend sends RiskOS an <strong>event</strong> each time
          something risky can happen: a transaction, a login, a new beneficiary,
          a device or profile change.
        </li>
        <li>
          RiskOS evaluates it in the same request and answers with a{" "}
          <strong>decision</strong>(<C>allow</C>, <C>step_up</C>, <C>review</C>,{" "}
          <C>decline</C>, <C>block</C>…), a risk score from 0 to 100, the
          signals that produced it and any regulatory control that applied.
        </li>
        <li>
          Your backend acts on the decision: completes the payment, asks the
          customer for a second factor, holds the funds for review, or declines.
        </li>
        <li>
          Risky decisions raise <strong>alerts</strong> your team triages in the
          RiskOS console; investigations become <strong>cases</strong>.{" "}
          <strong>Webhooks</strong> tell your systems when alerts and cases
          change.
        </li>
        <li>
          Your risk and compliance teams tune <strong>rules</strong> in the
          console, without code, and see what each rule catches.
        </li>
      </OL>
      <Callout>
        Decisions are advisory: RiskOS never moves money or locks accounts
        itself. Your platform stays in control of what each decision does.
      </Callout>

      <H2>Key concepts</H2>
      <Fields
        label="Key concepts"
        head={["Concept", "What it is"]}
        rows={[
          [
            "Workspace",
            "Your organisation in RiskOS (a tenant). Everything you send and configure belongs to it and is isolated from every other workspace.",
          ],
          [
            "Environment",
            "Each workspace has a test and a production environment, with separate keys, data, rules, webhooks and settings. Test keys start rsk_test_, production keys rsk_live_.",
          ],
          [
            "API key",
            "A secret that authenticates your backend and decides the workspace, the environment and the permissions (scopes) of every request.",
          ],
          [
            "Event",
            "Something that happened on your platform, sent to POST /api/v1/events. Each has your own unique event_id, which makes retries safe.",
          ],
          [
            "Evaluation and decision",
            "RiskOS's assessment of one event: score, risk level, decision, signals, controls. Stored with the event and explainable afterwards.",
          ],
          [
            "Signal",
            "A named reason that added risk points, such as new_device or velocity_anomaly, or one of your own rules.",
          ],
          [
            "Rule",
            "A condition your team defines (or activates from a preset pack) that adds risk points or applies a regulatory control.",
          ],
          [
            "Rule pack",
            "A jurisdiction's preset rules (for example Nigeria): regulatory controls, risk signals and governance presets, each with its legal source.",
          ],
          [
            "Alert and case",
            "Alerts group risky evaluations per customer for triage; cases hold an investigation with notes and a resolution.",
          ],
          [
            "Webhook",
            "A signed HTTPS notification RiskOS sends your systems when alerts and cases change.",
          ],
        ]}
      />

      <H2>Base URL and conventions</H2>
      <P>
        Every product endpoint lives under <C>/api/v1</C> on your RiskOS API
        host. The examples on these pages use <C>{API_URL_EXAMPLE}</C>; replace
        it with your deployment&apos;s API URL (<C>http://localhost:8080</C>{" "}
        when you run RiskOS locally).
      </P>
      <UL>
        <li>
          Requests and responses are JSON. Send{" "}
          <C>Content-Type: application/json</C>. Unknown fields are rejected
          everywhere, so a typo fails loudly instead of being ignored.
        </li>
        <li>
          Authenticate with <C>Authorization: Bearer rsk_…</C>. See{" "}
          <DocLink href="/docs/authentication">Authentication and keys</DocLink>
          .
        </li>
        <li>
          IDs are prefixed by type: <C>evt_</C> events, <C>evl_</C> evaluations,{" "}
          <C>rul_</C> rules, <C>alr_</C> alerts, <C>cas_</C> cases, <C>whk_</C>{" "}
          webhook endpoints, <C>key_</C> API keys. An ID of the wrong type is
          simply not found.
        </li>
        <li>
          Timestamps are RFC 3339 (<C>2026-10-04T09:30:00Z</C>). Amounts are
          integers in the currency&apos;s minor units (kobo, cents), with an ISO
          4217 currency code.
        </li>
        <li>
          Lists return <C>data</C> and <C>next_cursor</C>, newest first. Pass{" "}
          <C>next_cursor</C> back as <C>cursor</C> for the next page;{" "}
          <C>limit</C> is 1 to 100 (default 50).
        </li>
        <li>
          Every response carries an <C>X-Request-ID</C> header; errors also
          carry it in the body. Quote it when you contact support.
        </li>
        <li>
          Errors always have the same shape:{" "}
          <C>{`{"error":{"code","message","request_id"}}`}</C>. See{" "}
          <DocLink href="/docs/errors">Errors, limits and retries</DocLink>.
        </li>
      </UL>

      <H2>Where to start</H2>
      <UL>
        <li>
          <DocLink href="/docs/quickstart">Quickstart</DocLink>: send your first
          event and read its decision in about ten minutes.
        </li>
        <li>
          <DocLink href="/docs/events">Sending events</DocLink> and{" "}
          <DocLink href="/docs/decisions">Acting on decisions</DocLink>: the two
          pages every integration needs.
        </li>
        <li>
          <DocLink href="/docs/going-live">Going live</DocLink>: the checklist
          before production traffic.
        </li>
        <li>
          <DocLink href="/docs/reference">API reference</DocLink>: every
          endpoint, field and response, generated from the API contract itself.
        </li>
      </UL>
      <Next href="/docs/quickstart" label="Quickstart" />
    </DocPage>
  );
}
