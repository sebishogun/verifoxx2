#!/usr/bin/env bash
# docker-demo.sh - run the Verifoxx demo packs through the Docker image.
#
# Builds the image once (IMAGE_NAME env, default verifoxx:local), runs the
# default supplied-pack evaluation, then overrides the CLI flags for the
# edge-case pack. Each run captures the container's stdout (pure JSON via
# --output -) into a temporary file and compares it byte-for-byte against
# the tracked results/requests.json and fixtures/demo/expected.json; the
# CLI's human decision table and progress pass through to stderr. The
# temporary directory is removed on every exit and containers are removed
# with --rm.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

image="${IMAGE_NAME:-verifoxx:local}"

if [[ $# -ne 0 ]]; then
    printf 'usage: %s\n' "$0" >&2
    printf 'Set IMAGE_NAME to change the image tag (default verifoxx:local).\n' >&2
    exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
    printf 'docker-demo: error: docker command not found on PATH\n' >&2
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    printf 'docker-demo: error: docker daemon is not reachable (docker info failed)\n' >&2
    exit 1
fi

tmpdir=""
cleanup() {
    if [[ -n "$tmpdir" ]]; then
        rm -rf "$tmpdir"
    fi
}
trap cleanup EXIT

printf 'docker-demo: building image %s\n' "$image"
docker build -q -t "$image" "$root" >/dev/null

tmpdir="$(mktemp -d)"
supplied_out="$tmpdir/supplied.json"
edge_out="$tmpdir/edge.json"

printf '========================================\n'
printf 'Verifoxx docker demo: supplied requests pack\n'
printf '========================================\n'
docker run --rm "$image" >"$supplied_out"
if cmp -s "$supplied_out" "$root/results/requests.json"; then
    printf '\nSupplied pack output matches tracked results/requests.json. docker-demo: PASS\n'
else
    printf '\ndocker-demo: error: supplied pack output differs from tracked results/requests.json\n' >&2
    exit 1
fi

printf '========================================\n'
printf 'Verifoxx docker demo: edge-case requests pack\n'
printf '========================================\n'
docker run --rm "$image" --policy policies/policy.json --requests fixtures/demo/requests.json --evidence fixtures/demo/evidence.json --output - >"$edge_out"
if cmp -s "$edge_out" "$root/fixtures/demo/expected.json"; then
    printf '\nEdge pack output matches fixtures/demo/expected.json. docker-demo: PASS\n'
else
    printf '\ndocker-demo: error: edge pack output differs from fixtures/demo/expected.json\n' >&2
    exit 1
fi
