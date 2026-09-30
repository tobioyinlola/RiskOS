#!/usr/bin/env bash
# Browser test against the running stack (`make up`) with no mocks: creates a tenant and an
# invite with riskosctl, then runs frontend/tests/e2e/stack.spec.ts through the real console
# and API. Secrets printed by riskosctl (the tenant's first API key, the invite link) stay in
# this script's variables and are never echoed.
set -euo pipefail

cd "$(dirname "$0")/.."
compose=(docker compose)
ctl=("${compose[@]}" --profile tools run --rm --build riskosctl)
operator="e2e@riskos.local"
email="e2e-$(date +%s)@riskos.local"

tenant_out=$("${ctl[@]}" tenant create --operator "$operator" --name "E2E $(date -u +%FT%TZ)" --region eu-west-1 2>/dev/null)
tenant=$(grep -o 'ten_[0-9a-z]*' <<<"$tenant_out" | head -1)
[[ -n "$tenant" ]] || { echo "e2e-stack: could not create a tenant" >&2; exit 1; }

invite_out=$("${ctl[@]}" user invite --operator "$operator" --tenant "$tenant" --email "$email" 2>/dev/null)
invite=$(grep -o 'http[^ ]*accept-invite[^ ]*' <<<"$invite_out" | head -1)
[[ -n "$invite" ]] || { echo "e2e-stack: could not create an invite" >&2; exit 1; }

echo "e2e-stack: tenant $tenant, user $email"
cd frontend
E2E_STACK_URL="http://localhost:${FRONTEND_PORT:-3000}" E2E_INVITE_URL="$invite" E2E_EMAIL="$email" \
  pnpm exec playwright test --project desktop
