#!/usr/bin/env bash
# Verify that Makefile descriptions drive every menu/help view.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
makefile="$root/Makefile"
readme="$root/README.md"
tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

awk '/^[a-zA-Z0-9_-]+:/ {name=$0; sub(/:.*/, "", name); print name}' "$makefile" \
    | sort -u >"$tmpdir/declared"
awk '/^[a-zA-Z0-9_-]+:.*##/ {name=$0; sub(/:.*/, "", name); print name}' "$makefile" \
    | sort -u >"$tmpdir/described"

if ! diff -u "$tmpdir/declared" "$tmpdir/described"; then
    printf 'menu self-test: every public target must have a ## description\n' >&2
    exit 1
fi

"$root/scripts/menu.sh" --list | awk -F '\t' 'NF >= 2 {print $1}' | sort -u >"$tmpdir/listed"
if ! diff -u "$tmpdir/described" "$tmpdir/listed"; then
    printf 'menu self-test: menu --list must contain every described target exactly once\n' >&2
    exit 1
fi
while IFS= read -r target; do
    if ! grep -Fq "| \`$target\` |" "$readme"; then
        printf 'menu self-test: README Make Targets table is missing %s\n' "$target" >&2
        exit 1
    fi
done <"$tmpdir/listed"
if ! grep -qx shell "$tmpdir/listed"; then
    printf 'menu self-test: generated target list is missing the shell installer\n' >&2
    exit 1
fi
if ! grep -qx bench "$tmpdir/listed"; then
    printf 'menu self-test: generated target list is missing the reviewer benchmark\n' >&2
    exit 1
fi

if grep -q 'entries=(' "$root/scripts/menu.sh"; then
    printf 'menu self-test: scripts/menu.sh still duplicates targets in a static entries array\n' >&2
    exit 1
fi

preview="$("$root/scripts/menu.sh" --preview check)"
for required in check 'format checks' recipe fmt-check; do
    if [[ "$preview" != *"$required"* ]]; then
        printf 'menu self-test: check preview is missing %q:\n%s\n' "$required" "$preview" >&2
        exit 1
    fi
done
if [[ "$preview" != *"runs here"* && "$preview" != *"needs:"* ]]; then
    printf 'menu self-test: check preview has no readiness status:\n%s\n' "$preview" >&2
    exit 1
fi

preview="$("$root/scripts/menu.sh" --preview shell)"
for required in shell 'cross-shell mm shortcut' recipe; do
    if [[ "$preview" != *"$required"* ]]; then
        printf 'menu self-test: shell preview is missing %q:\n%s\n' "$required" "$preview" >&2
        exit 1
    fi
done

preview="$("$root/scripts/menu.sh" --preview bench)"
for required in bench 'lifecycle benchmarks' recipe scripts/bench.sh; do
    if [[ "$preview" != *"$required"* ]]; then
        printf 'menu self-test: bench preview is missing %q:\n%s\n' "$required" "$preview" >&2
        exit 1
    fi
done
if [[ ! -x "$root/scripts/bench.sh" ]]; then
    printf 'menu self-test: scripts/bench.sh is missing or not executable\n' >&2
    exit 1
fi
benchmark_script="$(<"$root/scripts/bench.sh")"
for required in GOMAXPROCS -benchmem EvaluateCLISuppliedPack EvaluateRows1024; do
    if [[ "$benchmark_script" != *"$required"* ]]; then
        printf 'menu self-test: scripts/bench.sh is missing %q\n' "$required" >&2
        exit 1
    fi
done

printf 'menu self-test: PASS\n'
