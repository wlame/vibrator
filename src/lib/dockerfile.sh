# Dockerfile generation from embedded template.
# Template and scripts are base64-encoded at build time by the Makefile
# and stored as variables below. At runtime we decode and substitute.

DOCKERFILE_TPL_B64="%%DOCKERFILE_TPL_B64%%"
ENTRYPOINT_B64="%%ENTRYPOINT_B64%%"
CLAUDE_EXEC_B64="%%CLAUDE_EXEC_B64%%"
ZSHRC_B64="%%ZSHRC_B64%%"
SETUP_PLUGINS_B64="%%SETUP_PLUGINS_B64%%"
CONTAINER_RULES_CONTEXT_B64="%%CONTAINER_RULES_CONTEXT_B64%%"
CONTAINER_RULES_SAFETY_B64="%%CONTAINER_RULES_SAFETY_B64%%"

dockerfile::generate() {
    local content
    content=$(echo "$DOCKERFILE_TPL_B64" | base64 -d)

    # Substitute embedded script blobs into Dockerfile
    # These are pure base64 (no special chars), safe for bash substitution.
    content="${content//@@ENTRYPOINT_B64@@/$ENTRYPOINT_B64}"
    content="${content//@@CLAUDE_EXEC_B64@@/$CLAUDE_EXEC_B64}"
    content="${content//@@ZSHRC_B64@@/$ZSHRC_B64}"
    content="${content//@@CONTAINER_RULES_CONTEXT_B64@@/$CONTAINER_RULES_CONTEXT_B64}"
    content="${content//@@CONTAINER_RULES_SAFETY_B64@@/$CONTAINER_RULES_SAFETY_B64}"

    # Plugin section: line-based replacement to avoid bash/awk special char issues
    # (bash ${//} treats & and \ as special in replacements; awk gsub does too)
    if [[ -n "${DETECTED_PLUGINS:-}" ]]; then
        local new_content=""
        local line
        while IFS= read -r line; do
            if [[ "$line" == *"@@PLUGIN_SECTION@@"* ]]; then
                new_content+="# Setup Claude plugins from host configuration"$'\n'
                new_content+="RUN echo '${SETUP_PLUGINS_B64}' | base64 -d > /tmp/setup-plugins.sh && chmod +x /tmp/setup-plugins.sh && \\"$'\n'
                new_content+="    /tmp/setup-plugins.sh '${DETECTED_PLUGINS}' && \\"$'\n'
                new_content+="    rm /tmp/setup-plugins.sh"$'\n'
            else
                new_content+="$line"$'\n'
            fi
        done <<< "$content"
        content="$new_content"
    else
        content="${content//@@PLUGIN_SECTION@@/}"
    fi

    echo "$content"
}

dockerfile::export() {
    local output_file="$1"
    log::info "Exporting Dockerfile to: ${output_file}"
    dockerfile::generate > "$output_file"
    log::success "Dockerfile exported."
    log::info "Build with:"
    log::info "  docker build --build-arg USERNAME=\$(whoami) --build-arg HOST_UID=\$(id -u) --build-arg HOST_GID=\$(id -g) -t your-image ."
}
