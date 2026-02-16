# Configuration defaults and environment variable loading.
# Sets all global state used throughout the script.

config::init() {
    # User identity
    CFG_USERNAME="$(whoami)"

    # Paths
    # Get absolute canonical path (resolves symlinks, normalizes ..)
    WORKSPACE="$(realpath "$(pwd)")"
    CLAUDE_CONFIG="$HOME/.claude"

    # Image naming
    IMAGE_NAME="claude-vb-${CFG_USERNAME}:latest"

    # Feature flags
    INTERACTIVE=true
    REMOVE_AFTER=false
    PRIVILEGED=false
    DANGEROUS=true
    NO_AGENTS=false
    VERBOSE=false
    INSTALL_PLUGINS=true
    DOCKER_IN_DOCKER=false
    MCP_HUB=false

    # Build/action flags
    FLAG_BUILD_ONLY=false
    FLAG_REBUILD=false
    FLAG_RECREATE=false
    EXPORT_DOCKERFILE=""

    # Resource limits
    MEMORY_LIMIT=""
    CPU_LIMIT=""

    # OS detection (canonical method)
    IS_MACOS=false
    if [[ "$(uname -s)" == "Darwin" ]]; then
        IS_MACOS=true
    fi

    # Selective mount: true on macOS (path incompatibility), false on Linux
    SELECTIVE_MOUNT="$IS_MACOS"
    MOUNT_FULL_CONFIG=false

    # Environment forwarding list
    FORWARDED_ENV=(
        ANTHROPIC_API_KEY
        OPENAI_API_KEY
        ANTHROPIC_MODEL
        CLAUDE_CODE_OAUTH_TOKEN
        TERM
    )

    # Extra volumes (populated by --aws etc.)
    EXTRA_VOLUMES=()

    # Docker passthrough args (after --)
    PASSTHROUGH_ARGS=()

    # Command to run inside container (remaining positional args)
    REMAINING_ARGS=()
}

config::load_oauth_token() {
    local token_file="$HOME/.claude-docker-token"
    if [[ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" && -f "$token_file" ]]; then
        CLAUDE_CODE_OAUTH_TOKEN=$(tr -d '[:space:]' < "$token_file")
        [[ -n "$CLAUDE_CODE_OAUTH_TOKEN" ]] && export CLAUDE_CODE_OAUTH_TOKEN
    fi
}

config::derive_container_name() {
    local two_parts
    two_parts=$(echo "$WORKSPACE" | awk -F'/' '{if(NF>=2) print $(NF-1)"/"$NF; else print $NF}')
    local sanitized
    sanitized=$(echo "$two_parts" | sed 's/[^a-zA-Z0-9_-]/-/g')
    local hash
    hash=$(echo "$WORKSPACE" | sha256sum | cut -c1-8)
    CONTAINER_NAME="claude-vb-${sanitized}-${hash}"
}

config::apply_env_overrides() {
    [[ "${VIBRATOR_VERBOSE:-}" == "1" ]] && VERBOSE=true
    [[ -n "${VIBRATOR_IMAGE:-}" ]] && IMAGE_NAME="${VIBRATOR_IMAGE}:latest"

    # Override selective mount if user explicitly requested full mount
    [[ "$MOUNT_FULL_CONFIG" == true ]] && SELECTIVE_MOUNT=false

    # Extra env vars from VIBRATOR_EXTRA_ENV
    if [[ -n "${VIBRATOR_EXTRA_ENV:-}" ]]; then
        local -a extra
        IFS=' ' read -ra extra <<< "$VIBRATOR_EXTRA_ENV"
        for var in "${extra[@]}"; do
            if [[ "$var" == !* ]]; then
                config::remove_forwarded_env "${var#!}"
            else
                config::add_forwarded_env "$var"
            fi
        done
    fi
}

config::add_forwarded_env() {
    FORWARDED_ENV+=("$1")
}

config::remove_forwarded_env() {
    local remove="$1"
    local -a new=()
    for v in ${FORWARDED_ENV[@]+"${FORWARDED_ENV[@]}"}; do
        [[ "$v" != "$remove" ]] && new+=("$v")
    done
    FORWARDED_ENV=(${new[@]+"${new[@]}"})
}
