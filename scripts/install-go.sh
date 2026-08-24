#!/usr/bin/env bash
# install-go.sh - reuse an installed Go toolchain or install one locally.
#
# If any runnable Go is already on PATH or at .tools/go/bin/go, exits without
# creating files or contacting the network. Otherwise downloads the official
# Go release archive and its .sha256 checksum from go.dev, verifies the archive
# before extraction, and replaces only .tools/go under the repository root.
# All staging, locking and backups live under .tools so the final rename stays
# same-filesystem. Never uses sudo and never writes outside the repository.
#
# A PID-based mkdir lock under .tools serializes installs. A lock whose
# recorded PID is no longer alive is treated as stale: it is atomically
# renamed to a unique sibling owned by this contender and deleted only if
# its captured PID is numeric and proven dead, after which acquisition is
# retried. If the captured PID is live or unreadable (e.g. the holder's
# create-before-write window), the renamed directory is restored to the
# active lock path when free and this process fails closed, never deleting
# a live or unreadable lock.
# INT and TERM are trapped to exit 130/143 after the EXIT-trap cleanup, so
# make never reports success for an interrupted install. Bash runs these
# traps only after the current foreground child is reaped: reliable delivery
# is terminal Ctrl-C (SIGINT to the whole foreground process group) or a
# process-group TERM (e.g. make killing the job), not a bare `kill -INT`
# sent only to this shell process.
set -euo pipefail

GO_VERSION="${GO_VERSION:-1.27.0}"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tools_dir="$root/.tools"
dest="$tools_dir/go"

stage=""
backup=""
lock_dir=""
stale_dir=""
had_backup=0
lock_held=0

die() {
    printf 'install-go: error: %s\n' "$*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Usage: install-go.sh [--dry-run]

Reuse any runnable Go already on PATH or under <repo>/.tools/go. If none is
available, install the pinned toolchain to <repo>/.tools/go.

Options:
  --dry-run  Report the existing Go no-op, or print the planned version,
              platform, URLs and destination without creating anything.

Environment:
  GO_VERSION  Fallback Go version when no runnable Go exists. Accepted forms:
              a three-part release such as 1.27.0, or a prerelease such as
              1.27rc1 / 1.27beta2 (default 1.27.0).
EOF
}

case "$#" in
    0) dry_run=0 ;;
    1)
        if [[ "$1" != "--dry-run" ]]; then
            usage
            die "unknown option: $1"
        fi
        dry_run=1
        ;;
    *)
        usage
        die "unexpected arguments: $*"
        ;;
esac

report_existing_go() { # executable source
    local executable="$1"
    local source="$2"
    local version
    if ! version="$("$executable" version 2>&1)"; then
        return 1
    fi
    version="${version%%$'\n'*}"
    printf 'install-go: Go is already available via %s at %s (%s); skipping repository-local installation.\n' \
        "$source" "$executable" "$version"
}

path_go="$(command -v go 2>/dev/null || true)"
if [[ -n "$path_go" ]] && report_existing_go "$path_go" PATH; then
    exit 0
fi

local_go="$dest/bin/go"
if [[ ! -L "$tools_dir" && ! -L "$dest" && -x "$local_go" ]] && report_existing_go "$local_go" .tools/go; then
    exit 0
fi

if [[ ! "$GO_VERSION" =~ ^([0-9]+\.[0-9]+\.[0-9]+|[0-9]+\.[0-9]+(rc|beta)[0-9]+)$ ]]; then
    die "invalid GO_VERSION '$GO_VERSION' (use a three-part release such as 1.27.0 or a prerelease such as 1.27rc1)"
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
    linux|darwin) ;;
    *) die "unsupported OS '$os' (supported: linux, darwin)" ;;
esac

case "$arch" in
    amd64|x86_64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported architecture '$arch' (supported: amd64, arm64)" ;;
esac

archive_url="https://go.dev/dl/go${GO_VERSION}.${os}-${arch}.tar.gz"
checksum_url="${archive_url}.sha256"

if (( dry_run )); then
    printf 'Go version:    %s\n' "$GO_VERSION"
    printf 'Platform:      %s/%s\n' "$os" "$arch"
    printf 'Archive URL:   %s\n' "$archive_url"
    printf 'Checksum URL:  %s\n' "$checksum_url"
    printf 'Destination:   %s\n' "$dest"
    printf 'No files were downloaded or created.\n'
    exit 0
fi

command -v curl >/dev/null 2>&1 || die "curl is required to download the Go toolchain"
command -v tar >/dev/null 2>&1 || die "tar is required to extract the Go toolchain"
checksum_tool=""
if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool="shasum"
else
    die "neither sha256sum nor shasum is available; a checksum utility is required"
fi

[[ -L "$tools_dir" ]] && die "refusing to install: $tools_dir is a symlink"
if [[ -e "$tools_dir" && ! -d "$tools_dir" ]]; then
    die "refusing to install: $tools_dir exists and is not a directory"
fi

cleanup() {
    local rc=$?
    local restore_failed=0
    trap - INT TERM
    if (( had_backup )) && [[ -e "$backup" ]] && [[ ! -e "$dest" ]]; then
        if mv "$backup" "$dest" 2>/dev/null; then
            backup=""
        else
            restore_failed=1
            rc=1
            printf 'install-go: error: could not restore prior toolchain; backup preserved at %s\n' "$backup" >&2
        fi
    fi
    if (( ! restore_failed )); then
        [[ -n "$backup" ]] && rm -rf "$backup" 2>/dev/null || true
    fi
    [[ -n "$stage" ]] && rm -rf "$stage" 2>/dev/null || true
    [[ -n "$stale_dir" ]] && rm -rf "$stale_dir" 2>/dev/null || true
    if (( lock_held )); then
        rm -rf "$lock_dir" 2>/dev/null || true
    fi
    exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Acquire the install lock exclusively. mkdir is atomic; the lock directory
# carries our PID so a later contender can decide live vs stale. A stale
# lock (dead PID) is atomically renamed to a unique sibling owned by this
# contender and deleted only if its captured PID is numeric and proven dead,
# then acquisition is retried. A captured lock whose PID is live or
# unreadable is restored to the active path when free and the acquire fails
# closed; an unreadable captured directory is never deleted.
acquire_lock() {
    local attempt pid captured captured_live
    stale_path="$tools_dir/.install.lock.stale.$$"
    if [[ -e "$stale_path" ]]; then
        prev="$(cat "$stale_path/pid" 2>/dev/null || true)"
        if [[ "$prev" =~ ^[0-9]+$ ]] && ! kill -0 "$prev" 2>/dev/null; then
            rm -rf "$stale_path" 2>/dev/null || true
        fi
        if [[ -e "$stale_path" ]]; then
            n=0
            while [[ -e "$stale_path.$n" ]]; do n=$((n+1)); done
            stale_path="$stale_path.$n"
        fi
    fi
    for attempt in 1 2 3; do
        if mkdir "$lock_dir" 2>/dev/null; then
            lock_held=1
            printf '%s\n' "$$" >"$lock_dir/pid"
            return 0
        fi
        pid=""
        if [[ -f "$lock_dir/pid" ]]; then
            pid="$(cat "$lock_dir/pid" 2>/dev/null || true)"
        fi
        if [[ "$pid" =~ ^[0-9]+$ ]] && kill -0 "$pid" 2>/dev/null; then
            die "another install-go process (PID $pid) is running (lock: $lock_dir)"
        fi
        if [[ "$pid" =~ ^[0-9]+$ ]] && ! kill -0 "$pid" 2>/dev/null; then
            # The recorded PID is dead, so the lock looks stale. Rename it to
            # a unique sibling (atomic) rather than removing the active path;
            # only the contender whose rename succeeds may proceed.
            if mv "$lock_dir" "$stale_path" 2>/dev/null; then
                stale_dir="$stale_path"
                captured="$(cat "$stale_dir/pid" 2>/dev/null || true)"
                if [[ "$captured" =~ ^[0-9]+$ ]] && ! kill -0 "$captured" 2>/dev/null; then
                    # Numeric and proven dead: this really is the stale lock.
                    # Delete our renamed copy and retry acquisition.
                    if rm -rf "$stale_dir" 2>/dev/null; then
                        stale_dir=""
                    fi
                    continue
                fi
                # Captured PID is live, missing, or non-numeric: never delete
                # it. Restore the captured directory to the active lock path
                # when that path is free, clear our ownership, fail closed.
                captured_live=0
                if [[ "$captured" =~ ^[0-9]+$ ]] && kill -0 "$captured" 2>/dev/null; then
                    captured_live=1
                fi
                if [[ ! -e "$lock_dir" ]] && mv "$stale_dir" "$lock_dir" 2>/dev/null; then
                    stale_dir=""
                    if (( captured_live )); then
                        die "another install-go process (PID $captured) is running (lock: $lock_dir)"
                    fi
                    die "lock $lock_dir exists but its owner PID is unreadable; refusing to steal it"
                fi
                # Restore failed or the active path is occupied again: leave
                # the captured directory in place and clear our ownership so
                # cleanup never deletes it; fail closed.
                stale_dir=""
                if (( captured_live )); then
                    die "another install-go process (PID $captured) is running (lock: $lock_dir); its lock could not be restored, refusing to delete it"
                fi
                die "lock $lock_dir was captured but could not be restored; owner PID unreadable, refusing to delete it"
            fi
            # The mv lost: another process already moved or replaced the
            # lock. Retry without deleting whatever is at the lock path now.
            continue
        fi
        die "lock $lock_dir exists but its owner PID is unreadable; refusing to steal it"
    done
    die "cannot acquire lock $lock_dir (contended)"
}

mkdir -p "$tools_dir"

lock_dir="$tools_dir/.install.lock"
acquire_lock

# Recover or clear leftovers from a run killed with SIGKILL (ordinary
# success/failure paths and the INT/TERM handlers never leave these; the
# EXIT trap cleans them up, and a SIGKILL'd run leaves only a now-stale
# lock whose dead PID the next acquire removes automatically).
if [[ -e "$dest" ]]; then
    rm -rf "$tools_dir"/go.backup.* 2>/dev/null || true
else
    for stale in "$tools_dir"/go.backup.*; do
        [[ -e "$stale" ]] || continue
        mv "$stale" "$dest" 2>/dev/null || true
        break
    done
fi
rm -rf "$tools_dir"/.go-install.* 2>/dev/null || true

# Remove renamed stale-lock copies whose CAPTURED PID is numeric and proven
# dead (e.g. the renamer was SIGKILL'd after renaming a dead lock). A copy
# whose captured PID is live or unreadable is a real lock in transit and is
# never touched here.
for stale in "$tools_dir"/.install.lock.stale.*; do
    [[ -e "$stale" ]] || continue
    captured="$(cat "$stale/pid" 2>/dev/null || true)"
    if [[ "$captured" =~ ^[0-9]+$ ]] && ! kill -0 "$captured" 2>/dev/null; then
        rm -rf "$stale" 2>/dev/null || true
    fi
done

stage="$(mktemp -d "$tools_dir/.go-install.XXXXXX")"

curl --fail --location --proto '=https' --proto-redir '=https' --silent --show-error \
    --output "$stage/go.tar.gz" "$archive_url"
curl --fail --location --proto '=https' --proto-redir '=https' --silent --show-error \
    --output "$stage/go.tar.gz.sha256" "$checksum_url"

expected="$(awk 'NR==1 { print $1 }' "$stage/go.tar.gz.sha256")"
if [[ ! "$expected" =~ ^[0-9a-f]{64}$ ]]; then
    die "invalid checksum file from $checksum_url (expected 64 hex digits)"
fi
if [[ "$checksum_tool" == "sha256sum" ]]; then
    actual="$(sha256sum "$stage/go.tar.gz" | awk '{ print $1 }')"
else
    actual="$(shasum -a 256 "$stage/go.tar.gz" | awk '{ print $1 }')"
fi
if [[ "$actual" != "$expected" ]]; then
    die "checksum mismatch for $archive_url; archive not trusted, nothing was installed"
fi

tar -xzf "$stage/go.tar.gz" -C "$stage"
[[ -x "$stage/go/bin/go" ]] || die "extracted archive has no executable go/bin/go"
staged_ver="$( "$stage/go/bin/go" version 2>&1 )" || die "extracted go binary failed to run"
staged_token="$(awk -v want="go${GO_VERSION}" '{ for (i=1; i<=NF; i++) if ($i == want) { print $i; exit } }' <<<"$staged_ver")"
if [[ "$staged_token" != "go${GO_VERSION}" ]]; then
    die "extracted Go reports '$staged_ver', expected version token go${GO_VERSION}"
fi

backup="$tools_dir/go.backup.$$"
if [[ -e "$dest" ]]; then
    mv "$dest" "$backup"
    had_backup=1
fi
mv "$stage/go" "$dest"
rm -rf "$backup"

printf 'Installed Go %s (%s/%s) to %s\n' "$GO_VERSION" "$os" "$arch" "$dest"
printf 'scripts/go.sh now prefers %s over any go on PATH.\n' "$dest/bin/go"
printf 'Make integration: every Go-based make target resolves the toolchain through scripts/go.sh.\n'
printf 'Run: make check   (or any make target)\n'
