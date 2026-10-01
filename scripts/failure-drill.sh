#!/usr/bin/env bash
# Failure drill against the running stack (`make up`): steady ingest while Redis, PostgreSQL, the
# worker and the API fail in turn, then checks against the database that
#   - every accepted event is stored exactly once and no event was lost (clients retry with the
#     same event_id);
#   - no degraded transaction was allowed (fail closed);
#   - every evaluation that should raise an alert did, exactly once per kind;
#   - every alert webhook was delivered, correctly signed, and all background jobs finished.
# Timeline and results: backend/test/load/results/failure-drill.json. Secrets printed by
# riskosctl stay in variables and are never echoed.
set -euo pipefail

cd "$(dirname "$0")/.."
RATE=${RATE:-80} # under the local per-key limit (100/s), so retries are not throttled
DURATION_S=${DURATION_S:-240}
DRAIN_TIMEOUT_S=${DRAIN_TIMEOUT_S:-600}
API=http://127.0.0.1:${API_PORT:-8080}
RESULTS=backend/test/load/results
K6_IMAGE=grafana/k6:2.3.0@sha256:9c2dee7f8ed74d317e4027c06a10f169b625638189de8d4555d0b3486a5aeb34
SINK_IMAGE=python:3.13-alpine
NETWORK=${COMPOSE_PROJECT_NAME:-riskos}_default
mkdir -p "$RESULTS"

compose() { docker compose "$@"; }
psql_q() { compose exec -T postgres psql -U riskos -d riskos -Atc "$1"; }
log() { printf '%s  %s\n' "$(date -u +%T)" "$*"; }
START=$(date +%s)
TIMELINE=()
mark() { TIMELINE+=("{\"t\":$(($(date +%s) - START)),\"event\":\"$1\"}"); log "$1"; }

# ---- setup -------------------------------------------------------------------------------
name="Failure drill $(date -u +%FT%TZ)"
tenant_out=$(compose --profile tools run --rm --build riskosctl tenant create --operator drill@riskos.local \
  --name "$name" --region eu-west-1 2>/dev/null)
key=$(grep -o 'rsk_test_[0-9a-z_]*' <<<"$tenant_out" | head -1)
[[ -n "$key" ]] || { echo "failure-drill: could not create a tenant" >&2; exit 1; }
tenant=$(psql_q "SELECT id FROM tenants WHERE name = '$name'")
env=$(psql_q "SELECT id FROM environments WHERE tenant_id = '$tenant' AND kind = 'test'")
log "tenant $tenant (test environment $env)"

sink=riskos-drill-sink
docker rm -f "$sink" >/dev/null 2>&1 || true
secret=$(curl -fsS -X POST "$API/api/v1/webhooks" -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
  -d "{\"url\":\"http://$sink:9099/hook\",\"event_types\":[\"alert.created\",\"alert.updated\"]}" | \
  python3 -c 'import json,sys; print(json.load(sys.stdin)["secret"])')
docker run -d --name "$sink" --network "$NETWORK" -e WEBHOOK_SECRET="$secret" \
  -v "$PWD/scripts/failure/webhook_sink.py:/sink.py:ro" "$SINK_IMAGE" python /sink.py >/dev/null
trap 'docker rm -f "$sink" >/dev/null 2>&1 || true' EXIT
sink_stats() { docker exec "$sink" python -c 'import urllib.request; print(urllib.request.urlopen("http://127.0.0.1:9099/").read().decode())'; }
sleep 2

# ---- load with faults --------------------------------------------------------------------
log "load: $RATE events/s for ${DURATION_S}s"
docker run --rm --network host --user "$(id -u):$(id -g)" -v "$PWD/backend/test/load:/scripts" \
  -e API_URL="$API" -e API_KEY="$key" -e RATE="$RATE" -e DURATION="${DURATION_S}s" "$K6_IMAGE" \
  run --quiet --summary-export=/scripts/results/failure-drill-k6.json /scripts/failure.js \
  >"$RESULTS/failure-drill-k6.log" 2>&1 &
k6_pid=$!
at() { sleep $(($1 - ($(date +%s) - START))) 2>/dev/null || true; }

at 20;  mark "redis stopped";          compose stop redis >/dev/null 2>&1
at 50;  mark "redis started";          compose start redis >/dev/null 2>&1
at 80;  mark "postgres stopped";       compose stop postgres >/dev/null 2>&1
at 95;  mark "postgres started";       compose start postgres >/dev/null 2>&1
at 125; mark "worker killed (SIGKILL)"; compose kill worker >/dev/null 2>&1
at 155; mark "worker started";         compose start worker >/dev/null 2>&1
at 185; mark "api restarted (SIGTERM)"; compose restart api >/dev/null 2>&1
k6_status=0
wait "$k6_pid" || k6_status=$?
mark "load finished (k6 exit $k6_status)"

# ---- drain -------------------------------------------------------------------------------
pending_sql="SELECT
  (SELECT count(*) FROM river_job WHERE state IN ('available','running','retryable','scheduled')
     AND created_at > to_timestamp($START)) +
  (SELECT count(*) FROM webhook_deliveries WHERE tenant_id = '$tenant' AND status = 'pending')"
until [[ "$(psql_q "$pending_sql")" == "0" ]]; do
  (( $(date +%s) - START < DURATION_S + DRAIN_TIMEOUT_S )) || { mark "drain timed out"; break; }
  sleep 5
done
mark "background work drained"

# ---- checks ------------------------------------------------------------------------------
metric() { python3 -c "import json,sys; m=json.load(open('$RESULTS/failure-drill-k6.json'))['metrics']; print(int(m.get(sys.argv[1],{}).get('count',0)))" "$1"; }
accepted=$(metric events_accepted); lost=$(metric events_lost); unsafe=$(metric decisions_degraded_allowed)
degraded=$(metric decisions_degraded); retried=$(metric events_retried); replayed=$(metric events_replayed)
stored=$(psql_q "SELECT count(*) FROM events WHERE tenant_id = '$tenant'")
distinct=$(psql_q "SELECT count(DISTINCT event_key) FROM events WHERE tenant_id = '$tenant'")
expected_alerts=$(psql_q "SELECT
  (SELECT count(*) FROM evaluations WHERE tenant_id = '$tenant' AND NOT dry_run AND decision IN ('review','block')) +
  (SELECT count(*) FROM evaluations WHERE tenant_id = '$tenant' AND NOT dry_run AND cardinality(degraded_signals) > 0)")
alerted=$(psql_q "SELECT count(*) FROM alert_evaluations WHERE tenant_id = '$tenant'")
discarded=$(psql_q "SELECT count(*) FROM river_job WHERE state = 'discarded' AND created_at > to_timestamp($START)")
deliveries=$(psql_q "SELECT count(*) FROM webhook_deliveries WHERE tenant_id = '$tenant'")
delivered=$(psql_q "SELECT count(*) FROM webhook_deliveries WHERE tenant_id = '$tenant' AND status = 'succeeded'")
stats=$(sink_stats)

python3 - "$RESULTS/failure-drill.json" <<EOF
import json, sys
stats = json.loads('''$stats''')
timeline = [$(IFS=,; echo "${TIMELINE[*]}")]
k6 = json.load(open("$RESULTS/failure-drill-k6.json"))["metrics"]
failed = {k.removeprefix("failed_status_"): int(v.get("count", 0)) for k, v in k6.items() if k.startswith("failed_status_")}
checks = {
    "no event lost": $lost == 0,
    "every accepted event stored exactly once": $accepted == $stored == $distinct,
    "no degraded transaction allowed": $unsafe == 0,
    "every alert-raising evaluation alerted once per kind": $expected_alerts == $alerted,
    "no background job discarded": $discarded == 0,
    "every webhook delivered": $deliveries == $delivered,
    "receiver saw every webhook": stats["unique"] == $deliveries,
    "every webhook correctly signed": stats["bad_signature"] == 0,
}
report = {
    "rate": $RATE, "duration_s": $DURATION_S, "timeline": timeline,
    "events": {"accepted": $accepted, "stored": $stored, "lost": $lost, "retried": $retried,
               "idempotent_replays": $replayed, "failed_attempts_by_status": failed},
    "decisions": {"degraded": $degraded, "degraded_allowed": $unsafe},
    "alerts": {"expected": $expected_alerts, "raised": $alerted},
    "webhooks": {"deliveries": $deliveries, "succeeded": $delivered, "received_unique": stats["unique"],
                 "received_duplicates": stats["duplicates"], "bad_signatures": stats["bad_signature"]},
    "jobs": {"discarded": $discarded},
    "latency_ms": {k: round(v, 1) for k, v in k6["http_req_duration"].items() if k != "thresholds"},
    "checks": checks,
}
json.dump(report, open(sys.argv[1], "w"), indent=2)
print(json.dumps({k: v for k, v in report.items() if k != "timeline"}, indent=2))
print("PASSED" if all(checks.values()) else "FAILED: " + ", ".join(k for k, ok in checks.items() if not ok))
sys.exit(0 if all(checks.values()) else 1)
EOF
