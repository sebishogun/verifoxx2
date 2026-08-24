#!/usr/bin/env bash
# Verify failed replacement restores the prior toolchain or preserves its backup.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

make_shims() { # repo fail_restore
    local repo="$1"
    local fail_restore="$2"
    local shims="$repo/shims"
    mkdir -p "$repo/scripts" "$repo/.tools/go" "$shims"
    cp "$root/scripts/install-go.sh" "$repo/scripts/install-go.sh"
    printf 'prior-toolchain\n' >"$repo/.tools/go/marker"

    cat >"$shims/go" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
    cat >"$shims/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
while (( $# )); do
    if [[ "$1" == "--output" ]]; then
        output="$2"
        shift 2
        continue
    fi
    shift
done
if [[ "$output" == *.sha256 ]]; then
    printf '%064d\n' 0 >"$output"
else
    : >"$output"
fi
EOF
    cat >"$shims/sha256sum" <<'EOF'
#!/usr/bin/env bash
printf '%064d  %s\n' 0 "$1"
EOF
    cat >"$shims/tar" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
destination=""
while (( $# )); do
    if [[ "$1" == "-C" ]]; then
        destination="$2"
        shift 2
        continue
    fi
    shift
done
mkdir -p "$destination/go/bin"
cat >"$destination/go/bin/go" <<'INNER'
#!/usr/bin/env bash
printf 'go version go1.27.0 test/arch\n'
INNER
chmod +x "$destination/go/bin/go"
EOF
    cat >"$shims/mv" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [[ "\$1" == */.go-install.*/go && "\$2" == */.tools/go ]]; then
    exit 1
fi
if (( $fail_restore )) && [[ "\$1" == */go.backup.* && "\$2" == */.tools/go ]]; then
    exit 1
fi
exec /bin/mv "\$@"
EOF
    chmod +x "$shims"/*
}

run_case() { # name fail_restore
    local name="$1"
    local fail_restore="$2"
    local repo="$tmpdir/$name"
    make_shims "$repo" "$fail_restore"

    set +e
    PATH="$repo/shims:/usr/bin:/bin" GO_VERSION=1.27.0 "$repo/scripts/install-go.sh" >"$repo/stdout" 2>"$repo/stderr"
    rc=$?
    set -e
    if (( rc == 0 )); then
        printf 'install-go rollback test: %s unexpectedly succeeded\n' "$name" >&2
        exit 1
    fi

    if (( fail_restore )); then
        backups=("$repo"/.tools/go.backup.*)
        if (( ${#backups[@]} != 1 )) || [[ ! -f "${backups[0]}/marker" ]]; then
            printf 'install-go rollback test: failed restore deleted the only backup\n' >&2
            exit 1
        fi
        if ! grep -q 'preserved' "$repo/stderr"; then
            printf 'install-go rollback test: failed restore did not report preserved backup:\n%s\n' "$(<"$repo/stderr")" >&2
            exit 1
        fi
    else
        if [[ ! -f "$repo/.tools/go/marker" ]]; then
            printf 'install-go rollback test: successful restore did not replace the prior toolchain\n' >&2
            exit 1
        fi
        backups=("$repo"/.tools/go.backup.*)
        if [[ -e "${backups[0]}" ]]; then
            printf 'install-go rollback test: successful restore left a backup behind\n' >&2
            exit 1
        fi
    fi
}

run_case restore-succeeds 0
run_case restore-fails 1
printf 'install-go rollback test: PASS\n'
