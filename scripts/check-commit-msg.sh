#!/usr/bin/env bash
# Enforces Conventional Commits on the subject line: type(scope)!: description
set -euo pipefail

subject="$(head -n1 "$1")"
types="feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|security"
pattern="^(${types})(\([a-z0-9._/-]+\))?!?: [^ ].{0,99}$"

# Allow git-generated merge and revert subjects.
if [[ "$subject" =~ ^(Merge|Revert\ \") ]]; then
  exit 0
fi

if ! [[ "$subject" =~ $pattern ]]; then
  cat >&2 <<EOF
Commit subject does not follow Conventional Commits:
  $subject

Expected: <type>(<optional scope>): <description>   (max 100 chars after the type)
Types:    ${types//|/, }
Example:  feat(events): validate event timestamps
EOF
  exit 1
fi
