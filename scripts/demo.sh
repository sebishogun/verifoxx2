#!/usr/bin/env bash
# demo.sh - run the Verifoxx demo packs through the built binary.
#
# Full mode (no arguments) evaluates the supplied pack into a temporary
# candidate file and verifies it byte-for-byte against the tracked
# results/requests.json. This script never modifies the tracked result;
# make eval is the explicit command that regenerates it. Both modes then
# evaluate the edge-case pack into the same temporary directory and verify
# it against fixtures/demo/expected.json. --edge-only runs only the
# edge-case pack. Output streams: this script's section headers and status
# narration print to stdout; the CLI's human decision table and progress
# print to stderr; machine-readable JSON is kept only in files. The
# temporary directory is removed on every exit.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

binary="$root/bin/verifoxx"
policy="$root/policies/policy.json"
supplied_req="$root/fixtures/requests.json"
supplied_ev="$root/fixtures/evidence.json"
supplied_out="$root/results/requests.json"
edge_req="$root/fixtures/demo/requests.json"
edge_ev="$root/fixtures/demo/evidence.json"
edge_expected="$root/fixtures/demo/expected.json"

mode="full"
case "${1:-}" in
    "")
        ;;
    --edge-only)
        mode="edge"
        ;;
    *)
        printf 'usage: %s [--edge-only]\n' "$0" >&2
        exit 2
        ;;
esac

if [[ ! -x "$binary" ]]; then
    printf 'demo: error: %s is missing or not executable\n' "$binary" >&2
    printf 'Build it first with: make build\n' >&2
    exit 1
fi

tmpdir=""
cleanup() {
    if [[ -n "$tmpdir" ]]; then
        rm -rf "$tmpdir"
    fi
}
trap cleanup EXIT

tmpdir="$(mktemp -d)"

if [[ "$mode" == "full" ]]; then
    printf '========================================\n'
    printf 'Verifoxx demo: supplied requests pack\n'
    printf '========================================\n'
    candidate="$tmpdir/supplied.json"
    "$binary" --policy "$policy" --requests "$supplied_req" --evidence "$supplied_ev" --output "$candidate"
    if cmp -s "$candidate" "$supplied_out"; then
        printf '\nSupplied pack output matches tracked results/requests.json. demo: verified\n'
    else
        printf '\ndemo: error: supplied pack output differs from tracked results/requests.json\n' >&2
        printf 'The tracked result is out of date; regenerate it with: make eval\n' >&2
        exit 1
    fi
fi

printf '========================================\n'
printf 'Verifoxx demo: edge-case requests pack\n'
printf '========================================\n'
edge_candidate="$tmpdir/edge.json"
"$binary" --policy "$policy" --requests "$edge_req" --evidence "$edge_ev" --output "$edge_candidate"
if cmp -s "$edge_candidate" "$edge_expected"; then
    printf '\nEdge pack output matches fixtures/demo/expected.json. demo: PASS\n'
else
    printf '\ndemo: error: edge pack output differs from fixtures/demo/expected.json\n' >&2
    printf 'If the behavior change is intentional, regenerate the golden with the CLI (make eval for supplied).\n' >&2
    exit 1
fi
