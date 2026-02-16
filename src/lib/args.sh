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
  --build                  Build Docker image and exit
  --export-dockerfile FILE Write generated Dockerfile to file
  --mount-full-config      Mount entire ~/.claude directory (overrides selective mode)
  --no-agents              Disable SSH and GPG agent forwarding
  --no-plugins             Skip Claude plugin installation
  --non-interactive        Disable interactive mode (no TTY)
  --privileged             Enable Docker privileged mode
  --mcp                    Start agent-browser MCP hub (Web UI at localhost:8080)
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

Note: SSH and GPG agents are automatically detected and forwarded if available.
      Use --no-agents to disable this behavior.
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
            --mcp)                MCP_HUB=true; shift ;;
            --dind|--docker)      DOCKER_IN_DOCKER=true; shift ;;
            --docker-socket)      VIBRATOR_DOCKER_SOCKET="$2"; shift 2 ;;
            --colima-profile)     COLIMA_PROFILE="$2"; shift 2 ;;
            --no-agents)          NO_AGENTS=true; shift ;;
            --build)              FLAG_BUILD_ONLY=true; shift ;;
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
            --)                   shift; break ;;
            -*)                   log::die "Unknown option: $1" ;;
            *)                    break ;;
        esac
    done

    REMAINING_ARGS=("$@")
}

args::_show_version() {
    cat <<EOF
vibrator version $VIBRATOR_VERSION

Docker runner for Claude Code with:
  • Auto-detection of Docker runtimes
  • Pre-configured MCP servers (Serena, Context7, Playwright)
  • Optional Langfuse observability integration
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
