#!/usr/bin/env bash
# Restore drill against the running stack (`make up`). Run it while the stack is idle (no load),
# so the backup and the live database can be compared row for row.
#   1. Seeds a tenant with events, then takes a backup: roles (pg_dumpall --roles-only) and the
#      database (pg_dump, custom format).
#   2. Restores both into a fresh PostgreSQL container that shares nothing with the live one.
#   3. Proves the restore: every table's row count matches the live database; every tenant's
#      audit hash chain verifies; row-level security still isolates tenants for the application
#      role; and an API instance on the restored database serves the seeded events with the
#      seeded API key.
# Timings (the recovery time objective's building blocks) go to
# backend/test/load/results/restore-drill.json. Secrets stay in variables and are never echoed.
set -euo pipefail

cd "$(dirname "$0")/.."
set -a; . ./.env; set +a
RESULTS=backend/test/load/results
NETWORK=${COMPOSE_PROJECT_NAME:-riskos}_default
PG_IMAGE=$(docker compose config --images postgres 2>/dev/null | head -1)
RESTORE=riskos-restore-pg
RESTORE_API=riskos-restore-api
RESTORE_API_PORT=${RESTORE_API_PORT:-18080}
WORK=$(mktemp -d)
mkdir -p "$RESULTS"
cleanup() { docker rm -f "$RESTORE" "$RESTORE_API" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT

log() { printf '%s  %s\n' "$(date -u +%T)" "$*"; }
now_ms() { local us=${EPOCHREALTIME/./}; echo $((us / 1000)); } # bash's clock: date's %3N is not portable
src() { docker compose exec -T postgres "$@"; }
dst() { docker exec -i "$RESTORE" "$@"; }
count_tables() { # $1: runner (src|dst); prints "table count" for every base table in public
  local run=$1 tables
  tables=$($run psql -U riskos -d riskos -Atc "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY 1")
  for t in $tables; do printf '%s %s\n' "$t" "$($run psql -U riskos -d riskos -Atc "SELECT count(*) FROM \"$t\"")"; done
}
ten_id() { python3 -c 'import sys,uuid; n=int.from_bytes(uuid.UUID(sys.argv[1]).bytes,"big")<<2; a="0123456789abcdefghjkmnpqrstvwxyz"; print("ten_"+"".join(a[(n>>(5*(25-i)))&31] for i in range(26)))' "$1"; }

# ---- seed ---------------------------------------------------------------------------------
name="Restore drill $(date -u +%FT%TZ)"
out=$(docker compose --profile tools run --rm --build riskosctl tenant create --operator drill@riskos.local \
  --name "$name" --region eu-west-1 2>/dev/null)
key=$(grep -o 'rsk_test_[0-9a-z_]*' <<<"$out" | head -1)
[[ -n "$key" ]] || { echo "restore-drill: could not create a tenant" >&2; exit 1; }
tenant=$(src psql -U riskos -d riskos -Atc "SELECT id FROM tenants WHERE name = '$name'")
for i in $(seq 1 25); do
  curl -fsS -o /dev/null -X POST "http://127.0.0.1:${API_PORT:-8080}/api/v1/events" \
    -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
    -d "{\"event_id\":\"restore-$i\",\"type\":\"login\",\"occurred_at\":\"$(date -u +%FT%TZ)\",\"actor\":{\"user_id\":\"u-$i\"},\"data\":{\"outcome\":\"success\",\"method\":\"password\"}}"
done
log "seeded tenant $tenant with 25 events"
sleep 3 # let background jobs from the seed settle before counting

# ---- backup -------------------------------------------------------------------------------
t0=$(now_ms)
src pg_dumpall -U riskos --roles-only >"$WORK/roles.sql"
src pg_dump -U riskos -d riskos -Fc >"$WORK/riskos.dump"
t1=$(now_ms)
count_tables src >"$WORK/live-counts"
dump_bytes=$(stat -c %s "$WORK/riskos.dump")
log "backup: $((t1 - t0)) ms, $dump_bytes bytes"

# ---- restore into a fresh server ----------------------------------------------------------
docker run -d --name "$RESTORE" --network "$NETWORK" -e POSTGRES_USER=riskos -e POSTGRES_DB=riskos \
  -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" "$PG_IMAGE" >/dev/null
until dst pg_isready -U riskos -d riskos -q 2>/dev/null && dst psql -U riskos -d riskos -Atc 'SELECT 1' >/dev/null 2>&1; do sleep 1; done
t2=$(now_ms)
# The owner role already exists in a fresh server; every other role is created from the backup.
dst psql -U riskos -d postgres -q -v ON_ERROR_STOP=0 <"$WORK/roles.sql" >/dev/null 2>&1 || true
dst pg_restore -U riskos -d riskos --exit-on-error <"$WORK/riskos.dump"
t3=$(now_ms)
log "restore: $((t3 - t2)) ms"

# ---- verify -------------------------------------------------------------------------------
count_tables dst >"$WORK/restored-counts"
tables=$(wc -l <"$WORK/live-counts")
mismatch=$(diff "$WORK/live-counts" "$WORK/restored-counts" || true)
[[ -z "$mismatch" ]] && counts_ok=true || counts_ok=false

restored_url="postgres://riskos:$POSTGRES_PASSWORD@$RESTORE:5432/riskos?sslmode=disable"
app_url="postgres://riskos_app:$POSTGRES_APP_PASSWORD@$RESTORE:5432/riskos?sslmode=disable"
chains_ok=true; chains=0
for t in $(dst psql -U riskos -d riskos -Atc "SELECT id FROM tenants ORDER BY created_at"); do
  chains=$((chains + 1))
  if ! docker compose --profile tools run --rm -e RISKOS_POSTGRES_URL="$app_url" \
      -e RISKOS_POSTGRES_MIGRATION_URL="$restored_url" riskosctl audit verify --tenant "$(ten_id "$t")" 2>/dev/null | grep -q '^OK'; then
    chains_ok=false; log "audit chain FAILED for tenant $t"
  fi
done

env_id=$(dst psql -U riskos -d riskos -Atc "SELECT id FROM environments WHERE tenant_id = '$tenant' AND kind = 'test'")
rls_q() { dst env PGPASSWORD="$POSTGRES_APP_PASSWORD" psql -h 127.0.0.1 -U riskos_app -d riskos -At "$@"; }
own=$(rls_q -c "SELECT set_config('app.tenant_id', '$tenant', false), set_config('app.environment_id', '$env_id', false)" \
  -c "SELECT count(*) FROM events" | tail -1)
unscoped=$(rls_q -c "SELECT count(*) FROM events")
[[ "$own" == "25" && "$unscoped" == "0" ]] && rls_ok=true || rls_ok=false

docker compose run -d --rm --no-deps --name "$RESTORE_API" -p "127.0.0.1:$RESTORE_API_PORT:8080" \
  -e RISKOS_POSTGRES_URL="$app_url" api >/dev/null
api_ok=false
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:$RESTORE_API_PORT/health/ready" >/dev/null 2>&1; then
    served=$(curl -fsS "http://127.0.0.1:$RESTORE_API_PORT/api/v1/events?limit=100" -H "Authorization: Bearer $key" |
      python3 -c 'import json,sys; print(len(json.load(sys.stdin)["data"]))')
    [[ "$served" == "25" ]] && api_ok=true
    break
  fi
  sleep 1
done
t4=$(now_ms)
log "verify: $((t4 - t3)) ms"

python3 - "$RESULTS/restore-drill.json" <<EOF
import json, sys
checks = {
    "row counts match the live database ($tables tables)": $( $counts_ok && echo True || echo False ),
    "every audit chain verifies ($chains tenants)": $( $chains_ok && echo True || echo False ),
    "row-level security isolates tenants": $( $rls_ok && echo True || echo False ),
    "the API serves the restored data": $( $api_ok && echo True || echo False ),
}
report = {
    "backup_ms": $((t1 - t0)), "restore_ms": $((t3 - t2)), "verify_ms": $((t4 - t3)),
    "backup_bytes": $dump_bytes, "tables": $tables, "tenants": $chains,
    "count_mismatches": """$mismatch""".splitlines(),
    "checks": checks,
}
json.dump(report, open(sys.argv[1], "w"), indent=2)
print(json.dumps(report, indent=2))
print("PASSED" if all(checks.values()) else "FAILED: " + ", ".join(k for k, ok in checks.items() if not ok))
sys.exit(0 if all(checks.values()) else 1)
EOF
