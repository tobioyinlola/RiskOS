#!/usr/bin/env bash
# pre-push guard: changes reach `main` only through a pull request on GitHub.
# Git passes "<local ref> <local sha> <remote ref> <remote sha>" lines on stdin.
set -euo pipefail

while read -r _local_ref _local_sha remote_ref _remote_sha; do
  if [[ "$remote_ref" == "refs/heads/main" ]]; then
    cat >&2 <<'EOF'
Direct pushes to main are not allowed.
Push to staging (git push origin staging) and open a pull request staging -> main on GitHub.
EOF
    exit 1
  fi
done
