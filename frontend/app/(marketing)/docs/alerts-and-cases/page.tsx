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
  P,
  UL,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE as API } from "@/lib/docs";

export const metadata: Metadata = { title: "Alerts and cases" };

export default function Page() {
  return (
    <DocPage
      title="Alerts and cases"
      lead="Risky decisions become alerts for your analysts; related alerts are investigated together in cases. Your team works them in the console, and your systems can follow and drive the same work through the API and webhooks."
    >
      <H2>How alerts are raised</H2>
      <P>
        Alerts are raised moments after the decision, by a background worker,
        never on the decision path: a slow worker delays alerts, never
        decisions. Each evaluation joins at most one alert of each kind. Dry
        runs, rule tests and idempotent replays never raise alerts.
      </P>
      <Fields
        label="Alert kinds"
        head={["Kind", "Raised when", "Grouped by", "Severity"]}
        rows={[
          [
            "risk",
            "The decision reaches the environment's threshold: review by default; your team can lower it to step_up (alert_min_decision in PUT /api/v1/risk/settings). block raises one too.",
            "The event's actor.user_id",
            "The evaluation's risk band; the highest when grouped",
          ],
          [
            "security",
            "A signal was degraded (part of RiskOS did not answer in time), or a webhook endpoint was disabled for failing.",
            "The environment, or the endpoint",
            "high",
          ],
        ]}
      />
      <P>
        <strong>Grouping:</strong> an evaluation joins the most recent{" "}
        <C>open</C> or <C>investigating</C> alert of the same kind and subject
        whose last evaluation is at most 1 hour older; otherwise it opens a new
        alert. A burst of risky events for one customer becomes one alert, with
        its evaluation count, highest score, first and last seen times and its
        newest 100 evaluations with their signals.
      </P>

      <H2>Triage</H2>
      <P>
        <C>PATCH /api/v1/alerts/{"{alert_id}"}</C> (<C>alerts:write</C>) changes
        the status, the assignee (empty unassigns) and adds a note to the
        alert&apos;s timeline. The assignee must be the email of an active
        dashboard user of your workspace, otherwise the request is refused with
        400; the same applies to cases.
      </P>
      <Fields
        label="Alert statuses"
        head={["From", "Allowed changes"]}
        rows={[
          [
            "open, investigating",
            "open, investigating, false_positive, resolved",
          ],
          ["escalated (in a case)", "None: resolve the case."],
          ["false_positive, resolved", "None: final."],
        ]}
      />
      <Code
        label="Triage an alert"
        code={`curl -X PATCH ${API}/api/v1/alerts/alr_06gf… \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"status": "investigating", "assignee": "ada@yourbank.example", "note": "Calling the customer"}'`}
      />
      <P>
        An alert is escalated only by opening a case with it. A change that
        alters nothing does nothing.
      </P>

      <H2>Cases</H2>
      <Code
        label="Open a case with alerts"
        code={`curl ${API}/api/v1/cases \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"title": "Possible account takeover", "alert_ids": ["alr_06gf…", "alr_06gh…"]}'`}
      />
      <UL>
        <li>
          A case can take up to 50 alerts, all <C>open</C> or{" "}
          <C>investigating</C>; if any cannot be escalated, nothing is created.
          Without a priority, the case takes its alerts&apos; highest severity (
          <C>elevated</C> without alerts).
        </li>
        <li>
          <C>PATCH /api/v1/cases/{"{case_id}"}</C> changes title, description,
          priority, assignee and status (<C>open</C>, <C>investigating</C>).
        </li>
        <li>
          <C>POST /api/v1/cases/{"{case_id}"}/notes</C> adds a note. Notes are
          append-only and allowed on resolved cases too.
        </li>
        <li>
          <C>GET /api/v1/cases/{"{case_id}"}</C> returns the case with its
          alerts, notes and timeline.
        </li>
      </UL>
      <H3>Resolving</H3>
      <P>
        Resolve a case with <C>PATCH</C>, <C>{`"status": "resolved"`}</C> and a{" "}
        <C>resolution</C>, with an optional <C>resolution_note</C>. Resolving
        closes its alerts (<C>false_positive</C> for a false-positive
        resolution, <C>resolved</C> otherwise) and is final.
      </P>
      <Fields
        label="Resolutions"
        head={["Resolution", "Meaning"]}
        rows={[
          [
            "confirmed_fraud",
            "Fraud happened. In Nigeria, the case then appears in the compliance work queue until an industry fraud report is prepared (GOV-12).",
          ],
          ["false_positive", "Nothing was wrong; the alerts were noise."],
          ["legitimate", "Unusual, but the customer's genuine activity."],
          ["inconclusive", "Could not be determined."],
        ]}
      />

      <H2>Following the work from your systems</H2>
      <UL>
        <li>
          Subscribe to <C>alert.created</C>, <C>alert.updated</C>,{" "}
          <C>case.created</C> and <C>case.resolved</C>{" "}
          <DocLink href="/docs/webhooks">webhooks</DocLink> to open tickets,
          post to chat or contact the customer.
        </li>
        <li>
          Scopes: <C>alerts:read</C>, <C>alerts:write</C>, <C>cases:read</C>,{" "}
          <C>cases:write</C>. Escalating alerts into a case needs both write
          scopes. Reading a case shows its alerts without <C>alerts:read</C>.
        </li>
        <li>
          Everything is audited; free text (notes, descriptions) stays in the
          alert or case, and the audit log records only that it changed.
        </li>
      </UL>
      <Callout>
        Alerts carry customer references and signal codes, never event payloads.
        Compliance-only signals appear as <C>compliance_review</C>, so analysts
        outside compliance never learn of a suspicion report.
      </Callout>
      <Next href="/docs/going-live" label="Going live" />
    </DocPage>
  );
}
