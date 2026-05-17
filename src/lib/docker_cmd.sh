# Build the docker run command as a proper bash array.
# No string concatenation - eliminates quoting and word-splitting bugs.

declare -a DOCKER_RUN_CMD=()

docker_cmd::build() {
    # Detect Docker runtime and socket path
    if ! docker_runtime::detect; then
        log::error "Failed to detect Docker runtime"
        return 1
    fi

    local docker_socket
    docker_socket=$(docker_runtime::get_socket)

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
    [[ -n "$CPU_LIMIT" ]]    && cmd+=(--cpu "$CPU_LIMIT")

    # Labels
    cmd+=(
        --label "vibrator.managed=true"
        --label "vibrator.workspace=$WORKSPACE"
        --label "vibrator.created=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        --label "vibrator.version=$VIBRATOR_VERSION"
        --label "vibrator.username=$CFG_USERNAME"
    )

    # Core environment
    cmd+=(
        -e "WORKSPACE_PATH=$WORKSPACE"
        -e "CLAUDE_CONFIG_PATH=/home/$CFG_USERNAME/.claude"
        -e "CONTAINER_USER=$CFG_USERNAME"
    )

    [[ "$DANGEROUS" == true ]]       && cmd+=(-e "VIBRATOR_DANGEROUS=1")
    [[ "$VERBOSE" == true ]]         && cmd+=(-e "VIBRATOR_VERBOSE=1")
    [[ "$DOCKER_IN_DOCKER" == true ]] && cmd+=(-e "VIBRATOR_DOCKER_IN_DOCKER=1")
    [[ "$AIDER" == true ]]           && cmd+=(-e "VIBRATOR_AIDER=1")
    [[ "$GENERIC" == true ]]         && cmd+=(-e "VIBRATOR_GENERIC=1")
    [[ "$AGENT_TEAMS" == true ]]     && cmd+=(-e "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1")
    [[ "$SIMPLE_BUILD" == true ]]    && cmd+=(-e "VIBRATOR_SIMPLE=1")

    # Forwarded environment variables
    docker_cmd::_add_forwarded_env

    # Optional: forward claude-mem server-beta config (URL, API key, project).
    # Read from ~/.config/vibrator/claude-mem.env if present.
    docker_cmd::_add_claude_mem_server_beta

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

# Read one key from a dotenv-style file in a subshell so the value never
# pollutes the host environment. Empty stdout = key unset.
docker_cmd::_dotenv_get() {
    local cfg_file="$1" key="$2"
    [[ -f "$cfg_file" ]] || return 0
    (
        # shellcheck disable=SC1090
        . "$cfg_file" >/dev/null 2>&1
        printf '%s' "${!key:-}"
    )
}

# Forward claude-mem server-beta connection details into the container.
#
# Three-path dispatch, first match wins:
#
#   A. EXPLICIT — the admin dotenv (~/.config/vibrator/claude-mem.env) or
#      the workspace .vb.env (loaded by config::load_pin_file) carries a
#      CLAUDE_MEM_SERVER_BETA_API_KEY already. Forward it as-is. Covers
#      both the legacy team-wide-key flow and the cached-after-bootstrap
#      flow — from this function's POV they're identical.
#
#   B. BOOTSTRAP — admin dotenv has CLAUDE_MEM_SERVER_DATABASE_URL but no
#      API key, and the workspace .vb.env has no cached key either. Run
#      claude_mem_bootstrap::run to mint a project-scoped key against the
#      DB and persist it to $WORKSPACE/.vb.env. The DB URL stays HOST-SIDE
#      ONLY — it is never forwarded to the container.
#
#   C. SKIP — admin dotenv missing or empty. Silently no-op, vibrator
#      remains usable for users who don't run claude-mem.
#
# Required forwarded variables (always when path A or B succeeds):
#   CLAUDE_MEM_RUNTIME=server-beta
#   CLAUDE_MEM_SERVER_BETA_URL=http://host.docker.internal:37877
#   CLAUDE_MEM_SERVER_BETA_API_KEY=cmem_<48-hex>
#
# Optional forwarded variables (always when set):
#   CLAUDE_MEM_SERVER_BETA_TEAM_ID=<uuid>
#   CLAUDE_MEM_SERVER_BETA_PROJECT_ID=<uuid>
#
# DATABASE_URL is NEVER forwarded — full DB credentials stay on the host.
docker_cmd::_add_claude_mem_server_beta() {
    local cfg_file="${VIBRATOR_CLAUDE_MEM_ENV:-$HOME/.config/vibrator/claude-mem.env}"
    [[ -f "$cfg_file" ]] || return 0

    # Read the admin dotenv values up-front. The workspace .vb.env was
    # already sourced by config::load_pin_file (which now whitelists the
    # CLAUDE_MEM_* keys), so $CLAUDE_MEM_SERVER_BETA_API_KEY may already
    # be set in OUR shell env from there.
    local runtime url admin_api_key admin_team_id admin_project_id database_url
    runtime=$(docker_cmd::_dotenv_get        "$cfg_file" CLAUDE_MEM_RUNTIME)
    url=$(docker_cmd::_dotenv_get            "$cfg_file" CLAUDE_MEM_SERVER_BETA_URL)
    admin_api_key=$(docker_cmd::_dotenv_get  "$cfg_file" CLAUDE_MEM_SERVER_BETA_API_KEY)
    admin_team_id=$(docker_cmd::_dotenv_get  "$cfg_file" CLAUDE_MEM_SERVER_BETA_TEAM_ID)
    admin_project_id=$(docker_cmd::_dotenv_get "$cfg_file" CLAUDE_MEM_SERVER_BETA_PROJECT_ID)
    database_url=$(docker_cmd::_dotenv_get   "$cfg_file" CLAUDE_MEM_SERVER_DATABASE_URL)

    if [[ -z "$runtime" || -z "$url" ]]; then
        log::verbose "claude-mem: $cfg_file missing RUNTIME or SERVER_BETA_URL — skipping"
        return 0
    fi

    # Resolve which API key (and ids) to forward, in priority order.
    local api_key team_id project_id source
    if [[ -n "${CLAUDE_MEM_SERVER_BETA_API_KEY:-}" ]]; then
        # Already set from the workspace .vb.env via config::load_pin_file —
        # this is the post-bootstrap cache hit.
        api_key="$CLAUDE_MEM_SERVER_BETA_API_KEY"
        team_id="${CLAUDE_MEM_SERVER_BETA_TEAM_ID:-$admin_team_id}"
        project_id="${CLAUDE_MEM_SERVER_BETA_PROJECT_ID:-$admin_project_id}"
        source="workspace .vb.env"
    elif [[ -n "$admin_api_key" ]]; then
        # Legacy: user pre-minted a team-wide key and pinned it in admin dotenv.
        api_key="$admin_api_key"
        team_id="$admin_team_id"
        project_id="$admin_project_id"
        source="admin dotenv"
    elif [[ -n "$database_url" ]]; then
        # Auto-bootstrap path. Mints + caches a project-scoped key, exports
        # the three vars for our shell, and persists to .vb.env.
        log::info "claude-mem: no cached key for this workspace — bootstrapping…"
        if claude_mem_bootstrap::run "$url" "$database_url"; then
            api_key="$CLAUDE_MEM_SERVER_BETA_API_KEY"
            team_id="$CLAUDE_MEM_SERVER_BETA_TEAM_ID"
            project_id="$CLAUDE_MEM_SERVER_BETA_PROJECT_ID"
            source="auto-bootstrap"
        else
            log::warn "claude-mem: bootstrap failed — container will start without server-beta wiring"
            return 0
        fi
    else
        log::verbose "claude-mem: no API key and no DATABASE_URL — skipping"
        return 0
    fi

    cmd+=(
        -e "CLAUDE_MEM_RUNTIME=$runtime"
        -e "CLAUDE_MEM_SERVER_BETA_URL=$url"
        -e "CLAUDE_MEM_SERVER_BETA_API_KEY=$api_key"
    )
    [[ -n "$team_id" ]]    && cmd+=(-e "CLAUDE_MEM_SERVER_BETA_TEAM_ID=$team_id")
    [[ -n "$project_id" ]] && cmd+=(-e "CLAUDE_MEM_SERVER_BETA_PROJECT_ID=$project_id")
    # NOTE: CLAUDE_MEM_SERVER_DATABASE_URL is intentionally NOT forwarded.
    # The container has no business knowing the DB credentials — the only
    # SQL we run lives on the host (claude_mem_bootstrap), and after that
    # the container talks pure HTTP to /v1/* with its project-scoped key.

    log::verbose "claude-mem: forwarding server-beta config ($url, key source: $source)"
}

docker_cmd::_add_volumes() {
    # Workspace (always mounted at same path as host)
    cmd+=(-v "$WORKSPACE:$WORKSPACE")

    # Host Claude config mounts (skipped in generic mode)
    if [[ "$GENERIC" == false ]]; then
        # Host claude.json for config merging in entrypoint
        if [[ -f "$HOME/.claude.json" ]]; then
            cmd+=(-v "$HOME/.claude.json:/home/$CFG_USERNAME/.claude.host.json:ro")
            log::verbose "Host Claude config detected, will be mounted for merging"
        fi

        # Claude config directory
        if [[ "$SELECTIVE_MOUNT" == true ]]; then
            log::verbose "Using selective config mount (macOS mode)"
            # Mount as settings.host.json so entrypoint can copy and modify
            [[ -f "$CLAUDE_CONFIG/settings.json" ]] && \
                cmd+=(-v "$CLAUDE_CONFIG/settings.json:/home/$CFG_USERNAME/.claude/settings.host.json:ro")
            # Mount host rules to separate location for merging with container rules
            [[ -d "$CLAUDE_CONFIG/rules" ]] && \
                cmd+=(-v "$CLAUDE_CONFIG/rules:/home/$CFG_USERNAME/.claude/rules-host:ro")
            # Mount hooks directory so hook scripts referenced in settings.json work in container
            [[ -d "$CLAUDE_CONFIG/hooks" ]] && \
                cmd+=(-v "$CLAUDE_CONFIG/hooks:/home/$CFG_USERNAME/.claude/hooks:ro")
            # claude-mem is installed at Docker build time via `npx claude-mem install`
            # (see Dockerfile.template). We deliberately do NOT bind-mount the host's
            # marketplace dir: the installer writes into it (e.g. .agents/), which
            # fails on a RO bind-mount. The container has its own fresh install.
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
    else
        log::verbose "Generic mode: skipping host Claude config mounts"
    fi

    # Codex CLI config and auth (read-write so OAuth token refresh persists
    # back to the host, matching the ~/.claude mount pattern). Holds
    # ~/.codex/auth.json (ChatGPT OAuth or API key) and config.toml.
    # Required for the "/planning:exec" skill's codex-review phase to
    # authenticate seamlessly using host credentials.
    [[ -d "$HOME/.codex" ]] && \
        cmd+=(-v "$HOME/.codex:/home/$CFG_USERNAME/.codex")

    # SSH keys (read-only)
    [[ -d "$HOME/.ssh" ]] && \
        cmd+=(-v "$HOME/.ssh:/home/$CFG_USERNAME/.ssh:ro")

    # Git config (read-only)
    [[ -f "$HOME/.gitconfig" ]] && \
        cmd+=(-v "$HOME/.gitconfig:/home/$CFG_USERNAME/.gitconfig:ro")

    # Docker socket (only when --dind/--docker enabled)
    if [[ "$DOCKER_IN_DOCKER" == true ]]; then
        if [[ -S "$docker_socket" ]]; then
            cmd+=(-v "$docker_socket:/var/run/docker.sock")
            log::verbose "Docker socket mounted for Docker-in-Docker: $docker_socket"
        else
            log::warn "Docker-in-Docker requested but Docker socket not found: $docker_socket"
        fi
    fi

    # SSH agent socket (opt-in with --ssh-gpg-agents)
    if [[ "$FORWARD_AGENTS" == true ]]; then
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
