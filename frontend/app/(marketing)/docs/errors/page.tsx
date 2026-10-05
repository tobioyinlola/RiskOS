import type { Metadata } from "next";

import {
  C,
  Callout,
  Code,
  DocPage,
  Fields,
  H2,
  Next,
  OL,
  P,
  Samples,
} from "@/features/docs/ui";
import { API_URL_EXAMPLE as API } from "@/lib/docs";

export const metadata: Metadata = { title: "Errors, limits and retries" };

export default function Page() {
  return (
    <DocPage
      title="Errors, limits and retries"
      lead="Every error has the same shape and a stable code. Most failures are safe to retry, and events are idempotent, so a careful client never loses an event and never sends one twice."
    >
      <H2>The error contract</H2>
      <Code
        label="An error"
        code={`HTTP/1.1 422 Unprocessable Entity
X-Request-ID: req_01j9x2k7m3q4r5s6t7v8w9x0yz

{
  "error": {
    "code": "STALE_EVENT",
    "message": "occurred_at is more than 24 hours old.",
    "request_id": "req_01j9x2k7m3q4r5s6t7v8w9x0yz"
  }
}`}
      />
      <P>
        Switch on <C>code</C>, which never changes; <C>message</C> is safe to
        log and show to your engineers, and never contains secrets or internals.
        Every response, successful or not, carries <C>X-Request-ID</C>.
      </P>

      <H2>Codes</H2>
      <Fields
        label="Error codes"
        head={["Code", "Status", "Meaning and what to do", "Retry"]}
        rows={[
          [
            "INVALID_REQUEST",
            "400",
            "The request does not match the contract: malformed JSON, a missing, mistyped or unknown field. The message names the field. Fix the request.",
            "No",
          ],
          [
            "UNAUTHENTICATED",
            "401",
            "No valid key: missing, malformed, unknown, expired or revoked. Check your key configuration.",
            "No",
          ],
          [
            "FORBIDDEN",
            "403",
            "The key lacks the scope, or the operation needs a person (approvals, the compliance profile).",
            "No",
          ],
          [
            "TENANT_SUSPENDED",
            "403",
            "The workspace is suspended. Contact RiskOS.",
            "No",
          ],
          [
            "NOT_FOUND",
            "404",
            "No such resource in this environment. Resources of other workspaces are also simply not found.",
            "No",
          ],
          [
            "CONFLICT",
            "409",
            "The resource changed or is in a state that forbids the change (an approved report, a pending rule change).",
            "After re-reading",
          ],
          [
            "IDEMPOTENCY_CONFLICT",
            "409",
            "This event_id was already used with a different payload. Use a new event_id for a new event.",
            "No",
          ],
          ["PAYLOAD_TOO_LARGE", "413", "The body is over 64 KB.", "No"],
          [
            "INVALID_EVENT",
            "422",
            "The event is well-formed but not acceptable: occurred_at too far ahead, an invalid IP, a compliance kind or fact that does not fit.",
            "No",
          ],
          [
            "STALE_EVENT",
            "422",
            "occurred_at is more than 24 hours old.",
            "No",
          ],
          [
            "SENSITIVE_DATA",
            "422",
            "A card number or a raw BVN or phone number was found. The value is never echoed. Remove it.",
            "No",
          ],
          [
            "RATE_LIMITED",
            "429",
            "Too many requests for this key or workspace. Wait Retry-After seconds.",
            "Yes",
          ],
          [
            "SERVICE_UNAVAILABLE",
            "503",
            "A dependency is down; nothing was stored. Wait Retry-After seconds.",
            "Yes",
          ],
          [
            "TIMEOUT",
            "503",
            "The request took too long; nothing was stored.",
            "Yes",
          ],
        ]}
      />

      <H2>Rate limits</H2>
      <P>
        Requests are limited per API key and per workspace with a token bucket,
        so short bursts are absorbed. A RiskOS deployment&apos;s defaults are
        100 requests per second per key (bursts up to 200) and 500 per second
        per workspace (bursts up to 1,000); your deployment may set others. Over
        the limit you get <C>429 RATE_LIMITED</C> with <C>Retry-After</C> in
        seconds. If you need more, use separate keys per service or ask for a
        higher limit.
      </P>

      <H2>Retrying safely</H2>
      <OL>
        <li>
          Retry on <C>429</C>, <C>503</C>, a timeout or a network error. Never
          retry other <C>4xx</C>: they will fail the same way.
        </li>
        <li>
          Retry an event with the <strong>same</strong> <C>event_id</C> and
          body. If the first attempt was stored after all, you get the stored
          decision with <C>Idempotent-Replay: true</C>; the event is never
          stored twice.
        </li>
        <li>
          Wait at least <C>Retry-After</C> seconds when present; otherwise back
          off exponentially, with random jitter so your instances do not retry
          in lockstep after an outage.
        </li>
        <li>
          Bound the total time to what your payment flow can wait (one or two
          seconds in a checkout), then fall back.
        </li>
      </OL>
      <Samples
        label="Send an event with retries"
        samples={[
          {
            lang: "Node.js",
            code: `const RETRYABLE = new Set([429, 503, 504]);

export async function decide(event, { deadlineMs = 2000 } = {}) {
  const end = Date.now() + deadlineMs;
  for (let attempt = 0; ; attempt++) {
    try {
      const res = await fetch("${API}/api/v1/events", {
        method: "POST",
        headers: {
          Authorization: \`Bearer \${process.env.RISKOS_API_KEY}\`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(event), // the same event_id on every attempt
        signal: AbortSignal.timeout(Math.max(100, end - Date.now())),
      });
      if (res.ok) return await res.json();
      if (!RETRYABLE.has(res.status)) {
        const { error } = await res.json();
        throw new Error(\`RiskOS \${error.code}: \${error.message} (\${error.request_id})\`);
      }
      const after = Number(res.headers.get("Retry-After") ?? 0) * 1000;
      await wait(Math.max(after, backoff(attempt)), end);
    } catch (e) {
      if (e.name !== "TimeoutError" && e.name !== "TypeError") throw e;
      await wait(backoff(attempt), end);
    }
  }
}

const backoff = (n) => Math.min(1000, 50 * 2 ** n) * (0.5 + Math.random());
async function wait(ms, end) {
  if (Date.now() + ms >= end) throw new Error("RiskOS: no decision in time");
  await new Promise((r) => setTimeout(r, ms));
}`,
          },
          {
            lang: "Python",
            code: `import os, random, time, requests

RETRYABLE = {429, 503, 504}

def decide(event, deadline_s=2.0):
    end = time.monotonic() + deadline_s
    attempt = 0
    while True:
        try:
            r = requests.post(
                "${API}/api/v1/events",
                headers={"Authorization": f"Bearer {os.environ['RISKOS_API_KEY']}"},
                json=event,  # the same event_id on every attempt
                timeout=max(0.1, end - time.monotonic()),
            )
            if r.ok:
                return r.json()
            if r.status_code not in RETRYABLE:
                err = r.json()["error"]
                raise RuntimeError(f"RiskOS {err['code']}: {err['message']} ({err['request_id']})")
            delay = float(r.headers.get("Retry-After", 0))
        except (requests.Timeout, requests.ConnectionError):
            delay = 0
        delay = max(delay, min(1.0, 0.05 * 2 ** attempt) * random.uniform(0.5, 1.5))
        if time.monotonic() + delay >= end:
            raise TimeoutError("RiskOS: no decision in time")
        time.sleep(delay)
        attempt += 1`,
          },
        ]}
      />

      <H2>When no decision arrives</H2>
      <P>
        Decide in advance what your platform does when RiskOS cannot answer in
        time, and keep it conservative for money leaving the customer: for
        example, hold large debits for review and allow small ones, or queue the
        event and decide asynchronously. RiskOS fails fast rather than queueing:
        a database outage answers <C>503</C> within about a second, and an
        outage of the behaviour store still returns decisions, marked degraded
        and failing closed.
      </P>
      <Callout>
        After an outage, keep retrying with the same <C>event_id</C> in the
        background so RiskOS still sees the event: its history, alerts and
        reports depend on it.
      </Callout>
      <Next href="/docs/webhooks" label="Webhooks" />
    </DocPage>
  );
}
