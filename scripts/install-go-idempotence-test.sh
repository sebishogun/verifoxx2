#!/usr/bin/env bash
# Verify that any runnable Go on PATH suppresses repository-local installation.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

mkdir -p "$tmpdir/bin"
cat >"$tmpdir/bin/go" <<'EOF'
#!/usr/bin/env bash
printf 'go version go1.0.0 test/arch\n'
EOF
chmod +x "$tmpdir/bin/go"

output="$(PATH="$tmpdir/bin:$PATH" "$root/scripts/install-go.sh" --dry-run)"
if [[ "$output" != *"already available"* ]]; then
    printf 'install-go idempotence test: expected existing-Go message, got:\n%s\n' "$output" >&2
    exit 1
fi
if [[ "$output" == *"Archive URL"* ]]; then
    printf 'install-go idempotence test: installer planned a download despite Go on PATH:\n%s\n' "$output" >&2
    exit 1
fi

printf 'install-go idempotence test: PASS\n'
