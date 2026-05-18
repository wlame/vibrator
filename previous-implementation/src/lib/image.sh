# Docker image operations: build, pull, push, check.

image::exists() {
    docker image inspect "$IMAGE_NAME" &>/dev/null
}

image::build() {
    log::info "Building Docker image ${IMAGE_NAME}..."

    local host_uid host_gid
    host_uid=$(id -u)
    host_gid=$(id -g)

    # Running as root (UID 0) breaks user creation in the container.
    # Fall back to standard 1000:1000 — file permissions on mounted volumes
    # will still work because the container user has sudo.
    if [[ "$host_uid" -eq 0 ]]; then
        log::verbose "Running as root; using UID/GID 1000 for container user"
        host_uid=1000
        host_gid=1000
    fi

    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064
    trap "rm -rf '$tmpdir'" RETURN

    dockerfile::generate > "$tmpdir/Dockerfile"

    local -a build_args=(
        docker build
        --progress=tty
        --build-arg "USERNAME=$CFG_USERNAME"
        --build-arg "HOST_UID=$host_uid"
        --build-arg "HOST_GID=$host_gid"
        # Variant info baked into the image so the welcome message and any
        # in-container tooling can introspect what's installed.
        --build-arg "VIBRATOR_PROFILE=$PROFILE"
        --build-arg "VIBRATOR_FEATURES=${FEATURES_ENABLED# }"
        --build-arg "VIBRATOR_VARIANT_FINGERPRINT=${VARIANT_FINGERPRINT:-unknown}"
        # Claude CLI version this build targets. Drives cache invalidation of
        # the Stage 3 install layer and the claude.version image LABEL.
        --build-arg "CLAUDE_CLI_VERSION=${CLAUDE_CLI_VERSION:-latest}"
    )
    [[ "$FLAG_REBUILD" == true ]] && build_args+=(--no-cache)
    build_args+=(-t "$IMAGE_NAME" "$tmpdir")

    log::verbose "Building with UID=${host_uid} GID=${host_gid}"
    log::info "This may take several minutes on first build..."

    if "${build_args[@]}"; then
        log::success "Successfully built ${IMAGE_NAME}"
    else
        log::die "Failed to build Docker image"
    fi
}

image::pull_registry() {
    local tag="$1"
    local remote="${VIBRATOR_REGISTRY}:${tag}"
    local local_name="claude-vb-generic:latest"

    log::info "Pulling ${remote}..."

    if docker pull "$remote"; then
        log::info "Tagging as ${local_name}..."
        docker tag "$remote" "$local_name" || log::die "Failed to tag image"
        log::success "Ready. Run 'vibrate' to start."
    else
        log::die "Failed to pull image. Check https://github.com/wlame/vibrator/pkgs/container/vibrator for available tags."
    fi
}

image::push() {
    local repo="$1"
    log::info "Pushing image to ${repo}..."

    image::exists || {
        log::warn "Local image not found. Getting it first..."
        image::pull
    }

    docker tag "$IMAGE_NAME" "$repo" || log::die "Failed to tag image"

    if docker push "$repo"; then
        log::success "Pushed ${repo}"
    else
        log::die "Failed to push. Make sure you are logged in: docker login"
    fi
}

image::ensure() {
    if ! image::exists; then
        log::info "Image ${IMAGE_NAME} not found. Building..."
        image::build
    fi
}

# List all vibrator-managed images for this user with their LABEL metadata,
# one per line:
#   <image_tag>|<vibrator.profile>|<vibrator.features>|<claude.version>
image::list_managed() {
    docker images \
        --filter "reference=claude-vb-${CFG_USERNAME}-*" \
        --format '{{.Repository}}:{{.Tag}}' 2>/dev/null \
        | while read -r tag; do
            [[ -z "$tag" ]] && continue
            local p f v
            p=$(docker inspect "$tag" --format '{{index .Config.Labels "vibrator.profile"}}' 2>/dev/null)
            f=$(docker inspect "$tag" --format '{{index .Config.Labels "vibrator.features"}}' 2>/dev/null)
            v=$(docker inspect "$tag" --format '{{index .Config.Labels "claude.version"}}' 2>/dev/null)
            printf '%s|%s|%s|%s\n' "$tag" "$p" "$f" "$v"
        done
    return 0
}

# Rebuild one image using its LABEL-recorded profile/features. The fingerprint
# the new build computes should match the tag, so docker reuses cached layers
# everywhere except the Claude install + downstream MCP/plugin layers.
image::_rebuild_from_labels() {
    local tag="$1" profile="$2" features="$3"
    if [[ -z "$profile" || -z "$features" ]]; then
        log::warn "Image $tag has no vibrator.profile/features labels — skipping"
        return 0
    fi

    # Reset the config state to match this image, then re-finalize.
    PROFILE="$profile"
    FEATURES_ENABLED=""
    local f
    for f in $features; do
        config::is_known_feature "$f" || continue
        config::feature_enable "$f"
    done
    IMAGE_NAME="claude-vb-${CFG_USERNAME}:latest"
    VARIANT_FINGERPRINT=""
    config::finalize_image_name

    if [[ "$IMAGE_NAME" != "$tag" ]]; then
        log::warn "Rebuild for $tag would produce $IMAGE_NAME instead (catalog drift?). Skipping."
        return 0
    fi

    log::info "Rebuilding $tag (profile=$profile)"
    image::build
}

# Walk every vibrator image and rebuild those whose baked Claude version
# differs from the current CLAUDE_CLI_VERSION. Cache hits keep this fast
# for image bytes that don't change — usually only Stage 3 + Stage 4 layers
# actually rebuild.
image::upgrade_claude() {
    local current="${CLAUDE_CLI_VERSION:-latest}"
    log::info "Target Claude CLI version: $current"

    # Collect listing into an array so the loop body can mutate globals
    # without being trapped in a subshell pipeline.
    local -a rows=()
    local row
    while IFS= read -r row; do
        [[ -n "$row" ]] && rows+=("$row")
    done < <(image::list_managed)

    if [[ "${#rows[@]}" -eq 0 ]]; then
        log::info "No vibrator-managed images found."
        return 0
    fi

    local total=${#rows[@]} stale=0 ok=0
    for row in "${rows[@]}"; do
        local tag profile features img_version
        IFS='|' read -r tag profile features img_version <<< "$row"
        if [[ "$img_version" == "$current" ]]; then
            log::verbose "Up to date: $tag (claude=$img_version)"
            ok=$((ok + 1))
            continue
        fi
        log::info "Stale: $tag (claude=${img_version:-unknown}, target=$current)"
        stale=$((stale + 1))
        image::_rebuild_from_labels "$tag" "$profile" "$features"
    done

    log::success "Upgrade complete: $stale rebuilt, $ok already current ($total total)"
}
