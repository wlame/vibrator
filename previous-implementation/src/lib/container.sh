# Container lifecycle management: check, exec, start, recreate.

declare -a DOCKER_EXEC_CMD=()

container::exists() {
    docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"
}

container::is_running() {
    docker ps --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"
}

container::remove() {
    log::info "Removing container ${CONTAINER_NAME}..."
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
}

container::check_version() {
    local container_ver
    container_ver=$(docker inspect \
        --format '{{index .Config.Labels "vibrator.version"}}' \
        "$CONTAINER_NAME" 2>/dev/null || echo "unknown")

    if [[ "$container_ver" != "$VIBRATOR_VERSION" ]]; then
        log::warn "Version mismatch:"
        log::warn "  Container: ${container_ver}"
        log::warn "  Script:    ${VIBRATOR_VERSION}"
        log::info "To upgrade: vibrate --recreate --rebuild"
    fi
}

container::_build_exec_cmd() {
    local -a cmd=(docker exec)
    [[ "$INTERACTIVE" == true ]] && cmd+=(-it)

    # Passthrough args
    for arg in ${PASSTHROUGH_ARGS[@]+"${PASSTHROUGH_ARGS[@]}"}; do
        cmd+=("$arg")
    done

    # Forwarded environment variables
    local _seen=""
    for var in ${FORWARDED_ENV[@]+"${FORWARDED_ENV[@]}"}; do
        if [[ -n "${!var:-}" ]] && [[ ":$_seen:" != *":$var:"* ]]; then
            cmd+=(-e "$var=${!var}")
            _seen+=":$var"
        fi
    done

    cmd+=("$CONTAINER_NAME" /usr/local/bin/claude-exec)

    for arg in ${REMAINING_ARGS[@]+"${REMAINING_ARGS[@]}"}; do
        cmd+=("$arg")
    done

    DOCKER_EXEC_CMD=("${cmd[@]}")
}

container::exec_running() {
    log::info "Container ${CONTAINER_NAME} is running. Attaching..."
    container::_build_exec_cmd
    exec "${DOCKER_EXEC_CMD[@]}"
}

container::start_stopped() {
    log::info "Container ${CONTAINER_NAME} is stopped. Starting..."
    # Start container in background, then exec through claude-exec
    docker start "$CONTAINER_NAME" >/dev/null
    container::_build_exec_cmd
    exec "${DOCKER_EXEC_CMD[@]}"
}

container::handle_existing() {
    container::check_version

    if container::is_running; then
        container::exec_running
    else
        container::start_stopped
    fi
    # Both paths call exec, so we never return here
}

container::handle_rm_conflict() {
    if container::exists; then
        log::error "Container ${CONTAINER_NAME} already exists."
        log::info "The --rm flag creates temporary containers, but one already exists."
        log::info ""
        log::info "Options:"
        log::info "  Remove --rm to reuse the existing container"
        log::info "  Use --recreate --rm to replace it"
        exit 1
    fi
}
