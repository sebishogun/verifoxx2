#!/usr/bin/env bash
# gofmt-check.sh - check gofmt formatting of all Go files under cmd/ and internal/.
#
# Resolves the Go toolchain through scripts/go.sh (the same script the
# Makefile uses via GO := ./scripts/go.sh), asks it for GOROOT, and runs
# that toolchain's gofmt -l over every *.go file on disk under cmd/ and internal/
# (including untracked files). File paths are collected from find -print and
# read line-delimited, so repository Go filenames must not contain newlines;
# spaces within a name are preserved. Fails listing the offending paths when
# any file is unformatted.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

goroot="$("$root/scripts/go.sh" env GOROOT 2>/dev/null)" || {
    printf 'gofmt-check: error: could not determine GOROOT via scripts/go.sh\n' >&2
    exit 1
}
gofmt="$goroot/bin/gofmt"

if [[ ! -x "$gofmt" ]]; then
    printf 'gofmt-check: error: gofmt not found at %s\n' "$gofmt" >&2
    exit 1
fi

files=()
while IFS= read -r f; do
    files+=("$f")
done < <(find "$root/cmd" "$root/internal" -type f -name '*.go' -print 2>/dev/null)

if (( ${#files[@]} == 0 )); then
    printf 'gofmt-check: no Go files under cmd/ and internal/; nothing to check\n'
    exit 0
fi

unformatted="$("$gofmt" -l "${files[@]}")"

if [[ -n "$unformatted" ]]; then
    count="$(wc -l <<<"$unformatted")"
    printf 'gofmt-check: %s file(s) are not gofmt-formatted:\n' "$count"
    while IFS= read -r f; do
        printf '  %s\n' "${f#"$root"/}"
    done <<<"$unformatted"
    printf 'Run: %s -w cmd internal\n' "$gofmt"
    exit 1
fi

printf 'gofmt-check: all %d Go files under cmd/ and internal/ are formatted\n' "${#files[@]}"
