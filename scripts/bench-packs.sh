#!/usr/bin/env bash
# Decision-path load with each country's rule pack against the running stack (`make up`):
# docs/PERFORMANCE_BASELINE.md §7. Creates one workspace per case (no pack; Nigeria, bank; Ghana,
# mobile money issuer; Rwanda, e-money issuer), each with exchange rates so US dollar thresholds
# are evaluated, then for each run: warms the workspace for WARM_S seconds at 100 events/s, resets
# Redis's command statistics, and runs k6 at RATE for DURATION. Prints one line per run: latency
# percentiles from k6 and Redis script time per event. Raise RISKOS_RATE_LIMIT_KEY_PER_SECOND
# above RATE first. Secrets printed by riskosctl stay in variables and are never echoed.
set -euo pipefail

cd "$(dirname "$0")/.."
RATE=${RATE:-100}
DURATION=${DURATION:-30s}
WARM_S=${WARM_S:-60}
ACCOUNTS=${ACCOUNTS:-20000}
ROUNDS=${ROUNDS:-2}
CASES=${CASES:-"none NG GH RW"}
API=http://127.0.0.1:${API_PORT:-8080}
RESULTS=backend/test/load/results
K6_IMAGE=grafana/k6:2.3.0@sha256:9c2dee7f8ed74d317e4027c06a10f169b625638189de8d4555d0b3486a5aeb34
mkdir -p "$RESULTS"

compose() { docker compose "$@"; }
psql_q() { compose exec -T postgres psql -U riskos -d riskos -Atc "$1"; }
log() { printf '%s  %s\n' "$(date -u +%T)" "$*" >&2; }

# workspace creates a tenant for a case and prints its test key.
workspace() {
  local c=$1 name out key tenant
  name="Bench $c $(date -u +%FT%TZ)"
  out=$(compose --profile tools run --rm --build riskosctl tenant create --operator bench@riskos.local \
    --name "$name" --region eu-west-1 2>/dev/null)
  key=$(grep -o 'rsk_test_[0-9a-z_]*' <<<"$out" | head -1)
  [[ -n "$key" ]] || { echo "bench-packs: could not create a tenant" >&2; exit 1; }
  tenant=$(psql_q "SELECT id FROM tenants WHERE name = '$name'")
  psql_q "INSERT INTO compliance_fx_rates (tenant_id, currency, usd_per_unit, updated_at, updated_by) VALUES
    ('$tenant', 'NGN', 0.00065, now(), 'bench'), ('$tenant', 'GHS', 0.08, now(), 'bench'),
    ('$tenant', 'RWF', 0.00072, now(), 'bench')" >/dev/null
  case $c in
    none) psql_q "DELETE FROM jurisdiction_activations WHERE tenant_id = '$tenant'" >/dev/null ;;
    NG) psql_q "UPDATE jurisdiction_activations SET licence_type = 'bank', signal_default = 'pack'
          WHERE tenant_id = '$tenant' AND fallback" >/dev/null ;;
    GH | RW)
      local pack entity
      pack=$(tr '[:upper:]' '[:lower:]' <<<"$c")
      entity="$pack-bench"
      psql_q "INSERT INTO jurisdiction_activations (id, tenant_id, jurisdiction, pack_id, pack_version, licence_type,
          capabilities, booking_entity, status, fallback, signal_default, updated_at, updated_by, activated_at, activated_by)
        SELECT gen_random_uuid(), '$tenant', '$c', p.id, p.version, 'emi', '{}', '$entity', 'live', false, 'pack',
          now(), 'bench', now(), 'bench'
        FROM rule_packs p WHERE p.id = '$pack' ORDER BY string_to_array(p.version, '.')::int[] DESC LIMIT 1" >/dev/null ;;
  esac
  echo "$key"
}

k6() {
  local key=$1 pack=$2 country=$3 rate=$4 duration=$5 out=$6
  docker run --rm --network host --user "$(id -u):$(id -g)" -v "$PWD/backend/test/load:/scripts" \
    -e API_URL="$API" -e API_KEY="$key" -e RATE="$rate" -e DURATION="$duration" -e PACK="$pack" \
    -e COUNTRY="$country" -e ACCOUNTS="$ACCOUNTS" "$K6_IMAGE" \
    run --quiet --summary-export="/scripts/results/$out" /scripts/events.js >/dev/null 2>&1 || true
}

script_usec() {
  compose exec -T redis redis-cli INFO commandstats | tr -d '\r' |
    sed -n 's/^cmdstat_evalsha:calls=[0-9]*,usec=\([0-9]*\),.*/\1/p'
}

declare -A KEYS_BY_CASE
for c in $CASES; do KEYS_BY_CASE[$c]=$(workspace "$c"); log "workspace for $c ready"; done

printf '%-5s %-5s %8s %8s %8s %8s %8s %8s %10s\n' case round rate achieved med p90 p95 p99 redis_ms
for round in $(seq 1 "$ROUNDS"); do
  for c in $CASES; do
    key=${KEYS_BY_CASE[$c]}
    pack=1 country=$c
    [[ $c == none ]] && pack=0 country=NG
    log "$c round $round: warming ${WARM_S}s"
    k6 "$key" "$pack" "$country" 100 "${WARM_S}s" "bench-warm.json"
    compose exec -T redis redis-cli CONFIG RESETSTAT >/dev/null
    out="bench-$c-$round.json"
    k6 "$key" "$pack" "$country" "$RATE" "$DURATION" "$out"
    usec=$(script_usec)
    python3 - "$RESULTS/$out" "$c" "$round" "$RATE" "${usec:-0}" <<'PY'
import json, sys
path, case, rnd, rate, usec = sys.argv[1:]
m = json.load(open(path))["metrics"]
d, reqs = m["http_req_duration"], m["http_reqs"]
n = reqs["count"]
print(f"{case:<5} {rnd:<5} {rate:>8} {reqs['rate']:>8.0f} {d['med']:>8.1f} {d['p(90)']:>8.1f} {d['p(95)']:>8.1f} "
      f"{d['p(99)']:>8.1f} {int(usec) / n / 1000:>10.2f}  failed={m['http_req_failed']['value']:.4f}")
PY
  done
done
