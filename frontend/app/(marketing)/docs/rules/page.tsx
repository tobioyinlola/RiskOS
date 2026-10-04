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

export const metadata: Metadata = { title: "Rules" };

export default function Page() {
  return (
    <DocPage
      title="Rules"
      lead="Rules add your own signals to every decision: when an event matches a rule's condition, the rule's code appears in signals and its weight is added to the score. Compliance staff build them in the console without code; this page describes the same rules through the API."
    >
      <Callout>
        Rules only add weight. To stop events outright, use the deny list (
        <C>/api/v1/deny-list</C>): a listed user, account, device, IP address,
        beneficiary or country always gets <C>block</C>.
      </Callout>

      <H2>A rule</H2>
      <Fields
        label="Rule fields"
        head={["Field", "Meaning"]}
        rows={[
          [
            "code",
            "The signal code the rule emits: 3 to 64 characters, a lowercase letter then lowercase letters, digits or _. Unique per environment; cannot reuse a built-in signal's code.",
          ],
          [
            "name, description",
            "What your team sees (name up to 100 characters, description up to 1,000).",
          ],
          ["severity", "low, medium, high or critical."],
          [
            "weight",
            "0 to 100: the points added to the score when an active rule matches.",
          ],
          [
            "mode",
            "active (adds its weight) or shadow (matches are recorded, the score and decision do not change).",
          ],
          [
            "enabled",
            "false switches a rule off. Rules are disabled, never deleted, so their history stays.",
          ],
          ["condition", "The condition tree (below)."],
          [
            "version",
            "The current version. Every change is a new, immutable version.",
          ],
          [
            "pending_change_id",
            "In production, a proposed change awaiting approval.",
          ],
        ]}
      />

      <H2>Conditions</H2>
      <P>A condition is a tree. Each node is exactly one of:</P>
      <UL>
        <li>
          <C>{`{"all": [node, ...]}`}</C>: every node matches (and);
        </li>
        <li>
          <C>{`{"any": [node, ...]}`}</C>: at least one node matches (or);
        </li>
        <li>
          <C>{`{"not": node}`}</C>: the node does not match;
        </li>
        <li>
          <C>{`{"field": "<field>", "op": "<operator>", "value": <value>}`}</C>:
          a comparison.
        </li>
      </UL>
      <Code
        label="A condition"
        code={`{
  "all": [
    { "field": "event.type", "op": "eq", "value": "transaction" },
    { "field": "transaction.direction", "op": "eq", "value": "debit" },
    { "field": "transaction.amount", "op": "gte", "value": 50000000 },
    {
      "any": [
        { "field": "feature.new_beneficiary", "op": "eq", "value": true },
        { "field": "beneficiary.country", "op": "not_in", "value": ["NG", "GH"] }
      ]
    }
  ]
}`}
      />
      <P>
        In words: a debit of at least 500,000.00 (amounts are in minor units, so
        50,000,000 kobo for naira) to a new beneficiary or to a beneficiary
        outside Nigeria and Ghana.
      </P>
      <H3>Operators</H3>
      <Fields
        label="Operators"
        head={["Field type", "Operators"]}
        rows={[
          ["Numbers", "eq, ne, gt, gte, lt, lte, in, not_in"],
          ["Strings", "eq, ne, in, not_in"],
          ["Booleans", "eq, ne"],
        ]}
      />
      <H3>Rules of the grammar</H3>
      <UL>
        <li>
          No code: no scripting, no expressions, no regular expressions. Unknown
          fields, operators or keys and wrongly typed values are rejected with{" "}
          <C>400 INVALID_REQUEST</C> and the path to the problem, such as{" "}
          <C>condition.all[1]: unknown field …</C>.
        </li>
        <li>
          Bounds: at most 6 levels deep, 50 nodes and 100 items in an <C>in</C>{" "}
          list; at most 200 rules per environment, disabled rules included.
        </li>
        <li>
          <strong>
            A comparison on a fact the event does not have never matches
          </strong>
          : a transaction field on a login, or a feature that could not be
          computed. Combine <C>not</C> with care: <C>not</C> of a missing fact
          is true.
        </li>
        <li>
          Money is in minor units of the event&apos;s currency; compare the
          currency too.
        </li>
        <li>
          Countries and currencies are ISO codes: <C>NG</C>, <C>GB</C>;{" "}
          <C>NGN</C>, <C>USD</C>.
        </li>
      </UL>

      <H2>Fields</H2>
      <P>
        <C>GET /api/v1/risk/catalog</C> returns every field with its type,
        operators, a plain label, group, unit (<C>money</C>, <C>count</C>,{" "}
        <C>ratio</C>), allowed values and the event types that carry it. Build
        tooling from the catalog rather than from this table.
      </P>
      <Fields
        label="Rule fields"
        head={["Field", "Type", "Meaning"]}
        rows={[
          [
            "event.type",
            "string",
            "transaction, login, beneficiary_added, device_change, session_change, profile_change, activity",
          ],
          [
            "context.country",
            "string (country)",
            "Country the event came from.",
          ],
          ["transaction.amount", "number (money)", "Amount, in minor units."],
          ["transaction.currency", "string (currency)", "Currency."],
          [
            "transaction.direction",
            "string",
            "debit (money out) or credit (money in).",
          ],
          [
            "transaction.channel",
            "string",
            "card, bank_transfer, instant_payment, wallet, other",
          ],
          [
            "beneficiary.country",
            "string (country)",
            "Beneficiary's country (transactions, beneficiary_added).",
          ],
          ["login.outcome", "string", "success or failure."],
          ["login.method", "string", "password, passkey, otp, sso, biometric"],
          [
            "profile.credential_change",
            "boolean",
            "Email, phone or password changed (profile_change).",
          ],
          ["feature.new_user", "boolean", "First time this customer is seen."],
          ["feature.new_device", "boolean", "New device for this customer."],
          [
            "feature.new_beneficiary",
            "boolean",
            "New beneficiary for this customer.",
          ],
          ["feature.new_country", "boolean", "New country for this customer."],
          [
            "feature.recent_credential_change",
            "boolean",
            "Email, phone or password changed in the last 24 hours.",
          ],
          [
            "feature.amount_to_mean_ratio",
            "number (ratio)",
            "Amount compared with this customer's usual amount.",
          ],
          [
            "feature.account_txn_1m",
            "number (count)",
            "Transactions by this account in the last minute.",
          ],
          ["feature.account_txn_1h", "number (count)", "… in the last hour."],
          [
            "feature.account_txn_24h",
            "number (count)",
            "… in the last 24 hours.",
          ],
          [
            "feature.failed_logins_15m",
            "number (count)",
            "Failed logins in the last 15 minutes.",
          ],
          [
            "feature.device_users",
            "number (count)",
            "Customers seen on this device.",
          ],
          [
            "feature.beneficiary_senders_24h",
            "number (count)",
            "Customers who paid this beneficiary in the last 24 hours.",
          ],
        ]}
      />

      <H2>Create, test and switch on</H2>
      <OL>
        <li>
          <strong>Try the condition on recent events.</strong> It says how many
          of your recent evaluations it would have matched, with examples,
          before you save anything.
        </li>
        <li>
          <strong>Create it in shadow mode.</strong> It runs on live traffic and
          its matches are recorded, without changing any decision.
        </li>
        <li>
          <strong>Watch its matches</strong> for a few days in the statistics.
        </li>
        <li>
          <strong>Switch it to active</strong> when it matches what you expect.
        </li>
      </OL>
      <H3>Try a condition on recent events</H3>
      <Code
        label="Test a condition on recent events"
        code={`curl ${API}/api/v1/rules/test-recent \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"days": 7, "condition": {"all": [
        {"field": "transaction.amount", "op": "gte", "value": 50000000},
        {"field": "feature.new_beneficiary", "op": "eq", "value": true}]}}'`}
      />
      <Code
        label="Response"
        code={`{
  "since": "2026-09-27T09:00:00Z",
  "checked": 4180,
  "matched": 37,
  "truncated": false,
  "sample": [
    {
      "evaluation_id": "evl_…", "event_id": "evt_…", "at": "2026-10-04T08:12:44Z",
      "event_type": "transaction", "amount": 75000000, "currency": "NGN",
      "decision": "review", "risk_score": 55
    }
  ]
}`}
      />
      <UL>
        <li>
          <C>days</C> is 1 to 30 (default 7). The newest 5,000 evaluations of
          the period are checked; <C>truncated</C> says there were more.{" "}
          <C>sample</C> holds up to 25 matches, newest first.
        </li>
        <li>
          It uses the facts recorded at the time of each evaluation: behaviour
          that has changed since is not recomputed. Nothing is stored and no
          score changes.
        </li>
      </UL>
      <H3>Create</H3>
      <Code
        label="Create a rule"
        code={`curl ${API}/api/v1/rules \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "code": "large_payment_new_payee",
    "name": "Large payment to a new payee",
    "description": "Debits of 500,000 NGN or more to a beneficiary never paid before.",
    "severity": "high",
    "weight": 30,
    "mode": "shadow",
    "condition": {"all": [
      {"field": "transaction.currency", "op": "eq", "value": "NGN"},
      {"field": "transaction.amount", "op": "gte", "value": 50000000},
      {"field": "feature.new_beneficiary", "op": "eq", "value": true}]}
  }'`}
      />
      <P>
        In a test environment this answers <C>201</C> with the rule (version 1).
        In production it answers <C>202</C> with a proposed change: see below.{" "}
        <C>409 CONFLICT</C> means the code is taken or the environment already
        has 200 rules.
      </P>
      <H3>Test against one event</H3>
      <P>
        <C>POST /api/v1/rules/{"{rule_id}"}/test</C> with{" "}
        <C>{`{"event": {...}}`}</C> (an event as you would send it), and
        optionally a draft <C>condition</C>, says whether it matched and the
        values of the fields the condition uses. Live behaviour is read, never
        updated, and nothing is stored.
      </P>
      <H3>Change, roll back</H3>
      <UL>
        <li>
          <C>PATCH /api/v1/rules/{"{rule_id}"}</C> with any of name,
          description, severity, weight, mode, enabled and condition. A patch
          that changes nothing creates no version.
        </li>
        <li>
          <C>GET /api/v1/rules/{"{rule_id}"}/versions</C> lists every version.
        </li>
        <li>
          <C>POST /api/v1/rules/{"{rule_id}"}/rollback</C> with{" "}
          <C>{`{"version": 3}`}</C> adds a new version with version 3&apos;s
          content (<C>restored_from: 3</C>); history is never rewritten.
        </li>
        <li>
          Changes reach every RiskOS instance within 5 seconds. Every change is
          audited.
        </li>
      </UL>

      <H2>Two people in production</H2>
      <P>
        In production, creating, changing or rolling back a rule does not apply
        it: the API answers <C>202</C> with a proposed change (<C>rch_…</C>) and
        the rule keeps its current version.
      </P>
      <UL>
        <li>
          Another person approves it with{" "}
          <C>POST /api/v1/rule-changes/{"{change_id}"}/approve</C>. That needs{" "}
          <C>rules:approve</C>, which only people hold (administrators and risk
          managers); API keys never can, and nobody can approve their own
          proposal.
        </li>
        <li>
          Approval applies the change as the rule&apos;s next version, credited
          to the proposer, and only if the rule has not changed since the
          proposal (<C>409</C> otherwise).
        </li>
        <li>
          <C>POST …/reject</C> rejects it, with an optional reason; the proposer
          can withdraw their own change the same way.{" "}
          <C>GET /api/v1/rule-changes</C> lists proposals.
        </li>
        <li>
          A rule, and a new rule&apos;s code, can each have one pending change
          at a time.
        </li>
        <li>Test environments apply changes at once, for fast iteration.</li>
      </UL>

      <H2>Statistics</H2>
      <P>
        <C>GET /api/v1/rules/stats?days=7</C> (1 to 30) returns, for every rule
        of the environment, how many evaluations it matched in the period
        (active and shadow alike), its last match and a count per UTC day,
        beside the number of evaluations in the period. Dry runs are not
        counted. The console&apos;s rule list and each rule&apos;s Activity tab
        show the same numbers.
      </P>
      <Code
        label="Rule statistics"
        code={`{
  "since": "2026-09-28T00:00:00Z",
  "days": 7,
  "evaluations": 18234,
  "rules": [
    {
      "rule_id": "rul_…",
      "matches": 41,
      "last_matched_at": "2026-10-04T08:12:44Z",
      "daily": [{ "day": "2026-09-28", "matches": 5 }, "…"]
    }
  ]
}`}
      />
      <P>
        Where a rule matched shows in each decision: active rules in{" "}
        <C>signals</C>, shadow rules only in the stored evaluation. Your
        jurisdiction&apos;s preset rules are configured separately; see{" "}
        <DocLink href="/docs/compliance">Compliance</DocLink>. Every endpoint is
        in the <DocLink href="/docs/reference/rules">rules reference</DocLink>.
      </P>
      <Next href="/docs/compliance" label="Compliance" />
    </DocPage>
  );
}
