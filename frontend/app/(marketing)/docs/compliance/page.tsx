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

export const metadata: Metadata = { title: "Compliance" };

export default function Page() {
  return (
    <DocPage
      title="Compliance"
      lead="Every workspace runs its jurisdiction's rule pack next to its own rules: regulatory controls the law requires, risk signals your compliance team can switch on, and the reporting work both create, with its legal deadlines. This page covers what your engineers need to wire it up."
    >
      <Callout>
        RiskOS never submits anything to a regulator. Reports are prepared and
        approved in RiskOS; a person files them in the regulator&apos;s portal
        (goAML for the NFIU) and records the reference it gave.
      </Callout>

      <H2>The compliance profile</H2>
      <P>
        The profile decides which pack applies and which of its rules: the
        jurisdiction, your licence type and your capabilities.
      </P>
      <UL>
        <li>
          <strong>Every workspace has one from the start.</strong> A new
          workspace gets the automatic profile: the Nigeria pack, licence type{" "}
          <C>other</C> and <C>confirmed: false</C>. Its regulatory controls run
          at their defaults (on, as the law requires); every risk signal starts
          off until your compliance team switches it on.
        </li>
        <li>
          <strong>Confirm it.</strong> A person with <C>compliance:write</C>{" "}
          saves the real licence type and capabilities (Compliance → Profile in
          the console, or <C>PUT /api/v1/compliance/profile</C>). Saving makes
          it <C>confirmed</C> and adopts the jurisdiction&apos;s latest pack
          version; rule configuration carries over.
        </li>
      </UL>
      <Code
        label="Set the profile"
        code={`PUT /api/v1/compliance/profile
{
  "jurisdiction": "NG",
  "licence_type": "psp",
  "capabilities": ["instant_payments"]
}`}
      />
      <Fields
        label="Nigeria licence types and capabilities"
        head={["Nigeria pack", "Values"]}
        rows={[
          [
            "licence_type",
            "bank, mfb, psp, mmo, mto, finance_company, fintech, digital_lender, vasp, other",
          ],
          [
            "capabilities",
            "instant_payments (offers instant payments), cash_fee (applies the weekly cash withdrawal charge)",
          ],
        ]}
      />

      <H2>Packs and jurisdictions</H2>
      <UL>
        <li>
          <C>GET /api/v1/compliance/packs</C> lists the packs available, with
          their number of controls, signals and governance presets, and which
          one your profile uses.
        </li>
        <li>
          <C>GET /api/v1/compliance/pack</C> and{" "}
          <C>GET /api/v1/compliance/rules</C> return your pack and its rules.
          Add <C>?jurisdiction=XX</C> to browse another jurisdiction&apos;s pack
          read-only; it applies only once your profile uses it.
        </li>
        <li>
          Packs are researched from primary sources: each rule cites them, and{" "}
          <C>GET /api/v1/compliance/gaps</C> lists open legal questions a person
          must settle. <C>…/clocks</C> lists the legal deadlines and{" "}
          <C>…/glossary</C> the terms.
        </li>
        <li>
          Nigeria (<C>NG</C>), Ghana (<C>GH</C>) and Rwanda (<C>RW</C>) have
          packs. <C>GET /api/v1/compliance/jurisdictions</C> lists them with
          their currency, time zone and regulators, and whether your workspace
          has activated each. Ghana and Rwanda reuse some Nigerian rules under
          their Nigerian IDs, with amounts restated in cedis or francs; their
          own rules have country IDs such as <C>GH-REG-04</C>.
        </li>
        <li>
          Every rule says where it comes from: a cited law (<C>regulatory</C>),
          a published typology with no mandated number (<C>industry_practice</C>
          ) or the library author&apos;s design, whose numbers are starting
          values, not regulation (<C>my_design</C>). Rules whose legal number
          was not found ship off, with <C>off_reason</C> saying why.
        </li>
      </UL>

      <H2>Several countries</H2>
      <P>
        A workspace can operate in several countries. Each country it operates
        in is an <em>activation</em>: a jurisdiction, a booking entity (the
        licensed entity that holds the customer relationship), its licence type
        and a status. <C>GET /api/v1/compliance/activations</C> lists them.
      </P>
      <UL>
        <li>
          <strong>Name the booking entity on each event.</strong> Send{" "}
          <C>compliance.booking_entity</C>: the event is judged by that
          entity&apos;s country pack only, in that country&apos;s time zone and
          currency.
        </li>
        <li>
          <strong>The fallback.</strong> An event that names no booking entity
          (or one you have not activated) is judged by your fallback activation,
          shown first in the list. Every workspace starts with Nigeria as its
          fallback. With no fallback, the event is reported as <C>unresolved</C>{" "}
          and no pack judges it: RiskOS never guesses a country.
        </li>
        <li>
          <strong>The decision says which country judged it</strong>, in{" "}
          <C>jurisdiction</C>: the country, the pack version and how it was
          found (<C>booking_entity</C>, <C>fallback</C>, <C>unresolved</C>, or{" "}
          <C>inactive</C> for an activation in draft or suspended).
        </li>
      </UL>
      <Code
        label="A Ghana-booked cash deposit"
        code={`POST /api/v1/events
{
  "event_id": "dep_0193",
  "type": "transaction",
  "occurred_at": "2026-10-04T09:12:00Z",
  "actor": { "user_id": "cus_77", "account_id": "acc_77" },
  "data": { "transaction_id": "t_0193", "amount": 5000000, "currency": "GHS",
            "direction": "credit", "channel": "branch" },
  "compliance": {
    "kind": "cash_deposit",
    "booking_entity": "gh-bank",
    "facts": { "txn.domestic": true }
  }
}

→ "jurisdiction": { "jurisdiction": "GH", "pack": "gh@2026.10.1",
                    "resolved_by": "booking_entity", "booking_entity": "gh-bank" },
  "controls": [{ "rule": "GH-REG-04", "tasks": ["report.CTR"], ... }]`}
      />

      <H2>Every active rule, for your integration</H2>
      <P>
        <C>GET /api/v1/compliance/active-rules</C> lists every pack rule running
        for your workspace in this environment, across every activated country
        (add <C>?jurisdiction=GH</C> for one), with what your systems must send
        for it to be evaluated:
      </P>
      <Fields
        label="Active rule fields"
        head={["Field", "Meaning"]}
        rows={[
          [
            "rule, name, kind",
            "The rule and what it is: a control, a risk signal.",
          ],
          ["jurisdiction, pack, booking_entity", "The activation it runs for."],
          ["mode", "live (acts on decisions) or shadow (recorded only)."],
          ["events", "The compliance.kind values that trigger it."],
          [
            "facts",
            "The compliance.facts it reads; send them, or the rule is not evaluated.",
          ],
          [
            "refs",
            "The compliance.refs its time windows are kept for (national_id, agent…).",
          ],
          [
            "unset_settings",
            "Settings your compliance team must set before it can be evaluated.",
          ],
        ]}
      />
      <P>
        Your own rules are at <C>GET /api/v1/rules</C>. Both kinds run on every
        event, and both appear in the decision.
      </P>

      <H2>Kinds of pack rule</H2>
      <Fields
        label="Kinds of pack rule"
        head={["Kind", "What it does", "Starts"]}
        rows={[
          [
            "Regulatory control",
            "Sets the action the law requires: decline, hold, restrict, flag, fee, block_session, review or block. Appears in the decision's controls.",
            "live",
          ],
          [
            "Risk signal",
            "Adds weight to the score and can require at least review or step_up.",
            "off on automatic profiles; your team switches each on",
          ],
          [
            "Governance preset",
            "A workflow (approvals, response times, reporting), not an event rule.",
            "on",
          ],
        ]}
      />

      <H2>Sending what the pack needs</H2>
      <P>
        Pack rules read your events plus a <C>compliance</C> block: the
        event&apos;s kinds and facts about the customer and the transaction that
        RiskOS cannot know (KYC tier, whether the account has a BVN, PEP
        status…). The fields are in{" "}
        <DocLink href="/docs/events#compliance-data">
          Sending events → Compliance data
        </DocLink>
        .
      </P>
      <OL>
        <li>
          Send events and look at <C>not_evaluated</C> in the decisions: each
          entry names a rule and what it lacked (<C>fact:…</C>, <C>setting:…</C>
          , <C>history:…</C>, <C>currency:…</C>).
        </li>
        <li>
          Add the facts your systems have. Missing data is never read as no
          risk.
        </li>
        <li>
          Your compliance team sets the amounts the institution must choose (
          <C>setting:…</C>) and the exchange rates for amounts the law states in
          another currency (<C>currency:…</C>).
        </li>
      </OL>
      <H3>Exchange rates</H3>
      <P>
        Rules on US dollar amounts read the amount your event sends in{" "}
        <C>txn.amount_usd</C>, else the event&apos;s amount converted with your
        rates. <C>PUT /api/v1/compliance/fx-rates/{"{currency}"}</C> (people
        with <C>compliance:write</C>) sets US dollars per one unit of the
        currency; <C>GET /api/v1/compliance/fx-rates</C> lists them. Without
        either, the rule is not evaluated.
      </P>

      <H2>Configuring a rule</H2>
      <P>
        Each environment can set a pack rule&apos;s mode (<C>live</C>,{" "}
        <C>shadow</C>, <C>off</C>) and its settings, with a reason:
      </P>
      <Code
        label="Configure a pack rule"
        code={`PUT /api/v1/compliance/rules/ATO-01/config
{
  "mode": "live",
  "settings": { "floor": 5000000 },
  "reason": "Floor agreed by the risk committee on 2 October"
}`}
      />
      <UL>
        <li>
          Needs <C>compliance:configure</C> (people only) and, in the console, a
          recent second factor. Setting names and their bounds are on the rule (
          <C>GET /api/v1/compliance/rules/{"{rule_id}"}</C>); every setting
          stays within the pack&apos;s minimum and maximum, and for legal limits
          the maximum is the legal one.
          <C>settings</C> holds only values that differ from the defaults.
        </li>
        <li>
          Every change is a new version with its reason and author (version 0 is
          the defaults). <C>…/history</C> lists them and <C>…/rollback</C>{" "}
          restores one.
        </li>
        <li>
          <strong>Two people in production.</strong> For a regulatory control,
          any settings change and any move away from <C>live</C> answers{" "}
          <C>202</C> with a proposed change (<C>pch_…</C>) that another person
          with <C>rules:approve</C> approves (
          <C>POST /api/v1/compliance/changes/{"{change_id}"}/approve</C>).
          Switching back to <C>live</C>, and changes in test or to risk signals,
          apply at once.
        </li>
        <li>
          Changes reach every RiskOS instance within 5 seconds and are audited.
        </li>
      </UL>
      <H3>Backtests</H3>
      <P>
        <C>POST /api/v1/compliance/rules/{"{rule_id}"}/backtest</C> (
        <C>compliance:read</C> and <C>events:read</C>) replays the
        environment&apos;s events of the last <C>days</C> (default 7, at most
        30; at most 20,000 events) through the rule with candidate{" "}
        <C>settings</C> and with its current configuration, side by side,
        whatever its mode. Nothing is stored and live state is untouched. Events
        evaluated before the pack ran are not replayed.
      </P>
      <Code
        label="Backtest a setting"
        code={`curl ${API}/api/v1/compliance/rules/ATO-01/backtest \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"days": 14, "settings": {"floor": 2000000}}'`}
      />

      <H2>The work queue</H2>
      <P>
        When a live control fires, RiskOS records the work it requires as{" "}
        <strong>compliance tasks</strong> with their legal deadlines:{" "}
        <C>GET /api/v1/compliance/tasks</C>. In the Nigeria pack, for example:
      </P>
      <Fields
        label="Examples of compliance tasks"
        head={["Task", "From", "Deadline"]}
        rows={[
          [
            "report.FTR",
            "REG-10, international transfers of US$10,000 or more",
            "1 day from the transfer (ftr_due)",
          ],
          [
            "report.STR",
            "A compliance officer confirming suspicion on a case (REG-14)",
            "24 hours from confirmation (str_due)",
          ],
          [
            "report.CTR, report.PEP_monthly, report.sanctions",
            "REG-17, REG-18, REG-11",
            "None in the pack",
          ],
          [
            "edd, source_of_funds, source_of_wealth, senior_approval",
            "REG-12, politically exposed persons",
            "None in the pack",
          ],
          [
            "close_account",
            "REG-16, accounts without a BVN",
            "close_after (30 days) from first seen",
          ],
          [
            "report.industry_fraud",
            "A confirmed-fraud case (GOV-12)",
            "None: the source states none",
          ],
        ]}
      />
      <P>
        Actions for your own systems (fresh authentication, enhanced MFA) are
        not tasks: they come back with the decision in <C>controls</C>. The
        queue also lists escalations: alerts not screened within 72 hours
        (REG-14) and urgent alerts nobody picked up within 30 minutes (GOV-11,
        configurable).
      </P>
      <H3>Reports</H3>
      <Code
        label="Report lifecycle"
        code={`open (draft) ──edit──▶ approved ──file (regulator's reference)──▶ filed
      └──────────── dismissed (with a note) ◀───────────┘`}
      />
      <UL>
        <li>
          <C>PATCH …/tasks/{"{id}"}/report</C> edits the draft;{" "}
          <C>POST …/approve</C> approves it (compliance officers only);{" "}
          <C>GET …/export</C> produces the file to upload; <C>POST …/file</C>{" "}
          records the portal&apos;s reference; <C>POST …/close</C> marks other
          tasks done or dismisses them with a note.
        </li>
        <li>
          A suspicious transaction report is approved only when it is complete
          enough for goAML: a predicate offence, at least one indicator and an
          accepted category (REG-19).
        </li>
        <li>
          <C>POST /api/v1/compliance/suspicion</C> records a compliance
          officer&apos;s suspicion on a case and opens the report.{" "}
          <C>POST /api/v1/compliance/fraud-reports</C> prepares an industry
          fraud report from a confirmed-fraud case.
        </li>
        <li>
          Approving, filing and confirming suspicion need a recent second factor
          in the console.
        </li>
      </UL>

      <H2>No tipping off</H2>
      <P>
        Suspicious transaction reports and compliance-only signals are visible
        only to compliance officers (<C>compliance:sensitive</C>), which
        administrators and API keys never hold. Everywhere else (decisions,
        alerts, webhooks, analytics) a compliance-only signal shows as{" "}
        <C>compliance_review</C> (&quot;Compliance review&quot;) and keeps its
        points. Never show that code or its cause to a customer.
      </P>
      <Next href="/docs/alerts-and-cases" label="Alerts and cases" />
    </DocPage>
  );
}
