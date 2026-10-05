import type { Metadata } from "next";

import {
  C,
  Callout,
  Code,
  DocPage,
  Fields,
  H2,
  H3,
  Next,
  OL,
  P,
  Samples,
  UL,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE as API } from "@/lib/docs";

export const metadata: Metadata = { title: "Webhooks" };

export default function Page() {
  return (
    <DocPage
      title="Webhooks"
      lead="Webhooks tell your systems when alerts and cases change, so your case tools, chat channels and customer messaging stay in step with your analysts. Every delivery is signed, retried and logged."
    >
      <Callout>
        Decisions are not sent as webhooks: <C>POST /api/v1/events</C> already
        returns them in the same request.
      </Callout>

      <H2>Event types</H2>
      <Fields
        label="Webhook event types"
        head={["Type", "Sent when", "data"]}
        rows={[
          [
            "alert.created",
            "An alert is raised: risk, security, or a webhook endpoint disabled for failing.",
            "alert",
          ],
          [
            "alert.updated",
            "An alert's status changes: triage, escalation into a case, closure with a case.",
            "alert, previous_status",
          ],
          ["case.created", "A case is opened.", "case, alert_ids"],
          ["case.resolved", "A case is resolved.", "case"],
          [
            "webhook.test",
            "You asked for a test (POST /api/v1/webhooks/{id}/test), to that endpoint only.",
            "message",
          ],
        ]}
      />
      <P>
        <C>alert</C> and <C>case</C> have exactly the shape the API returns from{" "}
        <C>GET /api/v1/alerts/{"{id}"}</C> and <C>GET /api/v1/cases/{"{id}"}</C>
        . Every body is an envelope:
      </P>
      <Code
        label="Webhook body"
        code={`{
  "id": "wev_06gf7h0q2v8k4c1d9e3f5g7h1j",
  "type": "alert.created",
  "created_at": "2026-10-04T09:31:02.123Z",
  "environment": "production",
  "data": {
    "alert": {
      "id": "alr_06gf7h0q2v8k4c1d9e3f5g7h1k",
      "kind": "risk",
      "severity": "critical",
      "status": "open",
      "title": "...",
      "...": "..."
    }
  }
}`}
      />
      <P>
        An event is recorded in the same database transaction as the change that
        caused it: it is sent only if the change happened, and a change never
        misses its event.
      </P>

      <H2>Register an endpoint</H2>
      <Code
        label="Register an endpoint"
        code={`curl ${API}/api/v1/webhooks \\
  -H "Authorization: Bearer $RISKOS_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "url": "https://hooks.example.com/riskos",
    "event_types": ["alert.created", "alert.updated", "case.created", "case.resolved"],
    "description": "Fraud ops tooling"
  }'`}
      />
      <UL>
        <li>
          The response contains the endpoint and its{" "}
          <strong>signing secret, shown once</strong>. Store it in your secrets
          manager.
        </li>
        <li>
          At most 20 endpoints per environment. Test and production have
          separate endpoints, secrets and deliveries.
        </li>
        <li>
          Scopes: <C>webhooks:read</C> and <C>webhooks:write</C>. Every change
          is audited.
        </li>
        <li>
          <C>PATCH /api/v1/webhooks/{"{id}"}</C> changes the URL, description,
          subscriptions or status (<C>active</C>, <C>disabled</C>).
        </li>
        <li>
          <C>POST /api/v1/webhooks/{"{id}"}/test</C> sends a <C>webhook.test</C>{" "}
          event to check your receiver.
        </li>
      </UL>
      <H3>URL requirements</H3>
      <P>
        Only <C>https</C> URLs, without credentials, on public addresses:
        private, loopback, link-local, metadata and other reserved addresses are
        refused, also when a hostname only resolves to one. Redirects are never
        followed, and TLS 1.2 is the minimum. When you run RiskOS locally,
        test-environment endpoints may use <C>http://</C> and private addresses
        so you can receive webhooks on your machine; production endpoints never
        can.
      </P>

      <H2>Verify every request</H2>
      <Fields
        label="Webhook headers"
        head={["Header", "Value"]}
        rows={[
          [
            "RiskOS-Webhook-Id",
            "The event ID (wev_…): the same for every endpoint and every retry.",
          ],
          ["RiskOS-Webhook-Type", "The event type."],
          [
            "RiskOS-Webhook-Delivery",
            "This delivery's ID (whd_…), for support.",
          ],
          [
            "RiskOS-Webhook-Timestamp",
            "Unix seconds when this attempt was signed.",
          ],
          [
            "RiskOS-Signature",
            "One or more v1=<hex> values, comma-separated (two during a secret rotation).",
          ],
        ]}
      />
      <P>
        <C>v1</C> is the hex HMAC-SHA256, keyed by the endpoint&apos;s secret,
        of <C>{"<RiskOS-Webhook-Id>.<RiskOS-Webhook-Timestamp>.<raw body>"}</C>.
        Your receiver must:
      </P>
      <OL>
        <li>
          recompute it over the <strong>raw</strong> body bytes (before any JSON
          parsing) and accept if it equals any <C>v1</C> value, with a
          constant-time comparison;
        </li>
        <li>reject timestamps more than 5 minutes from its clock;</li>
        <li>
          process each <C>RiskOS-Webhook-Id</C> once: retries and redeliveries
          reuse it;
        </li>
        <li>
          answer any <C>2xx</C> quickly and do slow work afterwards.
        </li>
      </OL>
      <Samples
        label="Verify a webhook"
        samples={[
          {
            lang: "Node.js",
            code: `const crypto = require("crypto");

function verify(secret, headers, rawBody, toleranceSec = 300) {
  const ts = headers["riskos-webhook-timestamp"];
  if (Math.abs(Date.now() / 1000 - Number(ts)) > toleranceSec) return false;
  const want = "v1=" + crypto.createHmac("sha256", secret)
    .update(\`\${headers["riskos-webhook-id"]}.\${ts}.\`).update(rawBody).digest("hex");
  return headers["riskos-signature"].split(",").some(
    (s) => s.trim().length === want.length &&
      crypto.timingSafeEqual(Buffer.from(s.trim()), Buffer.from(want)));
}

// Express: keep the raw body for verification.
app.post("/riskos", express.raw({ type: "application/json" }), (req, res) => {
  if (!verify(process.env.RISKOS_WEBHOOK_SECRET, req.headers, req.body)) {
    return res.sendStatus(400);
  }
  res.sendStatus(204);                 // answer first
  queue.add(JSON.parse(req.body));     // then process, once per RiskOS-Webhook-Id
});`,
          },
          {
            lang: "Python",
            code: `import hashlib, hmac, time

def verify(secret: str, headers, body: bytes, tolerance=300) -> bool:
    ts = headers["RiskOS-Webhook-Timestamp"]
    if abs(time.time() - int(ts)) > tolerance:
        return False
    msg = f'{headers["RiskOS-Webhook-Id"]}.{ts}.'.encode() + body
    want = "v1=" + hmac.new(secret.encode(), msg, hashlib.sha256).hexdigest()
    return any(hmac.compare_digest(want, s.strip())
               for s in headers["RiskOS-Signature"].split(","))`,
          },
          {
            lang: "Go",
            code: `func Verify(secret string, h http.Header, body []byte, now time.Time) bool {
	ts := h.Get("RiskOS-Webhook-Timestamp")
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || math.Abs(now.Sub(time.Unix(sec, 0)).Seconds()) > 300 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h.Get("RiskOS-Webhook-Id") + "." + ts + "."))
	mac.Write(body)
	want := []byte("v1=" + hex.EncodeToString(mac.Sum(nil)))
	for _, s := range strings.Split(h.Get("RiskOS-Signature"), ",") {
		if hmac.Equal([]byte(strings.TrimSpace(s)), want) {
			return true
		}
	}
	return false
}`,
          },
        ]}
      />

      <H2>Rotating the secret</H2>
      <P>
        <C>POST /api/v1/webhooks/{"{id}"}/rotate-secret</C> issues a new secret,
        shown once. For <C>overlap_hours</C> (default 24, at most 168)
        deliveries carry signatures from both the new and the previous secret,
        so you can deploy the new one without dropping deliveries.
      </P>

      <H2>Delivery and retries</H2>
      <UL>
        <li>
          Each attempt times out after 10 seconds. Any <C>2xx</C> is success;
          everything else, including redirects, is a failure.
        </li>
        <li>
          A delivery gets 9 attempts over about an hour: retries after 10 s, 30
          s, 1 min, 2 min, 5 min, 10 min, 20 min and 20 min, each with ±20%
          jitter. Then the delivery is <C>failed</C>.
        </li>
        <li>
          <C>GET /api/v1/webhooks/{"{id}"}/deliveries</C> lists deliveries; each
          one shows its attempts (time, duration, status code, a short error)
          and the exact payload sent.
        </li>
        <li>
          <C>POST …/deliveries/{"{delivery_id}"}/redeliver</C> starts a new
          cycle for a failed delivery, with the same event ID. Response bodies
          from your endpoint are never stored.
        </li>
        <li>
          An endpoint whose every attempt has failed for 3 days is disabled, its
          pending deliveries fail, and a security alert is raised. <C>PATCH</C>{" "}
          it with <C>{`{"status": "active"}`}</C> to re-enable it.
        </li>
      </UL>
      <Next href="/docs/rules" label="Rules" />
    </DocPage>
  );
}
