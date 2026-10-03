// k6 load test for the synchronous decision path: POST /api/v1/events.
//
// Each iteration ingests a new transaction event (unique event_id), so every request does the
// full path: authenticate, rate limit, contract validation, normalization, idempotency claim,
// evaluation, and one transaction writing the event, the evaluation and the idempotency record.
// Run with `make load-events API_KEY=rsk_test_...` against `make up`.
//
// PACK=1 adds the compliance block a rule pack reads (customer profile, device and screening
// facts), for a key whose workspace has a compliance profile: every request then also runs the
// pack's rules, with their Redis time windows and profiles (docs/PERFORMANCE_BASELINE.md §5).
import http from "k6/http";
import { check } from "k6";

const API_URL = __ENV.API_URL || "http://127.0.0.1:8080";
const API_KEY = __ENV.API_KEY;
if (!API_KEY) {
  throw new Error("API_KEY is required (a key with the events:write scope)");
}
const RUN = Date.now().toString(36);
const PACK = __ENV.PACK === "1";
// ACCOUNTS spreads the load over that many customers (default: one per VU, so every customer is
// busy); production traffic at a few hundred events per second comes from thousands.
const ACCOUNTS = Number(__ENV.ACCOUNTS || 0);

export const options = {
  scenarios: {
    ingest: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 500), // design peak: 500 events/s (ARCHITECTURE.md §2)
      timeUnit: "1s",
      duration: __ENV.DURATION || "30s",
      preAllocatedVUs: 100,
      maxVUs: 1000,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.001"],
    // Spec §20: core evaluation p95 < 200 ms. Internal target 60 ms (ARCHITECTURE.md §1).
    http_req_duration: ["p(95)<200"],
  },
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

function actor() {
  const n = ACCOUNTS > 0 ? (__VU * 7919 + __ITER) % ACCOUNTS : __VU;
  return { user_id: `u-${n}`, account_id: `acc-${n}`, device_id: `dev-${n}` };
}

export default function () {
  const id = `${RUN}-${__VU}-${__ITER}`;
  const body = JSON.stringify({
    event_id: `load-${id}`,
    type: "transaction",
    occurred_at: new Date().toISOString(),
    actor: actor(),
    context: { ip: "203.0.113.7", country: "NG" },
    data: {
      transaction_id: `t-${id}`,
      // From 2,500 to 250,000 NGN, so thresholds and baselines see realistic spread.
      amount: PACK ? 250000 + ((__ITER * 7919) % 100) * 247500 : 250000,
      currency: "NGN",
      direction: "debit",
      beneficiary_id: "ben-1",
      channel: "bank_transfer",
    },
    ...(PACK
      ? {
          compliance: {
            facts: {
              "customer.type": "individual",
              "account.age": 400 * 86400,
              "customer.is_pep": false,
              "device.is_bound_device": true,
              "device.rooted": false,
              "telco.sim_swap": false,
              "screening.sanctions_score": 3,
            },
          },
        }
      : {}),
  });
  const res = http.post(`${API_URL}/api/v1/events`, body, {
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${API_KEY}` },
  });
  check(res, {
    "status is 200": (r) => r.status === 200,
    // With a pack, controls may hold or limit a payment; the decision is still given.
    "has a decision": (r) =>
      r.status === 200 && (PACK ? !!r.json("decision") : r.json("decision") === "allow"),
  });
}
