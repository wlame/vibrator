# Portable argument parsing (no GNU getopt dependency).

args::usage() {
    cat >&2 <<'USAGE'
Usage: vibrate [OPTIONS] [COMMAND...]

Run Claude Code in a Docker container with pre-configured MCP servers.

Options:
  -E, --forward-env VAR    Forward environment variable (use !VAR to exclude)
  --config PATH            Claude config directory (default: ~/.claude)
  --workspace PATH         Workspace directory to mount (default: current directory)
                           Mounted at the SAME PATH inside container as on host
                           Example: /home/user/project → /home/user/project
  --image NAME             Docker image name (default: claude-vb-<user>:latest)
  --name NAME              Container name (auto-generated from workspace path)
  --verbose                Show detailed output and Docker commands
  --help                   Display help message

  --aws                    Enable AWS integration (credentials + ~/.aws mount)
  --generic                Build generic image (no host Claude config baked in)
                           Auto-enabled when no Claude Code installation is detected
  --build                  Build Docker image and exit
  --pull [TAG]             Pull pre-built image from ghcr.io (default: latest)
  --export-dockerfile FILE Write generated Dockerfile to file
  --mount-full-config      Mount entire ~/.claude directory (overrides selective mode)
  --ssh-gpg-agents         Enable SSH and GPG agent forwarding (disabled by default)
  --no-plugins             Skip Claude plugin installation
  --non-interactive        Disable interactive mode (no TTY)
  --privileged             Enable Docker privileged mode
  --aider                  Install aider AI coding assistant (~80MB, opt-in)
                           (alias for --with-aider)
  --teams                  Enable Claude Code agent teams mode (experimental)
  --simple, --no-tools     Build minimal image (alias for --profile minimal)

Build feature profiles:
  --profile NAME           Preset for which features to install. One of:
                             minimal       Only dev-cli (~150MB)
                             backend       No Playwright, no audit toolkit (~600MB)
                             default       Everything except aider (~2GB) [default]
                             kitchen-sink  Everything including aider
  --with-FEATURE           Enable a specific build-time feature
  --no-FEATURE             Disable a specific build-time feature
  --explain                Print the resolved feature set and exit (dry-run)
  --no-menu                Skip the interactive workspace picker (also: VIBRATOR_NO_MENU=1)
  --upgrade-claude         Rebuild every vibrator image whose baked Claude CLI version
                           differs from the current CLAUDE_CLI_VERSION (cache-friendly)
  --claude-mem-setup       Print the claude-mem host stack setup instructions and exit
  --claude-mem-status      Probe the host claude-mem stack and report wiring for cwd
  --claude-mem-bootstrap   Mint a project-scoped key for this workspace (no container)

Available features (toggle with --with-* / --no-*):
  playwright       Chromium + Playwright MCP (~500MB)
  audit-toolkit    trivy, syft, grype, semgrep, gitleaks, ... (~400MB, needs python)
  python           Python 3.13 via uv (~100MB)
  go               Go toolchain (~200MB)
  gh               GitHub CLI
  dev-cli          jq, yq, fzf, fd, ripgrep, tree, httpie, websocat, csvkit, delta, lazygit, glow
  serena           Serena MCP runtime wiring (needs python)
  claude-mem       claude-mem plugin bind-mount + server-beta env forwarding
  codex            OpenAI Codex CLI (used by /planning:exec)
  aider            aider AI pair programming (off by default)

Dependencies are auto-resolved: --with-serena will also enable python if it
was disabled. To opt out fully, pass both --no-python and --no-serena etc.
  --mount HOST:CONTAINER   Mount additional directory (can be repeated, e.g. /data:/data:ro)
  --dind, --docker         Enable Docker-in-Docker mode (mount socket, elevated privileges)
  --docker-socket PATH     Override Docker socket path (auto-detected by default)
  --colima-profile NAME    Colima profile name (default: default)
  --rebuild                Rebuild image from scratch (ignore cache)
  --recreate               Delete and recreate container
  --rm                     Auto-remove container on exit
  --username NAME          Container username (default: current user)
  --memory LIMIT           Memory limit (e.g., 2g, 512m)
  --cpu COUNT              CPU limit (e.g., 2, 0.5, 1.5)
  --version                Show version information
  --                       Pass remaining arguments to docker run/exec

Examples:
  vibrate                                    Interactive shell
  vibrate claude "help me"                   Run claude with prompt
  vibrate --workspace /path/to/project       Custom workspace
  vibrate --build                            Build image only
  vibrate --rebuild                          Force rebuild and run
  vibrate --rm --non-interactive claude auth status

Environment variables:
  VIBRATOR_IMAGE           Override default Docker Hub image
  VIBRATOR_VERBOSE         Set to 1 for verbose output
  VIBRATOR_EXTRA_ENV       Space-separated list of extra env vars to forward
  VIBRATOR_DOCKER_SOCKET   Override Docker socket path (same as --docker-socket)
  COLIMA_PROFILE           Colima profile name (same as --colima-profile)

Docker Runtime:
  Vibrator auto-detects your Docker runtime (Docker Desktop, OrbStack, Colima,
  Rancher Desktop, Podman). Use --docker-socket to override auto-detection.

  Supported runtimes:
    - Docker Desktop: ~/.docker/run/docker.sock
    - OrbStack:       ~/.orbstack/run/docker.sock
    - Colima:         ~/.colima/default/docker.sock (or custom profile)
    - Rancher Desktop: ~/.rd/docker.sock
    - Podman:         ~/.local/share/containers/podman/machine/podman.sock
    - Native Linux:   /var/run/docker.sock

Note: SSH and GPG agents are NOT forwarded by default for security.
      Use --ssh-gpg-agents to enable forwarding.
USAGE
}

args::parse() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --workspace)          WORKSPACE="$(realpath "$2")"; shift 2 ;;
            --config)             CLAUDE_CONFIG="$2"; shift 2 ;;
            --name)               CONTAINER_NAME="$2"; shift 2 ;;
            --image)              IMAGE_NAME="$2"; shift 2 ;;
            -E|--forward-env)
                if [[ "$2" == !* ]]; then
                    config::remove_forwarded_env "${2#!}"
                else
                    config::add_forwarded_env "$2"
                fi
                shift 2 ;;
            --verbose)            VERBOSE=true; shift ;;
            --help)               args::usage; exit 0 ;;
            --rm)                 REMOVE_AFTER=true; shift ;;
            --non-interactive)    INTERACTIVE=false; shift ;;
            --privileged)         PRIVILEGED=true; shift ;;
            --aider)              AIDER=true; config::feature_enable aider; USER_SPECIFIED_FEATURES=true; shift ;;
            --teams)              AGENT_TEAMS=true; shift ;;
            --simple|--no-tools)  SIMPLE_BUILD=true; config::apply_profile minimal; USER_SPECIFIED_FEATURES=true; shift ;;
            --generic)            GENERIC=true; shift ;;
            --mount)              EXTRA_VOLUMES+=("$2"); shift 2 ;;
            --dind|--docker)      DOCKER_IN_DOCKER=true; shift ;;
            --docker-socket)      VIBRATOR_DOCKER_SOCKET="$2"; shift 2 ;;
            --colima-profile)     COLIMA_PROFILE="$2"; shift 2 ;;
            --ssh-gpg-agents)     FORWARD_AGENTS=true; shift ;;
            --build)              FLAG_BUILD_ONLY=true; shift ;;
            --pull)
                # Optional tag argument (default: latest)
                if [[ -n "${2:-}" && "$2" != -* ]]; then
                    FLAG_PULL="$2"; shift 2
                else
                    FLAG_PULL="latest"; shift
                fi ;;
            --rebuild)            FLAG_REBUILD=true; shift ;;
            --recreate)           FLAG_RECREATE=true; shift ;;
            --export-dockerfile)  EXPORT_DOCKERFILE="$2"; shift 2 ;;
            --username)           CFG_USERNAME="$2"; shift 2 ;;
            --aws)                args::_enable_aws; shift ;;
            --no-plugins)         INSTALL_PLUGINS=false; shift ;;
            --mount-full-config)  MOUNT_FULL_CONFIG=true; shift ;;
            --memory)             MEMORY_LIMIT="$2"; shift 2 ;;
            --cpu)                CPU_LIMIT="$2"; shift 2 ;;
            --version)            args::_show_version; exit 0 ;;
            --profile)            config::apply_profile "$2"; USER_SPECIFIED_FEATURES=true; shift 2 ;;
            --explain)            FLAG_EXPLAIN_FEATURES=true; shift ;;
            --no-menu)            FLAG_NO_MENU=true; shift ;;
            --upgrade-claude)     FLAG_UPGRADE_CLAUDE=true; shift ;;
            --claude-mem-setup)     FLAG_CLAUDE_MEM_SETUP=true; shift ;;
            --claude-mem-status)    FLAG_CLAUDE_MEM_STATUS=true; shift ;;
            --claude-mem-bootstrap) FLAG_CLAUDE_MEM_BOOTSTRAP=true; shift ;;
            # Feature toggles. Listed AFTER --no-plugins / --no-menu / --no-tools
            # so those specific cases win before the wildcard match.
            --with-*)
                _feat="${1#--with-}"
                config::is_known_feature "$_feat" \
                    || log::die "Unknown feature: '$_feat' (valid: ${FEATURE_CATALOG[*]})"
                config::feature_enable "$_feat"
                [[ "$_feat" == "aider" ]] && AIDER=true  # mirror legacy flag
                USER_SPECIFIED_FEATURES=true
                shift ;;
            --no-*)
                _feat="${1#--no-}"
                config::is_known_feature "$_feat" \
                    || log::die "Unknown feature: '$_feat' (valid: ${FEATURE_CATALOG[*]})"
                config::feature_disable "$_feat"
                [[ "$_feat" == "aider" ]] && AIDER=false
                USER_SPECIFIED_FEATURES=true
                shift ;;
            --)                   shift; break ;;
            -*)                   log::die "Unknown option: $1" ;;
            *)                    break ;;
        esac
    done

    REMAINING_ARGS=("$@")

    # Resolve feature dependencies (e.g., enabling serena force-enables python).
    config::validate_features

    # --explain-features short-circuit: print resolved state and exit cleanly.
    if [[ "${FLAG_EXPLAIN_FEATURES:-false}" == true ]]; then
        config::print_features
        exit 0
    fi
}

args::_show_version() {
    cat <<EOF
vibrator version $VIBRATOR_VERSION

Docker runner for Claude Code with:
  • Auto-detection of Docker runtimes
  • Pre-configured MCP servers (Serena, Context7)
  • Graduated privilege system for security

Repository: https://github.com/wlame/vibrator
EOF
}

args::_enable_aws() {
    local -a aws_vars=(
        AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN
        AWS_REGION AWS_DEFAULT_REGION AWS_PROFILE
    )
    for v in "${aws_vars[@]}"; do
        config::add_forwarded_env "$v"
    done
    [[ -d "$HOME/.aws" ]] && EXTRA_VOLUMES+=("$HOME/.aws:/home/$CFG_USERNAME/.aws:ro")
}
