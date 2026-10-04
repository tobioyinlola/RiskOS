import type { Metadata } from "next";

import {
  C,
  Callout,
  Code,
  DocLink,
  DocPage,
  H2,
  Next,
  OL,
  P,
  Samples,
  UL,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE as API } from "@/lib/docs";

export const metadata: Metadata = { title: "Quickstart" };

const EVENT = `{
  "event_id": "txn-2026-10-04-000001",
  "type": "transaction",
  "occurred_at": "2026-10-04T09:30:00Z",
  "actor": { "user_id": "usr_4812", "account_id": "acc_4812", "device_id": "dev_9f2" },
  "context": { "ip": "102.89.34.7", "country": "NG" },
  "data": {
    "transaction_id": "t_000001",
    "amount": 2500000,
    "currency": "NGN",
    "direction": "debit",
    "beneficiary_id": "ben_77",
    "channel": "bank_transfer"
  }
}`;

export default function Page() {
  return (
    <DocPage
      title="Quickstart"
      lead="Send your first event to RiskOS and act on its decision. You need a RiskOS workspace and about ten minutes. Everything here happens in the test environment, which never affects production."
    >
      <H2>1. Get a workspace</H2>
      <P>
        Create one at <DocLink href="/signup">Create a workspace</DocLink>, or
        ask your administrator to invite you. The <strong>test</strong>{" "}
        environment is ready at once. The production environment of a self-serve
        workspace opens after RiskOS approves its compliance onboarding; see{" "}
        <DocLink href="/docs/going-live">Going live</DocLink>.
      </P>

      <H2>2. Create a test API key</H2>
      <OL>
        <li>
          Sign in to the console and choose the <strong>Test</strong>{" "}
          environment at the top.
        </li>
        <li>
          Open <strong>Developer → API keys</strong> and select{" "}
          <strong>Create key</strong>.
        </li>
        <li>
          Name it (for example &ldquo;payments backend&rdquo;) and give it the
          scopes it needs. To send events and read them back:{" "}
          <C>events:write</C> and <C>events:read</C>.
        </li>
        <li>
          Copy the key (<C>rsk_test_…</C>). It is shown once and never again;
          store it in your secrets manager, never in source code.
        </li>
      </OL>
      <P>Check that it works:</P>
      <Code
        label="Check the key"
        code={`curl ${API}/api/v1/whoami \\
  -H "Authorization: Bearer $RISKOS_API_KEY"`}
      />
      <P>
        The answer names your workspace (<C>tenant_id</C>), the environment (
        <C>test</C>), the key and its scopes.
      </P>

      <H2>3. Send your first event</H2>
      <P>
        A transaction event says who did what: the customer (<C>actor</C>, using
        your own opaque references, never names or emails), where from (
        <C>context</C>) and the payment (<C>data</C>, with the amount in minor
        units: 2,500,000 kobo is ₦25,000).
      </P>
      <Samples
        label="Send an event"
        samples={[
          {
            lang: "curl",
            code: `curl ${API}/api/v1/events \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${EVENT.replace(/\n\s*/g, " ")}'`,
          },
          {
            lang: "Node.js",
            code: `const res = await fetch("${API}/api/v1/events", {
  method: "POST",
  headers: {
    Authorization: \`Bearer \${process.env.RISKOS_API_KEY}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify(${EVENT.split("\n").join("\n  ")}),
  signal: AbortSignal.timeout(2000),
});
if (!res.ok) throw new Error(\`RiskOS \${res.status}: \${await res.text()}\`);
const decision = await res.json();
console.log(decision.decision, decision.risk_score);`,
          },
          {
            lang: "Python",
            code: `import os, requests

event = ${EVENT.replace(/true/g, "True")}

r = requests.post(
    "${API}/api/v1/events",
    headers={"Authorization": f"Bearer {os.environ['RISKOS_API_KEY']}"},
    json=event,
    timeout=2,
)
r.raise_for_status()
decision = r.json()
print(decision["decision"], decision["risk_score"])`,
          },
          {
            lang: "Go",
            code: `body := strings.NewReader(\`${EVENT}\`)
req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
	"${API}/api/v1/events", body)
req.Header.Set("Authorization", "Bearer "+os.Getenv("RISKOS_API_KEY"))
req.Header.Set("Content-Type", "application/json")

client := &http.Client{Timeout: 2 * time.Second}
res, err := client.Do(req)
if err != nil {
	return err
}
defer res.Body.Close()
var decision struct {
	Decision  string \`json:"decision"\`
	RiskScore int    \`json:"risk_score"\`
}
if err := json.NewDecoder(res.Body).Decode(&decision); err != nil {
	return err
}`,
          },
        ]}
      />
      <Callout tone="warn">
        <C>occurred_at</C> must be within the last 24 hours and at most 5
        minutes in the future. Use the current time when you try this, and a new{" "}
        <C>event_id</C> for every new event.
      </Callout>

      <H2>4. Read the decision</H2>
      <P>
        The same response carries the decision. A customer&apos;s first event
        has no history yet, so only the <C>new_user</C> signal fires:
      </P>
      <Code
        label="The decision"
        code={`{
  "request_id": "req_01j9x2k7m3q4r5s6t7v8w9x0yz",
  "event_id": "evt_06gg9w2n4k8p0d3e7f1h5j9m2q",
  "evaluation_id": "evl_06gg9w2n4k8p0d3e7f1h5j9m2r",
  "risk_score": 5,
  "risk_level": "minimal",
  "decision": "allow",
  "engine_version": "risk-engine-2.0.0",
  "signals": [
    { "code": "new_user", "label": "New user with no history", "weight": 5 }
  ],
  "degraded_signals": [],
  "warming_signals": [],
  "controls": [],
  "not_evaluated": []
}`}
      />
      <UL>
        <li>
          <C>decision</C> is what RiskOS recommends; <C>risk_score</C> (0 to
          100) and <C>risk_level</C> say how risky the event looked.
        </li>
        <li>
          <C>signals</C> explain the score: each one adds its <C>weight</C> in
          points.
        </li>
        <li>
          <C>controls</C> lists regulatory controls of your rule pack that
          applied (a reporting threshold, a legal limit); <C>not_evaluated</C>{" "}
          lists rules that needed data you did not send.
        </li>
      </UL>

      <H2>5. Act on it</H2>
      <P>
        Handle every decision value; new values only ever add stricter actions.
      </P>
      <Code
        label="Handle the decision"
        code={`switch (decision.decision) {
  case "allow":
  case "fee":
  case "flag":
    return complete(payment);          // fee and flag: complete, then do what the control says
  case "step_up":
    return askForSecondFactor(payment);
  case "review":
  case "hold":
    return holdForReview(payment);     // your analysts decide in the RiskOS console
  case "restrict":
    return limitAccount(payment);
  case "block_session":
    return endSessionAndReauthenticate();
  case "decline":
  case "block":
    return decline(payment);
  default:
    return holdForReview(payment);     // never fail open on a value you do not know
}`}
      />
      <P>
        <DocLink href="/docs/decisions">Acting on decisions</DocLink> explains
        each value and the regulatory controls behind <C>fee</C>, <C>hold</C>{" "}
        and <C>decline</C>.
      </P>

      <H2>6. See it in the console</H2>
      <P>
        Open <strong>Live monitor</strong> or <strong>Events</strong> in the
        console: your event is there with its score, signals and the full
        payload. Send a few more with the same <C>user_id</C> and different
        beneficiaries, devices and amounts, and watch history signals such as{" "}
        <C>new_beneficiary</C> and <C>velocity_anomaly</C> appear.
      </P>

      <H2>Next steps</H2>
      <UL>
        <li>
          Send <DocLink href="/docs/events">logins and account changes</DocLink>{" "}
          too: account takeover shows up there first.
        </li>
        <li>
          Make your client <DocLink href="/docs/errors">retry safely</DocLink>{" "}
          with the same <C>event_id</C>.
        </li>
        <li>
          <DocLink href="/docs/webhooks">Receive webhooks</DocLink> when alerts
          and cases change.
        </li>
        <li>
          Ask your compliance team to confirm your institution type and activate
          the <DocLink href="/docs/compliance">preset rules</DocLink> you need.
        </li>
      </UL>
      <Next href="/docs/authentication" label="Authentication and keys" />
    </DocPage>
  );
}
