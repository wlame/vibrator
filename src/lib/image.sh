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
