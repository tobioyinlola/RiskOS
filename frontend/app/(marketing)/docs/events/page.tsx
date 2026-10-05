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

export const metadata: Metadata = { title: "Sending events" };

const KINDS =
  "transfer_outbound, transfer_inbound, transfer_international, cash_payment, cash_withdrawal, cash_deposit, atm_withdrawal, pos_purchase, card_authorization, crypto_purchase, crypto_withdrawal, crypto_transfer, fiat_inflow, funding, loan_disbursement, loan_repayment, login, auth_failure, auth_success, device_bound, device_reregistered, beneficiary_added, session_activity, profile_change, loan_application, mfa_prompt, sim_reregistered, account_opened, account_reactivated, customer_onboarded, customer_review, credit_requested, list_updated, telco_lookup, data_export, api_request, api_lookup, config_change, role_change, staff_action, fraud_report, merchant_daily, terminal_activity";

export default function Page() {
  return (
    <DocPage
      title="Sending events"
      lead="An event tells RiskOS that something happened on your platform. Send one for every transaction, login and account change you want scored; RiskOS evaluates it in the same request and returns the decision."
    >
      <H2>The endpoint</H2>
      <Fields
        label="Event endpoints"
        head={["Endpoint", "Scope", "What it does"]}
        rows={[
          [
            "POST /api/v1/events",
            "events:write",
            "Validate, store and evaluate an event; returns the decision. Idempotent on event_id.",
          ],
          [
            "POST /api/v1/risk/evaluate",
            "risk:evaluate",
            "Evaluate without storing an event (a dry run): what-if checks and integration tests. Never raises alerts, never updates history.",
          ],
          [
            "GET /api/v1/events",
            "events:read",
            "List events newest first, with score, level and decision. Filters: type, decision, risk_level, user, account, device; after returns only newer events.",
          ],
          [
            "GET /api/v1/events/{event_id}",
            "events:read",
            "One event: the raw and normalized payloads and its stored evaluation.",
          ],
          [
            "GET /api/v1/entities/{kind}/{ref}",
            "events:read",
            "A user's, account's or device's profile from its most recent 5,000 events.",
          ],
        ]}
      />
      <P>
        Call <C>POST /api/v1/events</C> synchronously, before you complete the
        action, with a timeout of one to two seconds. Evaluation usually takes a
        few milliseconds; the timeout protects your flow if the network fails.
        Plan a fallback for when no decision arrives (see{" "}
        <DocLink href="/docs/errors">Errors, limits and retries</DocLink>).
      </P>

      <H2>The envelope</H2>
      <P>
        Every event has the same envelope; only <C>data</C> depends on the type.
      </P>
      <Fields
        label="Envelope fields"
        head={["Field", "Required", "Meaning"]}
        rows={[
          [
            "event_id",
            "Yes",
            "Your unique ID for this event, 1 to 128 of A-Z a-z 0-9 . _ : -. It is the idempotency key: retries must reuse it, new events must not.",
          ],
          [
            "type",
            "Yes",
            "login, transaction, beneficiary_added, device_change, session_change, profile_change or activity.",
          ],
          [
            "occurred_at",
            "Yes",
            "When it happened (RFC 3339). At most 5 minutes in the future and 24 hours in the past.",
          ],
          [
            "actor.user_id",
            "Yes",
            "Your reference for the customer. Opaque: never a name, email or phone number.",
          ],
          [
            "actor.account_id",
            "No",
            "Your reference for the account or wallet involved.",
          ],
          [
            "actor.device_id",
            "No",
            "Your reference for the device (an app installation ID, a device fingerprint).",
          ],
          ["actor.session_id", "No", "Your reference for the session."],
          [
            "context.ip",
            "No",
            "The customer's IP address (IPv4 or IPv6), not your server's.",
          ],
          ["context.user_agent", "No", "The customer's user agent."],
          [
            "context.country",
            "No",
            "ISO 3166-1 alpha-2 country, for example NG.",
          ],
          [
            "context.geo",
            "No",
            "{ lat, lon }. Rounded to 2 decimals (about 1 km) before it is stored.",
          ],
          [
            "data",
            "Per type",
            "The event's details; see below. Not used by activity events.",
          ],
          [
            "compliance",
            "No (yes for activity)",
            "What your rule pack needs: kind, facts and refs. See Compliance data.",
          ],
        ]}
      />
      <Callout>
        References (<C>event_id</C>, <C>user_id</C>, <C>account_id</C>,{" "}
        <C>device_id</C>, <C>beneficiary_id</C>…) are your own identifiers. Use
        stable, opaque values: the same customer must always have the same{" "}
        <C>user_id</C>, or history signals cannot work.
      </Callout>

      <H2>Event types</H2>
      <H3>transaction</H3>
      <P>
        A money movement: a transfer, a card payment, a wallet top-up, a
        withdrawal.
      </P>
      <Fields
        label="Transaction data"
        head={["Field", "Required", "Meaning"]}
        rows={[
          ["transaction_id", "Yes", "Your reference for the transaction."],
          [
            "amount",
            "Yes",
            "Integer amount in the currency's minor units (kobo, cents). ₦25,000 is 2500000.",
          ],
          ["currency", "Yes", "ISO 4217 code, for example NGN, USD."],
          [
            "direction",
            "Yes",
            "debit (money leaves the customer) or credit (money arrives).",
          ],
          [
            "beneficiary_id",
            "No",
            "Your reference for the payee. Send it: new_beneficiary and beneficiary_fan_in need it.",
          ],
          [
            "channel",
            "No",
            "card, bank_transfer, instant_payment, wallet or other.",
          ],
        ]}
      />
      <Code
        label="Transaction event"
        code={`{
  "event_id": "txn-000184",
  "type": "transaction",
  "occurred_at": "2026-10-04T09:30:00Z",
  "actor": { "user_id": "usr_4812", "account_id": "acc_4812", "device_id": "dev_9f2", "session_id": "ses_51" },
  "context": { "ip": "102.89.34.7", "country": "NG", "geo": { "lat": 6.52, "lon": 3.38 } },
  "data": {
    "transaction_id": "t_000184",
    "amount": 15000000,
    "currency": "NGN",
    "direction": "debit",
    "beneficiary_id": "ben_77",
    "channel": "instant_payment"
  }
}`}
      />
      <H3>login</H3>
      <Fields
        label="Login data"
        head={["Field", "Required", "Meaning"]}
        rows={[
          [
            "outcome",
            "Yes",
            "success or failure. Send failures too: failed_login_burst counts them.",
          ],
          ["method", "Yes", "password, passkey, otp, sso or biometric."],
        ]}
      />
      <Code
        label="Login event"
        code={`{
  "event_id": "login-88421",
  "type": "login",
  "occurred_at": "2026-10-04T09:28:12Z",
  "actor": { "user_id": "usr_4812", "device_id": "dev_new_31" },
  "context": { "ip": "197.210.55.4", "country": "NG", "user_agent": "BankApp/5.2 (Android 14)" },
  "data": { "outcome": "success", "method": "password" }
}`}
      />
      <H3>beneficiary_added</H3>
      <P>
        A payee added to the customer&apos;s list: <C>beneficiary_id</C>{" "}
        (required) and <C>country</C> (optional, ISO 3166-1 alpha-2).
      </P>
      <H3>device_change</H3>
      <P>
        A new device bound to the account: <C>new_device_id</C> (required) and{" "}
        <C>previous_device_id</C> (optional).
      </P>
      <H3>session_change</H3>
      <P>
        A session created, refreshed or terminated: <C>change</C> is{" "}
        <C>created</C>, <C>refreshed</C> or <C>terminated</C>.
      </P>
      <H3>profile_change</H3>
      <P>
        Profile fields changed: <C>fields</C> lists which of <C>email</C>,{" "}
        <C>phone</C>, <C>address</C>, <C>name</C> and <C>password</C> changed.
        Never send the values. An email, phone or password change arms{" "}
        <C>recent_credential_change</C> for the next 24 hours.
      </P>
      <Code
        label="Profile change event"
        code={`{
  "event_id": "prof-2291",
  "type": "profile_change",
  "occurred_at": "2026-10-04T09:10:00Z",
  "actor": { "user_id": "usr_4812", "device_id": "dev_new_31" },
  "data": { "fields": ["phone", "password"] }
}`}
      />
      <H3>activity</H3>
      <P>
        Anything else your rule pack needs to see: an account opened, a data
        export, an API request, a staff action. Activity events have no{" "}
        <C>data</C>; <C>compliance.kind</C> is required and says what happened.
      </P>
      <Code
        label="Activity event"
        code={`{
  "event_id": "acct-open-5521",
  "type": "activity",
  "occurred_at": "2026-10-04T08:00:00Z",
  "actor": { "user_id": "usr_9001", "account_id": "acc_9001" },
  "compliance": {
    "kind": "account_opened",
    "facts": { "account.has_bvn": false, "customer.type": "individual" }
  }
}`}
      />

      <H2 id="compliance-data">Compliance data</H2>
      <P>
        Every event may carry a <C>compliance</C> block with what your
        workspace&apos;s rule pack needs. All of it is optional: a rule whose
        data is missing is reported in the decision&apos;s <C>not_evaluated</C>{" "}
        list, never treated as &ldquo;no risk&rdquo;. Each preset rule&apos;s
        page in the console lists the facts it reads.
      </P>
      <Fields
        label="Compliance block"
        head={["Field", "Meaning"]}
        rows={[
          [
            "kind",
            "What the event is in the pack's terms. Money kinds go with transaction events; login, auth_failure and auth_success with login; device_bound and device_reregistered with device_change; the rest with activity. Without it, RiskOS derives the kind: a debit is transfer_outbound (card_authorization on the card channel), a credit transfer_inbound, a login login and auth_success or auth_failure.",
          ],
          [
            "facts",
            "Values from your systems and providers, by catalog name: device.rooted, telco.sim_swap, customer.type, customer.is_pep, screening.sanctions_score… Each is true or false, a number of at least 0 (durations in seconds, scores up to 100, hours up to 24), one of the fact's listed values, or { amount, currency } in minor units. At most 64.",
          ],
          [
            "refs",
            "Opaque references for entities rules count over time: card, bvn, phone, address, loan, inflow, merchant, api_key, sender, reporter, funding_source. Never the value itself.",
          ],
        ]}
      />
      <P>The kinds you can send are:</P>
      <P>
        <C>{KINDS}</C>
      </P>
      <Callout tone="warn">
        RiskOS refuses facts it computes itself (behaviour history, workflow
        facts, the event&apos;s own amount and hour) and identifiers sent as
        facts. A <C>bvn</C> reference of 11 digits or a <C>phone</C> reference
        that is a phone number is rejected with <C>422 SENSITIVE_DATA</C>: send
        your own opaque reference instead.
      </Callout>
      <P>
        Send <C>account.has_bvn</C> with an <C>actor.account_id</C> when an
        account is opened and whenever it changes: RiskOS keeps a register of
        which accounts have a BVN (never the BVN) for the daily check of
        accounts without one.
      </P>

      <H2>Idempotency and retries</H2>
      <UL>
        <li>
          <C>event_id</C> is unique per environment. Sending the same event
          again, with the same payload, returns the original decision verbatim
          with the header <C>Idempotent-Replay: true</C>; the event is never
          stored or evaluated twice.
        </li>
        <li>
          Key order and whitespace do not matter: RiskOS compares the normalized
          payload.
        </li>
        <li>
          The same <C>event_id</C> with a different payload is refused with{" "}
          <C>409 IDEMPOTENCY_CONFLICT</C> and nothing is evaluated.
        </li>
        <li>
          So: when a request times out or fails with <C>429</C> or <C>503</C>,
          retry it with the <strong>same</strong> <C>event_id</C> and body.
        </li>
      </UL>

      <H2>Validation</H2>
      <Fields
        label="Validation errors"
        head={["Problem", "Response"]}
        rows={[
          ["Body over 64 KB", "413 PAYLOAD_TOO_LARGE"],
          [
            "Malformed JSON, a missing or mistyped field, an unknown field",
            "400 INVALID_REQUEST, naming the field",
          ],
          [
            "occurred_at more than 5 minutes ahead, an invalid IP, a compliance kind or fact that does not fit",
            "422 INVALID_EVENT",
          ],
          ["occurred_at more than 24 hours old", "422 STALE_EVENT"],
          [
            "A card-number-like value anywhere (13 to 19 digits, Luhn-valid), a raw BVN or phone number as a reference",
            "422 SENSITIVE_DATA; the value is never echoed back",
          ],
          ["Same event_id, different payload", "409 IDEMPOTENCY_CONFLICT"],
        ]}
      />
      <Callout tone="warn">
        Never send card numbers, CVVs, PINs, passwords or one-time codes.
        Numeric customer identifiers that happen to look like card numbers are
        rejected too: prefix them (<C>usr_4111…</C>) or use opaque IDs.
      </Callout>

      <H2>Reading events back</H2>
      <Code
        label="List recent declined or blocked events for a user"
        code={`curl "${API}/api/v1/events?user=usr_4812&decision=decline&limit=20" \\
  -H "Authorization: Bearer $RISKOS_API_KEY"`}
      />
      <P>
        <C>GET /api/v1/events/{"{event_id}"}</C> returns the raw payload as
        received, the normalized payload and the full evaluation (signals,
        controls, the facts it saw), so any decision can be explained later.
      </P>

      <H2>Dry runs</H2>
      <P>
        <C>POST /api/v1/risk/evaluate</C> takes the same event body and returns
        a decision without storing the event, raising alerts or updating
        history. Use it to test your integration or ask &ldquo;what would happen
        if…&rdquo;. Dry runs are not idempotent; each one is a new evaluation
        flagged as a dry run.
      </P>
      <Next href="/docs/decisions" label="Acting on decisions" />
    </DocPage>
  );
}
