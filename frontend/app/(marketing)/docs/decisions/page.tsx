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

export const metadata: Metadata = { title: "Acting on decisions" };

export default function Page() {
  return (
    <DocPage
      title="Acting on decisions"
      lead="Every event returns a decision: what RiskOS recommends you do, how risky the event looked, and why. Your platform decides what each recommendation does; this page says what each one means."
    >
      <H2>The decision response</H2>
      <Fields
        label="Decision fields"
        head={["Field", "Meaning"]}
        rows={[
          [
            "decision",
            "The recommended action (see below). The most severe of the score policy and any regulatory control that applied.",
          ],
          [
            "risk_score",
            "0 to 100: the sum of the signals' weights, capped at 100.",
          ],
          [
            "risk_level",
            "The band of the score: minimal, low, elevated, high or critical.",
          ],
          [
            "signals",
            "Why: each signal that fired, with its code, label and the points (weight) it added. Your own rules appear here under their codes.",
          ],
          [
            "controls",
            "Regulatory controls of your rule pack that applied, with the action the law requires and the work they create.",
          ],
          [
            "not_evaluated",
            "Rules of your pack that could not run because data or a setting they need was missing, and what was missing.",
          ],
          [
            "degraded_signals",
            "Signals that could not be computed this time (a dependency did not answer in time). Never guessed.",
          ],
          [
            "warming_signals",
            "History signals held back while history rebuilds after a reset.",
          ],
          [
            "event_id, evaluation_id, request_id",
            "IDs to look the event and its evaluation up later, and to quote to support.",
          ],
          [
            "engine_version",
            "The version of the scoring engine that produced the decision, for reproducibility.",
          ],
        ]}
      />

      <H2>Decision values</H2>
      <P>
        From least to most severe. The score policy produces <C>allow</C>,{" "}
        <C>step_up</C>, <C>review</C> and <C>block</C>; regulatory controls of
        your rule pack add the others. Clients that switch on the decision must
        handle every value, and treat an unknown value as <C>review</C>, never
        as <C>allow</C>.
      </P>
      <Fields
        label="Decision values"
        head={["Decision", "Meaning", "What your platform does"]}
        rows={[
          ["allow", "No action needed.", "Complete the action."],
          [
            "fee",
            "A regulatory control requires a fee.",
            "Complete it and charge the fee in the control's fee_percent.",
          ],
          [
            "flag",
            "A control requires follow-up but not a stop.",
            "Complete it and do the follow-up the control names (its tasks).",
          ],
          [
            "step_up",
            "Risky enough to confirm it is really the customer.",
            "Ask for a second factor (OTP, biometric, passkey), then complete. Only logins and transactions are stepped up.",
          ],
          [
            "review",
            "An analyst should look before it completes.",
            "Hold it and let your team decide in the RiskOS alert queue (or your own process).",
          ],
          [
            "restrict",
            "A control limits the account until a requirement is met.",
            "Limit the account (for example inflows only) until it is met.",
          ],
          [
            "hold",
            "A control requires the funds to be held for review.",
            "Hold the funds for at most the control's hold_seconds.",
          ],
          [
            "block_session",
            "The session looks taken over.",
            "End the session and require fresh authentication.",
          ],
          [
            "decline",
            "A legal limit stops this transaction.",
            "Decline the transaction; the customer may retry within the limit.",
          ],
          [
            "block",
            "Stop the event: a deny-list match, or a critical score with automatic blocking on.",
            "Decline and do not retry.",
          ],
        ]}
      />

      <H2>Scores, bands and the default policy</H2>
      <Fields
        label="Bands"
        head={["Band", "Default scores", "Default decision"]}
        rows={[
          ["minimal", "0 to 19", "allow"],
          [
            "low",
            "20 to 49",
            "allow, or step_up when a strengthenable signal fired (new_device, geo_anomaly, recent_credential_change)",
          ],
          ["elevated", "50 to 69", "review"],
          [
            "high",
            "70 to 89",
            "step_up (review for event types that cannot be stepped up)",
          ],
          [
            "critical",
            "90 to 100",
            "review; block only when automatic blocking is on",
          ],
        ]}
      />
      <P>
        The thresholds (20/50/70/90) and signal weights can be changed per
        environment by your risk team. <C>GET /api/v1/risk/policy</C> returns
        the environment&apos;s current bands, thresholds and default decisions,
        so your systems never hard-code them.
      </P>
      <P>
        <strong>Automatic blocking</strong> is off by default: critical scores
        get <C>review</C>. Your risk team can run it in shadow mode, where
        evaluations are only marked <C>would_block</C>, and switch it on after
        14 days in shadow. A <strong>deny list</strong> match (a user, account,
        device, IP address, beneficiary or country your team listed) always
        gives <C>block</C>.
      </P>

      <H2>Built-in signals</H2>
      <Fields
        label="Built-in signals"
        head={["Signal", "Default weight", "Fires when"]}
        rows={[
          ["new_device", "20", "The customer's device was never seen before."],
          [
            "new_beneficiary",
            "15",
            "The customer never paid this beneficiary before.",
          ],
          [
            "amount_anomaly",
            "24",
            "A debit is above the mean plus 3 standard deviations and above twice the mean of the customer's amounts in that currency (with at least 5 prior transactions).",
          ],
          [
            "velocity_anomaly",
            "15",
            "More than 3 transactions from the account in the current minute, or more than 10 in the last hour.",
          ],
          [
            "geo_anomaly",
            "10",
            "The event comes from a country never seen for the customer.",
          ],
          [
            "failed_login_burst",
            "20",
            "5 or more failed logins for the customer in 15 minutes.",
          ],
          [
            "shared_device",
            "15",
            "5 or more distinct customers on the device.",
          ],
          [
            "recent_credential_change",
            "20",
            "A debit within 24 hours of an email, phone or password change.",
          ],
          [
            "beneficiary_fan_in",
            "15",
            "10 or more distinct customers paid the beneficiary in about 24 hours.",
          ],
          [
            "new_user",
            "5",
            "The customer has no history yet (on a first event, the only history signal).",
          ],
        ]}
      />
      <P>
        No single built-in signal decides alone. <C>GET /api/v1/risk/catalog</C>{" "}
        returns each signal with its effective weight in your environment.
      </P>

      <H2>Regulatory controls</H2>
      <P>
        When your workspace&apos;s rule pack has a control for the event (a
        reporting threshold, a legal limit, a hold), it appears in{" "}
        <C>controls</C>:
      </P>
      <Code
        label="A decision with a control"
        code={`{
  "risk_score": 22,
  "risk_level": "low",
  "decision": "decline",
  "signals": [{ "code": "new_device", "label": "New device", "weight": 20 }],
  "controls": [
    {
      "rule": "REG-01",
      "name": "New-device limit",
      "mode": "live",
      "decision": "decline",
      "reason": "new_device_or_account_cap",
      "tasks": [],
      "exemption_unknown": false
    }
  ],
  "not_evaluated": [
    { "rule": "ATO-04", "missing": ["fact:telco.sim_swap"] }
  ],
  "...": "..."
}`}
      />
      <UL>
        <li>
          A <C>live</C> control&apos;s decision is part of <C>decision</C>; a{" "}
          <C>shadow</C> control is reported only, so your compliance team can
          see what it would do.
        </li>
        <li>
          <C>tasks</C> name the work the control creates for your institution (
          <C>report.FTR</C>, <C>edd</C>, <C>case.open</C>…). RiskOS records it
          in the compliance work queue with its legal deadline; you do not need
          to act on it in your payment flow.
        </li>
        <li>
          <C>fee_percent</C> (for <C>fee</C>) and <C>hold_seconds</C> (for{" "}
          <C>hold</C>) give the amounts the law sets.
        </li>
        <li>
          <C>exemption_unknown: true</C> means the control applied because its
          exemption could not be checked: send the fact the exemption needs to
          avoid it.
        </li>
      </UL>

      <H2>Missing and degraded data</H2>
      <H3>not_evaluated</H3>
      <P>
        Each entry names a rule and what it was missing:{" "}
        <C>fact:&lt;name&gt;</C> (send it in <C>compliance.facts</C>),{" "}
        <C>setting:&lt;name&gt;</C> (your compliance team sets it),{" "}
        <C>history:&lt;window&gt;</C> (behaviour history or a reference the
        window needs) or <C>currency:&lt;code&gt;</C> (an exchange rate your
        team adds). Missing data never counts as no risk; it is listed so you
        can close the gap.
      </P>
      <H3>degraded_signals</H3>
      <P>
        If part of RiskOS cannot answer in time, the affected signals are listed
        instead of guessed, and the decision follows your environment&apos;s
        fail policy: by default at least <C>review</C> for everything except
        logins (fail closed), and the decision as it stands for logins (fail
        open). High-value events always get at least <C>review</C> while
        anything is degraded. Degraded evaluations also raise a security alert.
      </P>
      <H3>warming_signals</H3>
      <P>
        After RiskOS rebuilds its behaviour history, signals that would mislead
        are held back for a warm-up period and listed here. Warming does not
        trigger the fail policy.
      </P>
      <Callout>
        Store <C>evaluation_id</C> with your transaction. It lets your support
        and risk teams find the exact evaluation, its signals and the data it
        saw.
      </Callout>
      <P>
        Analysts&apos; work on risky decisions is described in{" "}
        <DocLink href="/docs/alerts-and-cases">Alerts and cases</DocLink>.
      </P>
      <Next href="/docs/errors" label="Errors, limits and retries" />
    </DocPage>
  );
}
