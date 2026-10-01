// k6 load for the failure drill (scripts/failure-drill.sh): steady transaction ingest while the
// drill stops and restarts Redis, PostgreSQL, the worker and the API.
//
// Each iteration behaves like a well-behaved client: it retries a failed request with the SAME
// event_id (idempotent) with backoff for up to about a minute. The drill then checks, against the
// database, that every event is stored exactly once, that nothing was lost, and that no degraded
// transaction was allowed.
import http from "k6/http";
import { sleep } from "k6";
import { Counter } from "k6/metrics";

const API_URL = __ENV.API_URL || "http://127.0.0.1:8080";
const API_KEY = __ENV.API_KEY;
if (!API_KEY) throw new Error("API_KEY is required");
const RUN = Date.now().toString(36);
const BACKOFF = [0.25, 0.5, 1, 1, 2, 2, 3, 3, 5, 5, 5, 5, 5, 5, 5, 5];

const accepted = new Counter("events_accepted");
const lost = new Counter("events_lost");
const retried = new Counter("events_retried");
const replays = new Counter("events_replayed");
const degraded = new Counter("decisions_degraded");
const unsafe = new Counter("decisions_degraded_allowed");
const statuses = {};
for (const s of ["0", "429", "500", "502", "503", "504", "other"]) {
  statuses[s] = new Counter(`failed_status_${s}`);
}

export const options = {
  scenarios: {
    ingest: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 100),
      timeUnit: "1s",
      duration: __ENV.DURATION || "4m",
      preAllocatedVUs: 200,
      maxVUs: 5000,
    },
  },
  thresholds: {
    events_lost: ["count==0"],
    decisions_degraded_allowed: ["count==0"],
  },
  summaryTrendStats: ["avg", "med", "p(95)", "p(99)", "max"],
};

export default function () {
  const id = `${RUN}-${__VU}-${__ITER}`;
  const body = JSON.stringify({
    event_id: `drill-${id}`,
    type: "transaction",
    occurred_at: new Date().toISOString(),
    actor: { user_id: `u-${__VU % 500}`, account_id: `acc-${__VU % 500}`, device_id: `dev-${__VU % 500}` },
    context: { ip: "203.0.113.7", country: "NG" },
    data: {
      transaction_id: `t-${id}`,
      amount: 250000,
      currency: "NGN",
      direction: "debit",
      beneficiary_id: "ben-1",
      channel: "bank_transfer",
    },
  });
  const params = {
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${API_KEY}` },
    timeout: "10s",
  };
  let wait = 0;
  for (let attempt = 0; attempt <= BACKOFF.length; attempt++) {
    if (attempt > 0) {
      if (attempt === 1) retried.add(1);
      // Honour Retry-After when given, and add jitter so clients do not retry in lockstep
      // (API.md: retries).
      sleep(Math.max(wait, BACKOFF[attempt - 1]) * (0.5 + Math.random()));
    }
    const res = http.post(`${API_URL}/api/v1/events`, body, params);
    wait = Number(res.headers["Retry-After"] || 0);
    if (res.status === 200) {
      accepted.add(1);
      if (res.headers["Idempotent-Replay"] === "true") replays.add(1);
      const d = res.json();
      if (d.degraded_signals && d.degraded_signals.length > 0) {
        degraded.add(1);
        // Transactions fail closed by default: a degraded evaluation is at least REVIEW.
        if (d.decision === "allow") unsafe.add(1);
      }
      return;
    }
    (statuses[String(res.status)] || statuses.other).add(1);
    // A client error other than rate limiting will not succeed on retry.
    if (res.status >= 400 && res.status < 500 && res.status !== 429) break;
  }
  lost.add(1);
}
