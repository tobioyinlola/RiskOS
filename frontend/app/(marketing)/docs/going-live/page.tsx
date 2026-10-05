import type { Metadata } from "next";

import {
  C,
  Callout,
  DocLink,
  DocPage,
  H2,
  Next,
  OL,
  P,
} from "@/features/docs/ui";

export const metadata: Metadata = { title: "Going live" };

export default function Page() {
  return (
    <DocPage
      title="Going live"
      lead="Work through this list in your test environment, then repeat the configuration steps in production: the two environments share no settings, rules, webhooks or keys."
    >
      <H2>Integration</H2>
      <OL>
        <li>
          Every event your platform should screen is sent before the action
          completes: transactions, logins, new beneficiaries, device, session
          and profile changes (
          <DocLink href="/docs/events">Sending events</DocLink>).
        </li>
        <li>
          <C>event_id</C> is stable for an event and reused on every retry;{" "}
          <C>occurred_at</C> is the real time of the event, in UTC or with an
          offset.
        </li>
        <li>Amounts are integers in minor units, with the right currency.</li>
        <li>
          No card numbers, raw BVNs or phone numbers anywhere in an event: send
          your own opaque references. Your test traffic never triggers{" "}
          <C>422 SENSITIVE_DATA</C>.
        </li>
        <li>
          Every decision value is handled, and an unknown value is treated as{" "}
          <C>review</C> (
          <DocLink href="/docs/decisions">Acting on decisions</DocLink>).
        </li>
        <li>
          Retries follow{" "}
          <DocLink href="/docs/errors">Errors, limits and retries</DocLink>:
          only on <C>429</C>, <C>503</C> and network errors, with backoff and a
          deadline; a documented fallback when no decision arrives in time.
        </li>
        <li>
          <C>evaluation_id</C> is stored with your transaction, and{" "}
          <C>X-Request-ID</C> is logged on errors.
        </li>
      </OL>

      <H2>Keys and access</H2>
      <OL>
        <li>
          Production uses <C>rsk_live_…</C> keys, one per system, each with only
          the scopes it needs, kept in a secrets manager (
          <DocLink href="/docs/authentication">Authentication and keys</DocLink>
          ).
        </li>
        <li>
          Nobody outside your backend can see a key: not apps, browsers or logs.
        </li>
        <li>
          You have practised a rotation with an overlap, so a real one is
          routine.
        </li>
        <li>
          Your team&apos;s console accounts have the right roles, with at least
          two people able to approve production rule changes.
        </li>
      </OL>

      <H2>Compliance and rules</H2>
      <OL>
        <li>
          Your compliance profile is confirmed: the right licence type and
          capabilities (<DocLink href="/docs/compliance">Compliance</DocLink>).
        </li>
        <li>
          <C>not_evaluated</C> is empty, or every remaining entry is understood:
          the facts your systems have are sent, institution settings are set and
          exchange rates are entered.
        </li>
        <li>
          Your compliance team has chosen which risk signals to switch on, after
          watching them in monitoring and backtesting their settings.
        </li>
        <li>
          Your own rules ran in shadow on realistic traffic, and their match
          counts are what you expect (
          <DocLink href="/docs/rules">Rules</DocLink>).
        </li>
        <li>
          The score thresholds and the alert threshold suit your risk appetite.
          Automatic blocking stays off, or ran in shadow for at least 14 days
          before being switched on.
        </li>
        <li>
          The people who prepare, approve and file regulatory reports know the
          work queue and the deadlines. RiskOS never files for you.
        </li>
      </OL>

      <H2>Operations</H2>
      <OL>
        <li>
          Production webhook endpoints are registered, verify signatures, reject
          old timestamps, process each event ID once and answer quickly (
          <DocLink href="/docs/webhooks">Webhooks</DocLink>). A test delivery
          succeeded.
        </li>
        <li>
          Someone watches failed deliveries and the security alerts RiskOS
          raises when an endpoint is disabled or signals are degraded.
        </li>
        <li>
          Analysts know how to triage alerts and resolve cases (
          <DocLink href="/docs/alerts-and-cases">Alerts and cases</DocLink>).
        </li>
        <li>
          You know how to reach RiskOS support and what to quote: request and
          evaluation IDs.
        </li>
      </OL>
      <Callout>
        Switch traffic over gradually if you can (a share of customers first)
        and compare decisions with what you expected before sending everything.
      </Callout>
      <P>
        Every endpoint, field and error is in the{" "}
        <DocLink href="/docs/reference">API reference</DocLink>.
      </P>
      <Next href="/docs/reference" label="API reference" />
    </DocPage>
  );
}
