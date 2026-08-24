#!/usr/bin/env bash
# compose-demo.sh - verify both Verifoxx fixture packs through Docker Compose.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${IMAGE_NAME:-verifoxx:local}"
project="verifoxx-demo-$$"
compose=(docker compose --project-name "$project" --file "$root/compose.yaml")

if [[ $# -ne 0 ]]; then
    printf 'usage: %s\n' "$0" >&2
    printf 'Set IMAGE_NAME to change the image tag (default verifoxx:local).\n' >&2
    exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
    printf 'compose-demo: error: docker command not found on PATH\n' >&2
    exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
    printf 'compose-demo: error: docker compose plugin is unavailable\n' >&2
    exit 1
fi
if ! docker info >/dev/null 2>&1; then
    printf 'compose-demo: error: docker daemon is not reachable\n' >&2
    exit 1
fi

tmpdir=""
cleanup() {
    IMAGE_NAME="$image" "${compose[@]}" down --remove-orphans >/dev/null 2>&1 || true
    if [[ -n "$tmpdir" ]]; then
        rm -rf "$tmpdir"
    fi
}
trap cleanup EXIT

printf 'compose-demo: building image %s\n' "$image"
IMAGE_NAME="$image" "${compose[@]}" build

tmpdir="$(mktemp -d)"
supplied_out="$tmpdir/supplied.json"
edge_out="$tmpdir/edge.json"

printf '========================================\n'
printf 'Verifoxx Compose demo: supplied requests pack\n'
printf '========================================\n'
IMAGE_NAME="$image" "${compose[@]}" run --rm -T --no-deps verifoxx >"$supplied_out"
if cmp -s "$supplied_out" "$root/results/requests.json"; then
    printf '\nSupplied pack output matches tracked results/requests.json. compose-demo: PASS\n'
else
    printf '\ncompose-demo: error: supplied output differs from results/requests.json\n' >&2
    exit 1
fi

printf '========================================\n'
printf 'Verifoxx Compose demo: edge-case requests pack\n'
printf '========================================\n'
IMAGE_NAME="$image" "${compose[@]}" run --rm -T --no-deps verifoxx \
    --policy policies/policy.json \
    --requests fixtures/demo/requests.json \
    --evidence fixtures/demo/evidence.json \
    --output - >"$edge_out"
if cmp -s "$edge_out" "$root/fixtures/demo/expected.json"; then
    printf '\nEdge pack output matches fixtures/demo/expected.json. compose-demo: PASS\n'
else
    printf '\ncompose-demo: error: edge output differs from fixtures/demo/expected.json\n' >&2
    exit 1
fi
