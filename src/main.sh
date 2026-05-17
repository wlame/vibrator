# Main execution flow. Wires all modules together.

main() {
    config::init
    config::load_oauth_token
    # Apply project pin (./.vb.env) BEFORE CLI parsing so CLI flags can
    # override the pinned profile/features for one-off runs.
    config::load_pin_file

    args::parse "$@"

    # Handle --claude-mem-setup (print onboarding and exit, no docker needed)
    if [[ "$FLAG_CLAUDE_MEM_SETUP" == true ]]; then
        config::print_claude_mem_setup
        exit 0
    fi

    # Handle --claude-mem-status (probe host stack, dump resolved wiring)
    if [[ "$FLAG_CLAUDE_MEM_STATUS" == true ]]; then
        claude_mem::print_status
        exit $?
    fi

    # Handle --claude-mem-bootstrap (mint a project-scoped key host-side
    # without launching a container). Useful for CI seeding or for manually
    # re-bootstrapping after revoking the cached key.
    if [[ "$FLAG_CLAUDE_MEM_BOOTSTRAP" == true ]]; then
        local cfg="${VIBRATOR_CLAUDE_MEM_ENV:-$HOME/.config/vibrator/claude-mem.env}"
        if [[ ! -f "$cfg" ]]; then
            log::die "claude-mem: admin config missing ($cfg). Run: vibrate --claude-mem-setup"
        fi
        local url database_url
        url=$(claude_mem::_read_dotenv "$cfg" CLAUDE_MEM_SERVER_BETA_URL)
        database_url=$(claude_mem::_read_dotenv "$cfg" CLAUDE_MEM_SERVER_DATABASE_URL)
        [[ -n "$url" ]]          || log::die "claude-mem: CLAUDE_MEM_SERVER_BETA_URL missing in $cfg"
        [[ -n "$database_url" ]] || log::die "claude-mem: CLAUDE_MEM_SERVER_DATABASE_URL missing in $cfg (required for bootstrap)"
        claude_mem_bootstrap::run "$url" "$database_url"
        exit $?
    fi

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

    # Interactive workspace picker for fresh folders without a .vb.env.
    # Skips silently when conditions don't fit (no TTY, user flags given,
    # existing container, etc.) — see menu::should_show.
    if menu::should_show; then
        menu::main
    fi

    # Compute variant fingerprint and (unless --image or generic mode set
    # IMAGE_NAME explicitly) rewrite IMAGE_NAME to include profile + hash.
    # Replaces the previous --simple-only :simple tag hack.
    config::finalize_image_name

    # Derive container name if not explicitly set via --name. Includes the
    # variant fingerprint so different profiles for the same workspace map
    # to distinct containers.
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

    # Handle --upgrade-claude (rebuild every stale image and exit)
    if [[ "$FLAG_UPGRADE_CLAUDE" == true ]]; then
        image::upgrade_claude
        exit 0
    fi

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

    # Last-mile reminder: claude-mem feature is on but the user hasn't
    # written the host-side env file yet. One line, with the hint to run
    # `vibrate --claude-mem-setup` for the full instructions.
    if config::feature_enabled "claude-mem" && ! config::claude_mem_configured; then
        log::warn "claude-mem is enabled but not configured on this host."
        log::warn "Hooks will fire but the memory pipeline is empty. Run:"
        log::warn "  vibrate --claude-mem-setup"
        echo ""
    fi

    # Run the container
    exec "${DOCKER_RUN_CMD[@]}"
}

main "$@"
