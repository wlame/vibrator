#!/usr/bin/env bash
# docker_runtime.sh - Docker runtime detection for macOS compatibility
# Supports: Docker Desktop, Colima, Podman, OrbStack, Rancher Desktop

# Exported variables (set by docker_runtime::detect)
DETECTED_DOCKER_RUNTIME=""
DETECTED_DOCKER_SOCKET=""

# Detect Docker runtime and socket path
# Returns: 0 on success, 1 on failure
# Sets: DETECTED_DOCKER_RUNTIME and DETECTED_DOCKER_SOCKET
docker_runtime::detect() {
    local runtime_info
    runtime_info=$(docker_runtime::_detect_runtime)

    local runtime="${runtime_info%%:*}"
    local socket="${runtime_info#*:}"

    if [[ "$runtime" == "none" ]]; then
        log::error "No Docker runtime detected"
        log::info "Please install and start one of:"
        log::info "  - Docker Desktop (https://www.docker.com/products/docker-desktop)"
        log::info "  - OrbStack (https://orbstack.dev)"
        log::info "  - Colima (brew install colima)"
        log::info "  - Rancher Desktop (https://rancherdesktop.io)"
        log::info "  - Podman Desktop (https://podman-desktop.io)"
        return 1
    fi

    log::verbose "Detected Docker runtime: $runtime"
    log::verbose "Socket path: $socket"

    if ! docker_runtime::_verify_runtime "$runtime" "$socket"; then
        log::error "Docker runtime '$runtime' detected but not responsive"
        log::info "Please ensure the runtime is started and accessible"
        return 1
    fi

    # Export for use by other modules
    export DETECTED_DOCKER_RUNTIME="$runtime"
    export DETECTED_DOCKER_SOCKET="$socket"

    return 0
}

# Internal: Detect runtime and socket path
# Returns: "runtime:socket_path" or "none:"
docker_runtime::_detect_runtime() {
    # 1. Check user override via environment variable
    if [[ -n "$VIBRATOR_DOCKER_SOCKET" ]]; then
        if [[ -S "$VIBRATOR_DOCKER_SOCKET" ]]; then
            echo "custom:$VIBRATOR_DOCKER_SOCKET"
            return 0
        else
            log::warn "VIBRATOR_DOCKER_SOCKET set but socket not found: $VIBRATOR_DOCKER_SOCKET"
        fi
    fi

    # 2. Check DOCKER_HOST environment variable
    if [[ -n "$DOCKER_HOST" ]]; then
        local socket_path="${DOCKER_HOST#unix://}"
        if [[ -S "$socket_path" ]]; then
            local runtime
            runtime=$(docker_runtime::_identify_runtime_by_socket "$socket_path")
            echo "$runtime:$socket_path"
            return 0
        fi
    fi

    # 3. Check docker context (if docker CLI available)
    if command -v docker >/dev/null 2>&1; then
        local context_info
        context_info=$(docker_runtime::_detect_by_context)
        if [[ -n "$context_info" ]] && [[ "$context_info" != "none:" ]]; then
            local runtime="${context_info%%:*}"
            local socket="${context_info#*:}"
            if [[ -S "$socket" ]]; then
                echo "$context_info"
                return 0
            fi
        fi
    fi

    # 4. Socket file detection (priority order)
    docker_runtime::_detect_by_socket_presence
}

# Internal: Detect runtime by docker context
# Returns: "runtime:socket_path" or "none:"
docker_runtime::_detect_by_context() {
    local context_endpoint
    context_endpoint=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || echo "")

    if [[ -z "$context_endpoint" ]]; then
        echo "none:"
        return 1
    fi

    # Extract socket path and identify runtime
    local socket_path="${context_endpoint#unix://}"

    case "$context_endpoint" in
        *"desktop-linux"*|*".docker"*)
            echo "docker-desktop:$HOME/.docker/run/docker.sock"
            ;;
        *"orbstack"*|*".orbstack"*)
            echo "orbstack:$HOME/.orbstack/run/docker.sock"
            ;;
        *"colima"*|*".colima"*)
            # Try to detect profile from socket path
            local profile="default"
            if [[ "$socket_path" =~ \.colima/([^/]+)/ ]]; then
                profile="${BASH_REMATCH[1]}"
            fi
            echo "colima:$HOME/.colima/$profile/docker.sock"
            ;;
        *"rancher-desktop"*|*".rd"*)
            echo "rancher-desktop:$HOME/.rd/docker.sock"
            ;;
        *)
            # Unknown context, try to identify by socket path
            if [[ -n "$socket_path" ]]; then
                local runtime
                runtime=$(docker_runtime::_identify_runtime_by_socket "$socket_path")
                echo "$runtime:$socket_path"
            else
                echo "none:"
            fi
            ;;
    esac
}

# Internal: Detect runtime by socket file presence (priority order)
# Returns: "runtime:socket_path" or "none:"
docker_runtime::_detect_by_socket_presence() {
    # Docker Desktop (new location since 4.18)
    if [[ -S "$HOME/.docker/run/docker.sock" ]]; then
        echo "docker-desktop:$HOME/.docker/run/docker.sock"
        return 0
    fi

    # OrbStack
    if [[ -S "$HOME/.orbstack/run/docker.sock" ]]; then
        echo "orbstack:$HOME/.orbstack/run/docker.sock"
        return 0
    fi

    # Colima (check for profiles)
    if [[ -d "$HOME/.colima" ]]; then
        # Check for active profile (from COLIMA_PROFILE env or default)
        local profile="${COLIMA_PROFILE:-default}"
        if [[ -S "$HOME/.colima/$profile/docker.sock" ]]; then
            echo "colima:$HOME/.colima/$profile/docker.sock"
            return 0
        fi

        # Fallback: find any running colima profile
        local colima_socket
        colima_socket=$(find "$HOME/.colima" -name "docker.sock" -type s 2>/dev/null | head -n1)
        if [[ -n "$colima_socket" ]]; then
            echo "colima:$colima_socket"
            return 0
        fi
    fi

    # Rancher Desktop
    if [[ -S "$HOME/.rd/docker.sock" ]]; then
        echo "rancher-desktop:$HOME/.rd/docker.sock"
        return 0
    fi

    # Podman machine
    if [[ -S "$HOME/.local/share/containers/podman/machine/podman.sock" ]]; then
        echo "podman:$HOME/.local/share/containers/podman/machine/podman.sock"
        return 0
    fi

    # Standard location (Linux or symlink)
    if [[ -S "/var/run/docker.sock" ]]; then
        # Check if it's a symlink to determine actual runtime
        if [[ -L "/var/run/docker.sock" ]]; then
            local target
            if [[ "$(uname -s)" == "Darwin" ]]; then
                # macOS readlink doesn't have -f, use different approach
                target=$(readlink "/var/run/docker.sock")
            else
                target=$(readlink -f "/var/run/docker.sock")
            fi

            case "$target" in
                *".docker"*) echo "docker-desktop:/var/run/docker.sock" ;;
                *".orbstack"*) echo "orbstack:/var/run/docker.sock" ;;
                *".colima"*) echo "colima:/var/run/docker.sock" ;;
                *".rd"*) echo "rancher-desktop:/var/run/docker.sock" ;;
                *) echo "unknown:/var/run/docker.sock" ;;
            esac
        else
            echo "native:/var/run/docker.sock"
        fi
        return 0
    fi

    # No socket found
    echo "none:"
    return 1
}

# Internal: Identify runtime by socket path pattern
# Args: socket_path
# Returns: runtime name
docker_runtime::_identify_runtime_by_socket() {
    local socket_path="$1"

    case "$socket_path" in
        *".docker"*) echo "docker-desktop" ;;
        *".orbstack"*) echo "orbstack" ;;
        *".colima"*) echo "colima" ;;
        *".rd"*) echo "rancher-desktop" ;;
        *"podman"*) echo "podman" ;;
        "/var/run/docker.sock") echo "native" ;;
        *) echo "custom" ;;
    esac
}

# Internal: Verify runtime is running and accessible
# Args: runtime, socket_path
# Returns: 0 if verified, 1 if not
docker_runtime::_verify_runtime() {
    local runtime="$1"
    local socket_path="$2"

    case "$runtime" in
        docker-desktop)
            # Verify Docker Desktop is running
            if ! pgrep -f "Docker Desktop" >/dev/null 2>&1; then
                log::verbose "Docker Desktop process not found"
                # Don't fail - socket might still work
            fi
            ;;
        orbstack)
            # Verify OrbStack is running
            if ! pgrep -f "OrbStack" >/dev/null 2>&1; then
                log::verbose "OrbStack process not found"
            fi
            ;;
        colima)
            # Verify Colima VM is running
            if command -v colima >/dev/null 2>&1; then
                if ! colima status 2>/dev/null | grep -q "Running"; then
                    log::verbose "Colima not running according to colima status"
                fi
            fi
            ;;
        rancher-desktop)
            # Verify Rancher Desktop is running
            if ! pgrep -f "Rancher Desktop" >/dev/null 2>&1; then
                log::verbose "Rancher Desktop process not found"
            fi
            ;;
        podman)
            # Verify Podman machine is running
            if command -v podman >/dev/null 2>&1; then
                if ! podman machine list 2>/dev/null | grep -q "Running"; then
                    log::verbose "Podman machine not running"
                fi
            fi
            ;;
        native|unknown|custom)
            # Just verify socket is accessible
            if [[ ! -S "$socket_path" ]]; then
                log::verbose "Socket not accessible: $socket_path"
                return 1
            fi
            ;;
    esac

    # Final test: Can we connect to Docker API?
    if command -v docker >/dev/null 2>&1; then
        if ! docker -H "unix://$socket_path" info >/dev/null 2>&1; then
            log::verbose "Docker API not responsive at $socket_path"
            return 1
        fi
    else
        # No docker CLI, just check socket exists
        if [[ ! -S "$socket_path" ]]; then
            return 1
        fi
    fi

    return 0
}

# Get detected socket path
# Returns: socket path or default
docker_runtime::get_socket() {
    echo "${DETECTED_DOCKER_SOCKET:-/var/run/docker.sock}"
}

# Get detected runtime name
# Returns: runtime name or "unknown"
docker_runtime::get_name() {
    echo "${DETECTED_DOCKER_RUNTIME:-unknown}"
}

# Check if running on macOS
# Returns: 0 if macOS, 1 if not
docker_runtime::is_macos() {
    [[ "$(uname -s)" == "Darwin" ]]
}

# Show runtime information
docker_runtime::show_info() {
    local runtime socket
    runtime=$(docker_runtime::get_name)
    socket=$(docker_runtime::get_socket)

    log::info "Docker Runtime: $runtime"
    log::info "Socket: $socket"
}
