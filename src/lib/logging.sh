# Logging functions using tput for terminal-aware colors.
# All output goes to stderr so stdout stays clean for programmatic use
# (e.g., --export-dockerfile piped to a file).

_LOG_PREFIX="[vibrator]"

# Cache tput values once (tput is slow, avoid repeated calls)
if [[ -t 2 ]]; then
    _C_RESET=$(tput sgr0 2>/dev/null || true)
    _C_BOLD=$(tput bold 2>/dev/null || true)
    _C_RED=$(tput setaf 1 2>/dev/null || true)
    _C_GREEN=$(tput setaf 2 2>/dev/null || true)
    _C_YELLOW=$(tput setaf 3 2>/dev/null || true)
    _C_CYAN=$(tput setaf 6 2>/dev/null || true)
else
    _C_RESET="" _C_BOLD="" _C_RED="" _C_GREEN="" _C_YELLOW="" _C_CYAN=""
fi

log::info() {
    printf '%s %s\n' "$_LOG_PREFIX" "$*" >&2
}

log::success() {
    printf '%s%s %s%s\n' "$_C_GREEN" "$_LOG_PREFIX" "$*" "$_C_RESET" >&2
}

log::warn() {
    printf '%s%s WARNING: %s%s\n' "$_C_YELLOW" "$_LOG_PREFIX" "$*" "$_C_RESET" >&2
}

log::error() {
    printf '%s%s ERROR: %s%s\n' "$_C_RED" "$_LOG_PREFIX" "$*" "$_C_RESET" >&2
}

log::verbose() {
    [[ "${VERBOSE:-false}" == true ]] || return 0
    printf '%s%s %s%s\n' "$_C_CYAN" "$_LOG_PREFIX" "$*" "$_C_RESET" >&2
}

log::die() {
    log::error "$@"
    exit 1
}

# Pretty-print a docker command array for verbose/dry-run output.
# Each flag gets its own line for readability.
log::docker_cmd() {
    local -a args=("$@")
    local i=0 line=""

    printf '%s Docker command:\n' "$_LOG_PREFIX" >&2
    for arg in "${args[@]}"; do
        if [[ $i -eq 0 ]]; then
            # First two words: "docker run" or "docker exec"
            line="  ${_C_BOLD}${arg}${_C_RESET}"
        elif [[ $i -eq 1 && "$arg" != -* ]]; then
            line+=" ${_C_BOLD}${arg}${_C_RESET}"
        elif [[ "$arg" == --* ]]; then
            [[ -n "$line" ]] && printf '%s \\\n' "$line" >&2
            line="    ${_C_CYAN}${arg}${_C_RESET}"
        elif [[ "$arg" == -* && ${#arg} -le 3 ]]; then
            [[ -n "$line" ]] && printf '%s \\\n' "$line" >&2
            line="    ${_C_CYAN}${arg}${_C_RESET}"
        else
            line+=" ${arg}"
        fi
        ((i++))
    done
    [[ -n "$line" ]] && printf '%s\n' "$line" >&2
}
