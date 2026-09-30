#!/usr/bin/env bash
# Installs pinned, checksum-verified developer CLIs into ./bin (git-ignored).
# Go-based generators (oapi-codegen, sqlc, goose, govulncheck, goimports) are not installed here:
# they are pinned in backend/tools/go.mod and run with `go tool`.
#
# Versions are the single source of truth for local use; CI pins the same versions.
set -euo pipefail

GOLANGCI_LINT_VERSION=2.14.0
GITLEAKS_VERSION=8.30.1
LEFTHOOK_VERSION=2.1.15

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/bin"
mkdir -p "$BIN"

case "$(uname -s)/$(uname -m)" in
  Linux/x86_64) ;;
  *) echo "install-tools.sh supports linux/amd64 only (use WSL on Windows); got $(uname -s)/$(uname -m)" >&2; exit 1 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# fetch URL FILE: download into $tmp.
fetch() { curl -fsSL --retry 3 -o "$tmp/$2" "$1"; }

# verify FILE CHECKSUMS_FILE: check FILE against its line in CHECKSUMS_FILE.
verify() {
  (cd "$tmp" && grep -E "[[:space:]]\*?$1\$" "$2" | sha256sum -c --status -) ||
    { echo "checksum mismatch for $1" >&2; exit 1; }
}

installed_version() { [[ -x "$BIN/$1" ]] && "$BIN/$1" "${@:2}" 2>/dev/null || true; }

if ! installed_version golangci-lint version | grep -q "$GOLANGCI_LINT_VERSION"; then
  base="https://github.com/golangci/golangci-lint/releases/download/v$GOLANGCI_LINT_VERSION"
  f="golangci-lint-$GOLANGCI_LINT_VERSION-linux-amd64.tar.gz"
  fetch "$base/$f" "$f"
  fetch "$base/golangci-lint-$GOLANGCI_LINT_VERSION-checksums.txt" sums.txt
  verify "$f" sums.txt
  tar -xzf "$tmp/$f" -C "$tmp"
  install -m 0755 "$tmp/golangci-lint-$GOLANGCI_LINT_VERSION-linux-amd64/golangci-lint" "$BIN/golangci-lint"
fi

if ! installed_version gitleaks version | grep -q "$GITLEAKS_VERSION"; then
  base="https://github.com/gitleaks/gitleaks/releases/download/v$GITLEAKS_VERSION"
  f="gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
  fetch "$base/$f" "$f"
  fetch "$base/gitleaks_${GITLEAKS_VERSION}_checksums.txt" sums.txt
  verify "$f" sums.txt
  tar -xzf "$tmp/$f" -C "$tmp" gitleaks
  install -m 0755 "$tmp/gitleaks" "$BIN/gitleaks"
fi

if ! installed_version lefthook version | grep -q "$LEFTHOOK_VERSION"; then
  base="https://github.com/evilmartians/lefthook/releases/download/v$LEFTHOOK_VERSION"
  f="lefthook_${LEFTHOOK_VERSION}_Linux_x86_64.gz"
  fetch "$base/$f" "$f"
  fetch "$base/lefthook_checksums.txt" sums.txt
  verify "$f" sums.txt
  gunzip -c "$tmp/$f" > "$tmp/lefthook"
  install -m 0755 "$tmp/lefthook" "$BIN/lefthook"
fi

echo "tools installed in $BIN:"
"$BIN/golangci-lint" version --short 2>/dev/null || "$BIN/golangci-lint" version
echo "gitleaks $("$BIN/gitleaks" version)"
echo "lefthook $("$BIN/lefthook" version)"
