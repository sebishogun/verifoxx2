#!/usr/bin/env bash
# Run the bounded reviewer benchmark suite with reproducible Go settings.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

exec env GOMAXPROCS=1 "$root/scripts/go.sh" test \
    -p 1 \
    -run '^$' \
    -bench 'Benchmark(EvaluateCLISuppliedPack|SteadyFrameSuppliedPack|SessionSuppliedPack|EvaluateRows5|EvaluateRows1024)$' \
    -benchmem -benchtime=500ms -count=3 -timeout 60s \
    ./cmd/verifoxx ./internal/engine ./internal/eval
