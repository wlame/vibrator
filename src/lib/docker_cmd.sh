# Build the docker run command as a proper bash array.
# No string concatenation - eliminates quoting and word-splitting bugs.

declare -a DOCKER_RUN_CMD=()

docker_cmd::build() {
    local -a cmd=(docker run)

    # Lifecycle
    [[ "$REMOVE_AFTER" == true ]] && cmd+=(--rm)
    [[ "$INTERACTIVE" == true ]] && cmd+=(-it)

    # Security model
    if [[ "$PRIVILEGED" == true ]]; then
        cmd+=(--privileged)
    elif [[ "$DOCKER_IN_DOCKER" == true ]]; then
        # Docker-in-Docker mode: elevated privileges for container operations
        cmd+=(
            --cap-add=SYS_ADMIN
            --security-opt seccomp=unconfined
            --shm-size=2g
        )
        log::verbose "Docker-in-Docker mode enabled (elevated privileges)"
    else
        # Default: minimal privileges (secure by default)
        cmd+=(
            --security-opt no-new-privileges
            --shm-size=2g
        )
        log::verbose "Running in secure mode (minimal privileges)"
    fi

    cmd+=(--init)
    cmd+=(--name "$CONTAINER_NAME")
    cmd+=(--network host)

    # Resource limits
    [[ -n "$MEMORY_LIMIT" ]] && cmd+=(--memory "$MEMORY_LIMIT")
    [[ -n "$CPU_LIMIT" ]]    && cmd+=(--cpus "$CPU_LIMIT")

    # Labels
    cmd+=(
        --label "vibrator.managed=true"
        --label "vibrator.workspace=$WORKSPACE"
        --label "vibrator.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        --label "vibrator.version=$VIBRATOR_VERSION"
        --label "vibrator.username=$CFG_USERNAME"
    )

    # Core environment
    local ws_basename
    ws_basename=$(basename "$WORKSPACE")
    cmd+=(
        -e "WORKSPACE_PATH=/home/$CFG_USERNAME/$ws_basename"
        -e "CLAUDE_CONFIG_PATH=/home/$CFG_USERNAME/.claude"
        -e "CONTAINER_USER=$CFG_USERNAME"
    )

    [[ "$DANGEROUS" == true ]]       && cmd+=(-e "VIBRATOR_DANGEROUS=1")
    [[ "$VERBOSE" == true ]]         && cmd+=(-e "VIBRATOR_VERBOSE=1")
    [[ "$DOCKER_IN_DOCKER" == true ]] && cmd+=(-e "VIBRATOR_DOCKER_IN_DOCKER=1")

    # Forwarded environment variables
    docker_cmd::_add_forwarded_env

    # Volume mounts
    docker_cmd::_add_volumes

    # Extra volumes (e.g., --aws)
    for vol in ${EXTRA_VOLUMES[@]+"${EXTRA_VOLUMES[@]}"}; do
        cmd+=(-v "$vol")
    done

    # Image
    cmd+=("$IMAGE_NAME")

    # Passthrough args (from --)
    for arg in ${PASSTHROUGH_ARGS[@]+"${PASSTHROUGH_ARGS[@]}"}; do
        cmd+=("$arg")
    done

    # Command args (positional)
    for arg in ${REMAINING_ARGS[@]+"${REMAINING_ARGS[@]}"}; do
        cmd+=("$arg")
    done

    DOCKER_RUN_CMD=("${cmd[@]}")
}

docker_cmd::_add_forwarded_env() {
    local _seen=""
    for var in ${FORWARDED_ENV[@]+"${FORWARDED_ENV[@]}"}; do
        if [[ -n "${!var:-}" ]] && [[ ":$_seen:" != *":$var:"* ]]; then
            cmd+=(-e "$var=${!var}")
            _seen+=":$var"
        fi
    done
}

docker_cmd::_add_volumes() {
    local ws_basename
    ws_basename=$(basename "$WORKSPACE")

    # Host claude.json for config merging in entrypoint
    if [[ -f "$HOME/.claude.json" ]]; then
        cmd+=(-v "$HOME/.claude.json:/home/$CFG_USERNAME/.claude.host.json:ro")
        log::verbose "Host Claude config detected, will be mounted for merging"
    fi

    # Workspace (always mounted)
    cmd+=(-v "$WORKSPACE:/home/$CFG_USERNAME/$ws_basename")

    # Claude config directory
    if [[ "$SELECTIVE_MOUNT" == true ]]; then
        log::verbose "Using selective config mount (macOS mode)"
        [[ -f "$CLAUDE_CONFIG/settings.json" ]] && \
            cmd+=(-v "$CLAUDE_CONFIG/settings.json:/home/$CFG_USERNAME/.claude/settings.json:ro")
        # Mount host rules to separate location for merging with container rules
        [[ -d "$CLAUDE_CONFIG/rules" ]] && \
            cmd+=(-v "$CLAUDE_CONFIG/rules:/home/$CFG_USERNAME/.claude/rules-host:ro")
    else
        log::verbose "Using full config mount (Linux mode)"
        # In full mount mode, mount rules separately for merging
        if [[ -d "$CLAUDE_CONFIG/rules" ]]; then
            cmd+=(-v "$CLAUDE_CONFIG/rules:/home/$CFG_USERNAME/.claude/rules-host:ro")
            # Mount config without rules subdirectory
            cmd+=(-v "$CLAUDE_CONFIG:/home/$CFG_USERNAME/.claude-full")
        else
            cmd+=(-v "$CLAUDE_CONFIG:/home/$CFG_USERNAME/.claude")
        fi
    fi

    # SSH keys (read-only)
    [[ -d "$HOME/.ssh" ]] && \
        cmd+=(-v "$HOME/.ssh:/home/$CFG_USERNAME/.ssh:ro")

    # Git config (read-only)
    [[ -f "$HOME/.gitconfig" ]] && \
        cmd+=(-v "$HOME/.gitconfig:/home/$CFG_USERNAME/.gitconfig:ro")

    # Docker socket (only when --dind/--docker enabled)
    if [[ "$DOCKER_IN_DOCKER" == true ]]; then
        if [[ -S "/var/run/docker.sock" ]]; then
            cmd+=(-v "/var/run/docker.sock:/var/run/docker.sock")
            log::verbose "Docker socket mounted for Docker-in-Docker"
        else
            log::warn "Docker-in-Docker requested but /var/run/docker.sock not found"
        fi
    fi

    # SSH agent socket (auto-detect and forward unless --no-agents)
    if [[ "$NO_AGENTS" != true ]]; then
        if [[ -n "${SSH_AUTH_SOCK:-}" && -S "${SSH_AUTH_SOCK:-}" ]]; then
            local resolved
            resolved=$(readlink -f "$SSH_AUTH_SOCK" 2>/dev/null || echo "$SSH_AUTH_SOCK")
            cmd+=(-v "$resolved:/ssh-agent" -e "SSH_AUTH_SOCK=/ssh-agent")
            log::verbose "SSH agent socket forwarded"
        fi

        # GPG agent socket (auto-detect and forward)
        if command -v gpgconf >/dev/null 2>&1; then
            local gpg_socket
            gpg_socket=$(gpgconf --list-dirs agent-extra-socket 2>/dev/null || true)
            if [[ -S "${gpg_socket:-}" ]]; then
                cmd+=(-v "$gpg_socket:/gpg-agent-extra")
                log::verbose "GPG agent socket forwarded"
            fi
        fi
    fi
}

docker_cmd::print() {
    if [[ ${#DOCKER_RUN_CMD[@]} -gt 0 ]]; then
        log::docker_cmd "${DOCKER_RUN_CMD[@]}"
    fi
}
