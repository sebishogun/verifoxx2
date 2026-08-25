#!/usr/bin/env bash
# Verify that the doctor gives an actionable, platform-specific Make remedy.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmpdir="$(mktemp -d)"
cleanup() {
    rm -rf "$tmpdir"
}
trap cleanup EXIT

mkdir -p "$tmpdir/bin"
for tool in bash dirname sed head uname awk tr curl tar sha256sum shasum; do
    path="$(command -v "$tool" 2>/dev/null || true)"
    if [[ -n "$path" ]]; then
        ln -s "$path" "$tmpdir/bin/$tool"
    fi
done

platform="$(uname -s)"
case "$platform" in
    Darwin)
        expected="xcode-select --install"
        ;;
    Linux)
        expected="install GNU Make with your system package manager"
        for candidate in apt-get dnf pacman zypper apk; do
            path="$(command -v "$candidate" 2>/dev/null || true)"
            if [[ -n "$path" ]]; then
                ln -s "$path" "$tmpdir/bin/$candidate"
                case "$candidate" in
                    apt-get) expected="sudo apt-get install make" ;;
                    dnf) expected="sudo dnf install make" ;;
                    pacman) expected="sudo pacman -S make" ;;
                    zypper) expected="sudo zypper install make" ;;
                    apk) expected="run as root: apk add bash make" ;;
                esac
                break
            fi
        done
        ;;
    *)
        expected="install GNU Make with your system package manager"
        ;;
esac

set +e
output="$(PATH="$tmpdir/bin" /bin/bash "$root/scripts/doctor.sh" 2>&1)"
status=$?
set -e

if [[ $status -eq 0 ]]; then
    printf 'doctor self-test: doctor succeeded with Make removed from PATH\n' >&2
    exit 1
fi
for required in 'make' 'MISSING' "$expected" 'scripts/docker-demo.sh'; do
    if [[ "$output" != *"$required"* ]]; then
        printf 'doctor self-test: missing %q in output:\n%s\n' "$required" "$output" >&2
        exit 1
    fi
done

printf 'doctor self-test: PASS\n'
