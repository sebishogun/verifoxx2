#!/usr/bin/env bash
# Interactive picker generated from Makefile targets carrying `##` descriptions.
# fzf gets readiness marks and a recipe preview; without fzf the same rows are
# presented as a numbered prompt. No target names or descriptions are duplicated.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
makefile="$root/Makefile"
script="$root/scripts/menu.sh"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    bold=$'\033[1m'
    dim=$'\033[2m'
    reset=$'\033[0m'
    green=$'\033[32m'
    yellow=$'\033[33m'
    cyan=$'\033[36m'
else
    bold=""
    dim=""
    reset=""
    green=""
    yellow=""
    cyan=""
fi

have() {
    command -v "$1" >/dev/null 2>&1
}

go_ready() {
    "$root/scripts/go.sh" version >/dev/null 2>&1
}

docker_ready() {
    have docker && docker info >/dev/null 2>&1
}

compose_ready() {
    docker_ready && docker compose version >/dev/null 2>&1
}

checksum_ready() {
    have sha256sum || have shasum
}

requirements_for() {
    case "$1" in
        docker-*) echo docker ;;
        compose-*) echo compose ;;
        all|build|eval|fmt-check|test|vet|check|demo|demo-edge|setup|shell) echo go ;;
    esac
}

missing_for() {
    local target="$1"
    local need

    if [[ "$target" == "install-go" ]] && ! go_ready; then
        have curl || printf 'curl\n'
        have tar || printf 'tar\n'
        checksum_ready || printf 'sha256sum or shasum\n'
        return
    fi

    while IFS= read -r need; do
        [[ -n "$need" ]] || continue
        case "$need" in
            go) go_ready || printf 'Go 1.27+\n' ;;
            docker) docker_ready || printf 'Docker daemon\n' ;;
            compose) compose_ready || printf 'Docker daemon and Compose plugin\n' ;;
        esac
    done < <(requirements_for "$target")
}

install_hint() {
    case "$1" in
        "Go 1.27+") printf 'run: make install-go\n' ;;
        "Docker daemon") printf 'install/start Docker and grant this user access\n' ;;
        "Docker daemon and Compose plugin") printf 'install/start Docker with the Compose plugin\n' ;;
        "sha256sum or shasum") printf 'install a SHA-256 checksum utility\n' ;;
        *) printf 'install %s\n' "$1" ;;
    esac
}

targets() {
    awk -F ':' '/^[a-zA-Z0-9_-]+:.*##/ {print $1}' "$makefile" | awk '!seen[$0]++'
}

describe_target() {
    awk -v target="$1" -F ':' '$1 == target && /##/ {sub(/.*## */, ""); print; exit}' "$makefile"
}

preview_target() {
    local target="$1"
    local description missing definition recipes

    description="$(describe_target "$target")"
    printf '%s%s%s\n\n' "$bold" "$target" "$reset"
    [[ -n "$description" ]] && printf '%s\n\n' "$description"

    missing="$(missing_for "$target")"
    if [[ -z "$missing" ]]; then
        printf '%s● runs here%s\n\n' "$green" "$reset"
    else
        printf '%s○ needs:%s\n' "$yellow" "$reset"
        while IFS= read -r requirement; do
            [[ -n "$requirement" ]] || continue
            printf '  %s\n    %s%s%s\n' "$requirement" "$dim" "$(install_hint "$requirement")" "$reset"
        done <<<"$missing"
        printf '\n'
    fi

    printf '%sdefinition / recipe%s\n' "$cyan" "$reset"
    definition="$(awk -v target="$target" '$0 ~ "^" target ":" {print; exit}' "$makefile")"
    printf '  %s\n' "$definition"
    recipes="$(awk -v target="$target" '
        $0 ~ "^" target ":" {found=1; next}
        found && /^\t/ {print "  " $0; next}
        found && /^$/ {next}
        found {exit}
    ' "$makefile")"
    [[ -n "$recipes" ]] && printf '%s\n' "$recipes"
    return 0
}

build_rows() {
    local target description missing mark
    while IFS= read -r target; do
        description="$(describe_target "$target")"
        missing="$(missing_for "$target")"
        if [[ -z "$missing" ]]; then
            mark="${green}●${reset}"
        else
            mark="${dim}○${reset}"
        fi
        printf '%s %-14s %s%s%s\n' "$mark" "$target" "$dim" "${description:0:68}" "$reset"
    done < <(targets)
}

list_targets() {
    local target
    while IFS= read -r target; do
        printf '%s\t%s\n' "$target" "$(describe_target "$target")"
    done < <(targets)
}

usage() {
    printf 'usage: %s [--list | --rows | --preview TARGET | MAKE_TARGET...]\n' "$0" >&2
}

case "${1:-}" in
    --preview)
        [[ $# -eq 2 ]] || { usage; exit 2; }
        preview_target "$2"
        exit 0
        ;;
    --rows)
        [[ $# -eq 1 ]] || { usage; exit 2; }
        build_rows
        exit 0
        ;;
    --list)
        [[ $# -eq 1 ]] || { usage; exit 2; }
        list_targets
        exit 0
        ;;
esac

if [[ $# -gt 0 ]]; then
    exec make -C "$root" "$@"
fi

if [[ ! -t 0 || ! -t 1 ]]; then
    printf 'menu: interactive mode needs a TTY; use --list for non-interactive discovery\n' >&2
    exit 1
fi

pick_with_fzf() {
    build_rows | fzf \
        --ansi \
        --height='90%' \
        --layout=reverse \
        --border=rounded \
        --border-label=' make targets ' \
        --prompt='target > ' \
        --pointer='▸' \
        --header=$'● runs here   ○ needs something\nenter run · ctrl-r reload · esc quit' \
        --preview="bash '$script' --preview {2}" \
        --preview-window='right,58%,border-left,wrap' \
        --bind="ctrl-r:reload(bash '$script' --rows)" \
        | awk '{print $2}'
}

pick_by_number() {
    local names=()
    local target description missing mark choice
    local index=1

    while IFS= read -r target; do
        names+=("$target")
        description="$(describe_target "$target")"
        missing="$(missing_for "$target")"
        if [[ -z "$missing" ]]; then
            mark="${green}●${reset}"
        else
            mark="${dim}○${reset}"
        fi
        printf '%s %2d) %-14s %s%s%s\n' "$mark" "$index" "$target" "$dim" "${description:0:68}" "$reset" >/dev/tty
        index=$((index + 1))
    done < <(targets)

    printf '\n%s● runs here   ○ needs something%s\n' "$dim" "$reset" >/dev/tty
    printf 'target number (enter to quit) > ' >/dev/tty
    read -r choice </dev/tty || true
    [[ -n "${choice:-}" ]] || return 0
    if [[ ! "$choice" =~ ^[0-9]+$ ]]; then
        printf 'not a number: %s\n' "$choice" >/dev/tty
        return 0
    fi
    if (( choice >= 1 && choice <= ${#names[@]} )); then
        printf '%s\n' "${names[choice - 1]}"
    fi
}

target=""
if have fzf; then
    target="$(pick_with_fzf || true)"
else
    target="$(pick_by_number)"
fi

[[ -n "$target" ]] || exit 0
printf '%s$ make %s%s\n' "$dim" "$target" "$reset"
exec make -C "$root" "$target"
