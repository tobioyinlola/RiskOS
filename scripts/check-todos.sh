#!/usr/bin/env bash
# Rejects TODO comments that do not name the phase resolving them: TODO(phase-N).
# Go code is covered by golangci-lint (godox); this covers everything else that is committed.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
if git grep -nP 'TODO(?!\(phase-\d+\))' -- ':!docs/spec/**' ':!*.go' ':!scripts/check-todos.sh' ':!CONTRIBUTING.md' ':!**/pnpm-lock.yaml' ':!backend/.golangci.yml' ':!frontend/eslint.config.mjs'; then
  echo "found TODO without a phase; write TODO(phase-N): …" >&2
  exit 1
fi
