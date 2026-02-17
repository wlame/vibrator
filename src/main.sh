# Main execution flow. Wires all modules together.

main() {
    config::init
    config::load_oauth_token

    args::parse "$@"

    # Auto-detect generic mode: if user doesn't have Claude Code and didn't
    # explicitly pass --generic, warn and switch automatically.
    if [[ "$GENERIC" == false ]] && ! checks::claude_on_host; then
        log::warn "No Claude Code installation detected on host (~/.claude not found)."
        log::warn "Switching to generic image mode (no host settings will be baked in)."
        log::info "To silence this warning, use: vibrate --generic"
        echo ""
        GENERIC=true
    fi

    # In generic mode, override image name and username
    if [[ "$GENERIC" == true ]]; then
        IMAGE_NAME="claude-vb-generic:latest"
        CFG_USERNAME="claude-user"
    fi

    # Derive container name if not explicitly set via --name
    if [[ -z "${CONTAINER_NAME:-}" ]]; then
        config::derive_container_name
    fi

    config::apply_env_overrides

    checks::basic_tools
    checks::workspace_exists
    checks::conflicting_flags

    # --- Special commands that don't need Docker ---

    if [[ -n "$EXPORT_DOCKERFILE" ]]; then
        [[ "$GENERIC" == false ]] && plugins::detect
        dockerfile::export "$EXPORT_DOCKERFILE"
        exit 0
    fi

    # --- From here we need Docker ---

    checks::docker_available
    checks::docker_daemon
    checks::docker_runtime
    checks::disk_space

    # Handle --pull (pull pre-built image, then exit)
    if [[ -n "$FLAG_PULL" ]]; then
        image::pull_registry "$FLAG_PULL"
        exit 0
    fi
    [[ "$GENERIC" == false ]] && plugins::detect

    # Handle --build (build only, then exit)
    if [[ "$FLAG_BUILD_ONLY" == true ]]; then
        image::build
        log::success "Build complete."
        exit 0
    fi

    # Handle image acquisition: --rebuild or auto-build
    if [[ "$FLAG_REBUILD" == true ]]; then
        container::exists && container::remove
        image::exists && docker rmi "$IMAGE_NAME" 2>/dev/null || true
        image::build
    else
        image::ensure
    fi

    # Build the docker run command array
    docker_cmd::build

    # Handle --recreate
    if [[ "$FLAG_RECREATE" == true ]] && container::exists; then
        container::remove
    fi

    # Handle existing container (reuse it)
    if [[ "$REMOVE_AFTER" == false && "$FLAG_RECREATE" == false ]]; then
        if container::exists; then
            container::handle_existing
            # handle_existing calls exec, never returns
        fi
    elif [[ "$REMOVE_AFTER" == true ]]; then
        container::handle_rm_conflict
    fi

    # Show command in verbose mode
    if [[ "$VERBOSE" == true ]]; then
        log::info "Container: ${CONTAINER_NAME}"
        log::info "Workspace: ${WORKSPACE}"
        docker_cmd::print
    fi

    # Run the container
    exec "${DOCKER_RUN_CMD[@]}"
}

main "$@"
