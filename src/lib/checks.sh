# Pre-flight checks for required tools and Docker environment.

checks::basic_tools() {
    local -a missing=()
    local -a required=(awk cut grep jq sed sha256sum wc)

    for tool in "${required[@]}"; do
        command -v "$tool" &>/dev/null || missing+=("$tool")
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log::error "Missing required tools: ${missing[*]}"
        log::info "Install them and try again."
        exit 1
    fi
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
