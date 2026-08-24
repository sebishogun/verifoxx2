#!/usr/bin/env bash
# doctor.sh - report the local workflow dependencies for this repository.
#
# Required: Bash, Make, Go 1.27+ (resolved through scripts/go.sh, which
# prefers the repository-local .tools/go toolchain). Installer readiness
# (curl, tar, a checksum utility) and optional tools (Docker, Compose, fzf) are
# reported but never fail the check. No network access and no Docker
# daemon interaction.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

required_ok=1

report() { # name version status note
    printf '  %-12s %-22s %-10s %s\n' "$1" "$2" "$3" "$4"
}

go_at_least() { # version major minor
    local ver="$1"
    local need_major="$2"
    local need_minor="$3"
    local major="${ver%%.*}"
    local rest="${ver#*.}"
    local minor="${rest%%.*}"
    if (( major > need_major )); then
        return 0
    fi
    if (( major == need_major && minor >= need_minor )); then
        return 0
    fi
    return 1
}

printf 'Verifoxx local workflow doctor\n'
printf '==============================\n'

printf 'Required:\n'

bash_ver="${BASH_VERSION%%(*}"
report bash "${bash_ver:-present}" OK ""

if command -v make >/dev/null 2>&1; then
    make_ver="$(make --version 2>/dev/null | sed -n '1p')"
    report make "${make_ver:-present}" OK ""
else
    report make "-" MISSING "install GNU make"
    required_ok=0
fi

if go_ver_out="$("$root/scripts/go.sh" version 2>/dev/null)"; then
    go_ver="$(sed -n 's/.*go\([0-9][0-9.]*\).*/\1/p' <<<"$go_ver_out" | head -n1)"
    if [[ -n "$go_ver" ]] && go_at_least "$go_ver" 1 27; then
        goroot="$("$root/scripts/go.sh" env GOROOT 2>/dev/null)"
        report go "$go_ver" OK "via ${goroot:-?}/bin/go"
    else
            report go "${go_ver:-unknown}" OUTDATED "need Go 1.27+; upgrade the existing installation"
        required_ok=0
    fi
else
    report go "-" MISSING "run scripts/install-go.sh"
    required_ok=0
fi

printf 'Installer readiness:\n'

if command -v curl >/dev/null 2>&1; then
    curl_ver="$(curl --version 2>/dev/null | sed -n '1s/^curl \([0-9.]*\).*/\1/p')"
    report curl "${curl_ver:-present}" OK ""
else
    report curl "-" MISSING "needed by scripts/install-go.sh"
fi

if command -v tar >/dev/null 2>&1; then
    tar_ver="$(tar --version 2>/dev/null | sed -n '1p')"
    report tar "${tar_ver:-present}" OK ""
else
    report tar "-" MISSING "needed by scripts/install-go.sh"
fi

if command -v sha256sum >/dev/null 2>&1; then
    cs_ver="$(sha256sum --version 2>/dev/null | sed -n '1p')"
    report sha256sum "${cs_ver:-present}" OK ""
elif command -v shasum >/dev/null 2>&1; then
    cs_ver="$(shasum --version 2>/dev/null | sed -n '1p')"
    report shasum "${cs_ver:-present}" OK ""
else
    report "checksum tool" "-" MISSING "needed by scripts/install-go.sh"
fi

printf 'Optional:\n'

if command -v docker >/dev/null 2>&1; then
    docker_ver="$(docker --version 2>/dev/null | sed -n '1s/^Docker version \([^,]*\).*/Docker \1/p')"
    report docker "${docker_ver:-present}" present ""
    if compose_ver="$(docker compose version --short 2>/dev/null)"; then
        report compose "${compose_ver:-present}" present ""
    else
        report compose "-" missing "optional (Compose workflow)"
    fi
else
    report docker "-" missing "optional (Docker workflow)"
    report compose "-" missing "optional (Compose workflow)"
fi

if command -v fzf >/dev/null 2>&1; then
    fzf_ver="$(fzf --version 2>/dev/null | awk '{ print $1 }')"
    report fzf "${fzf_ver:-present}" present ""
else
    report fzf "-" missing "optional (interactive menu)"
fi

if (( required_ok )); then
    printf 'Result: all required local workflow dependencies present\n'
else
    printf 'Result: required dependencies missing or outdated\n'
    printf 'Remedy: if Go is absent, scripts/install-go.sh installs it locally; upgrade an existing outdated Go. Install GNU make for make targets.\n'
    exit 1
fi
