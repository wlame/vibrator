#!/bin/sh
# Vibrator exec wrapper — invoked instead of bare /bin/<shell> on every
# `docker run` CMD and every `docker exec` re-entry. Its job is to do
# any wiring that needs to refresh on each session start (not just once
# at container creation), then exec the user's command.
#
# Currently:
#   - Live Serena MCP transport switching (C6): the host may toggle
#     `uvx serena start-mcp-server --transport http` on/off between
#     sessions. We re-probe on every entry so a server start (or stop)
#     gets picked up without a container rebuild.
#
# POSIX sh — runs as the unprivileged user, inherits the container's
# env (incl. PATH, HOME). No `set -e` because a probe failure should
# NEVER block the user's shell from starting; we'd rather silently
# fall back to stdio than fail-closed.

# Verbose-only log helper — see entrypoint.sh for the `return 0`
# rationale (set -e isn't on here, but match the convention).
_vb_log() {
    [ -n "$VIBRATOR_VERBOSE" ] && printf '[vibrator] %s\n' "$*" >&2
    return 0
}

SERENA_PORT="${SERENA_PORT:-8765}"
SERENA_URL="http://host.docker.internal:${SERENA_PORT}/mcp"
CLAUDE_JSON="$HOME/.claude.json"

# Only touch the config if claude-mem's settings file exists AND jq
# is available — fresh containers may not have either yet.
if [ -f "$CLAUDE_JSON" ] && command -v jq >/dev/null 2>&1 && command -v curl >/dev/null 2>&1; then
    # Probe: server present if either a GET returns a recognizable MCP
    # response body OR a HEAD returns the mcp-session-id header.
    # Short timeouts (300ms connect, 500ms total) — we're in the
    # session-start hot path and a slow probe is more annoying than
    # missing a server.
    if curl -sf --connect-timeout 0.3 --max-time 0.5 "$SERENA_URL" 2>/dev/null \
            | grep -q "mcp-session-id\|jsonrpc" \
       || curl -sf --connect-timeout 0.3 --max-time 0.5 -I "$SERENA_URL" 2>/dev/null \
            | grep -q "mcp-session-id"; then
        # Server up — switch (or keep) MCP transport at http.
        CURRENT_TYPE=$(jq -r '.mcpServers.serena.type // "unknown"' "$CLAUDE_JSON" 2>/dev/null)
        if [ "$CURRENT_TYPE" != "http" ]; then
            jq --arg url "$SERENA_URL" \
                '.mcpServers.serena = {type: "http", url: $url}' \
                "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
                && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
            _vb_log "Serena: switched to http transport ($SERENA_URL)"
        else
            _vb_log "Serena: using http transport ($SERENA_URL)"
        fi
    else
        # Server down — switch (or keep) MCP transport at stdio.
        # Spawn local `uvx serena start-mcp-server --project-from-cwd`
        # as a child process when claude requests the serena MCP.
        CURRENT_TYPE=$(jq -r '.mcpServers.serena.type // "unknown"' "$CLAUDE_JSON" 2>/dev/null)
        if [ "$CURRENT_TYPE" = "http" ]; then
            jq '.mcpServers.serena = {
                type: "stdio",
                command: "uvx",
                args: ["--from", "git+https://github.com/oraios/serena", "serena", "start-mcp-server", "--project-from-cwd"]
            }' "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
                && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
            _vb_log "Serena: fell back to stdio (host server unreachable)"
        fi
    fi
fi

# cd to the workspace if it's set. This duplicates the entrypoint's
# step 1, but the wrapper runs on every `docker exec` too — and exec
# doesn't run the entrypoint. Without this, re-entries would land in
# the user's HOME on host-restart cycles.
if [ -n "$WORKSPACE_PATH" ] && [ -d "$WORKSPACE_PATH" ]; then
    cd "$WORKSPACE_PATH" || _vb_log "WORKSPACE_PATH=$WORKSPACE_PATH cd failed"
fi

# Exec the user's command. With no args, default to /bin/sh so the
# wrapper is safe to use as a CMD even when no explicit command was
# wired up. In practice the call sites always pass a shell ($SHELL or
# /bin/zsh etc.).
if [ $# -gt 0 ]; then
    exec "$@"
else
    exec /bin/sh
fi
