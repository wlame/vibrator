# Plugin detection from host ~/.claude/settings.json.
# The actual baking happens in templates/setup-plugins.sh at image build time.

DETECTED_PLUGINS=""

plugins::detect() {
    [[ "$INSTALL_PLUGINS" == true ]] || return 0

    local settings="$CLAUDE_CONFIG/settings.json"
    [[ -f "$settings" ]] || return 0

    DETECTED_PLUGINS=$(jq -r \
        '.enabledPlugins // {} | to_entries[] | select(.value == true) | .key' \
        "$settings" 2>/dev/null | tr '\n' ' ')

    [[ -n "$DETECTED_PLUGINS" ]] && log::verbose "Detected plugins: ${DETECTED_PLUGINS}"
}
