#!/usr/bin/env bash
# Rejects files larger than the limit (default 1 MiB). Lockfiles are exempt: they are large by
# nature and reviewed through dependency tooling. So is generated code (*.gen.go, *.gen.ts,
# *.gen.json), which grows with the API contract and is checked by `make generate-check`, which
# fails when it is not exactly what its source produces.
# Usage: check-large-files.sh [files...]   (defaults to all tracked files)
set -euo pipefail

limit="${MAX_FILE_BYTES:-1048576}"
if [[ $# -eq 0 ]]; then
  mapfile -t files < <(git ls-files)
else
  files=("$@")
fi

status=0
for f in "${files[@]}"; do
  [[ -f "$f" ]] || continue
  case "$f" in
    */pnpm-lock.yaml | */go.sum | *.gen.go | *.gen.ts | *.gen.json) continue ;;
  esac
  size=$(stat -c %s "$f")
  if (( size > limit )); then
    echo "file too large: $f ($size bytes > $limit)" >&2
    status=1
  fi
done
exit $status
