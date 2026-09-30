// k6 baseline load test for the cheapest endpoint (GET /health/live).
//
// Purpose: measure the fixed cost of the HTTP stack (network, middleware, routing, JSON) so later
// endpoints can be compared against it. Run with `make load` while `make up` is running.
// Results are recorded in docs/PERFORMANCE_BASELINE.md.
import http from "k6/http";
import { check } from "k6";

const API_URL = __ENV.API_URL || "http://127.0.0.1:8080";

export const options = {
  scenarios: {
    constant_rate: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 2000),
      timeUnit: "1s",
      duration: __ENV.DURATION || "30s",
      preAllocatedVUs: 200,
      maxVUs: 1000,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.001"],
    // Guardrail, not the target: the spec's p95 < 200 ms applies to risk evaluation.
    http_req_duration: ["p(95)<50", "p(99)<100"],
  },
  summaryTrendStats: ["avg", "min", "med", "p(90)", "p(95)", "p(99)", "max"],
};

export default function () {
  const res = http.get(`${API_URL}/health/live`);
  check(res, {
    "status is 200": (r) => r.status === 200,
    "has request id": (r) => r.headers["X-Request-Id"] !== undefined,
  });
}
