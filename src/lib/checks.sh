# Pre-flight checks for required tools and Docker environment.

checks::basic_tools() {
    local -a missing=()
    local -a required=(awk cut grep sed sha256sum wc)

    # jq is only needed for host config parsing (non-generic mode)
    [[ "$GENERIC" == false ]] && required+=(jq)

    for tool in "${required[@]}"; do
        command -v "$tool" &>/dev/null || missing+=("$tool")
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log::error "Missing required tools: ${missing[*]}"
        log::info "Install them and try again."
        exit 1
    fi
}

checks::claude_on_host() {
    # Check if Claude Code is installed or configured on the host
    [[ -d "$CLAUDE_CONFIG" ]] && return 0
    command -v claude &>/dev/null && return 0
    return 1
}

checks::docker_available() {
    command -v docker &>/dev/null || \
        log::die "Docker is not installed. Install Docker and try again."
}

checks::docker_daemon() {
    if ! docker info >/dev/null 2>&1; then
        log::die "Docker daemon is not running. Start Docker and try again."
    fi
}

checks::docker_runtime() {
    log::verbose "Checking Docker runtime..."

    if ! docker_runtime::detect; then
        log::die "Failed to detect Docker runtime. Please ensure Docker is running."
    fi

    # Show detected runtime in verbose mode
    if [[ "$VERBOSE" == true ]]; then
        docker_runtime::show_info
    fi
}

checks::disk_space() {
    if ! command -v df &>/dev/null; then
        return 0
    fi
    local available_mb
    available_mb=$(df -m / 2>/dev/null | awk 'NR==2 {print $4}')
    if [[ ${available_mb:-0} -lt 5120 ]]; then
        log::warn "Low disk space ($((available_mb / 1024))GB available). Docker builds may fail."
    fi
}

checks::workspace_exists() {
    [[ -d "$WORKSPACE" ]] || log::die "Workspace path does not exist: $WORKSPACE"
}

checks::conflicting_flags() {
    # Reserved for future flag conflict checks
    :
}
