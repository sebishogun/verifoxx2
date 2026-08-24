#!/usr/bin/env bash
# go.sh - resolve and exec the Go toolchain for this repository.
# Prefers the repository-local install at .tools/go/bin/go, falls back to
# any go on PATH, and otherwise prints a hint pointing at make install-go.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
local_go="$root/.tools/go/bin/go"

if [[ -x "$local_go" ]]; then
    exec "$local_go" "$@"
fi

if command -v go >/dev/null 2>&1; then
    exec "$(command -v go)" "$@"
fi

printf 'Go 1.27+ was not found. Run: make install-go\n' >&2
exit 1
