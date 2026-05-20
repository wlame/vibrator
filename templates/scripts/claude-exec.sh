#!/bin/sh
# Vibrator exec wrapper — invoked instead of bare /bin/<shell> on every
# `docker run` CMD and every `docker exec` re-entry. Its job is to do
# any wiring that needs to refresh on each session start (not just once
# at container creation), then exec the user's command.
#
# Data-driven design:
#   /etc/vibrator/integrations.json is a JSON array of entries
#   generated at build time from the host-side integration registry
#   (see internal/integration/manifest.go). For each entry the script:
#
#     - If MCP has an http URL: probe it. On success, write http config
#       to ~/.claude.json. On failure, fall back to the stdio entry if
#       one is declared.
#     - If MCP has only stdio: write the stdio config unconditionally.
#     - If env vars are declared: export them into this shell so the
#       harness inherits them.
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

MANIFEST=/etc/vibrator/integrations.json
CLAUDE_JSON="$HOME/.claude.json"

# Probe one URL. Returns 0 if the server responded with MCP-style
# headers or body, non-zero otherwise. Short timeouts because this
# runs in the session-start hot path — a slow probe is more annoying
# than missing a server.
_vb_probe_http() {
    url="$1"
    [ -z "$url" ] && return 1
    curl -sf --connect-timeout 0.3 --max-time 0.5 "$url" 2>/dev/null \
            | grep -q "mcp-session-id\|jsonrpc" \
        || curl -sf --connect-timeout 0.3 --max-time 0.5 -I "$url" 2>/dev/null \
            | grep -q "mcp-session-id"
}

# Write an http MCP entry to ~/.claude.json. Idempotent — checks the
# existing entry and skips the jq write if already in the right shape.
# Args: $1=name $2=url
_vb_write_mcp_http() {
    name="$1"; url="$2"
    current=$(jq -r --arg n "$name" '.mcpServers[$n].type // "unknown"' "$CLAUDE_JSON" 2>/dev/null)
    if [ "$current" != "http" ]; then
        jq --arg n "$name" --arg url "$url" \
            '.mcpServers[$n] = {type: "http", url: $url}' \
            "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
            && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
        _vb_log "$name: switched to http transport ($url)"
    else
        _vb_log "$name: using http transport ($url)"
    fi
}

# Write a stdio MCP entry to ~/.claude.json. Takes the entire stdio
# subobject as JSON on stdin so command/args/env round-trip cleanly.
# Args: $1=name; stdin=stdio JSON ({command,args?,env?})
_vb_write_mcp_stdio() {
    name="$1"
    stdio_json=$(cat)
    current=$(jq -r --arg n "$name" '.mcpServers[$n].type // "unknown"' "$CLAUDE_JSON" 2>/dev/null)
    # Build the target entry: {type:"stdio", command, args, env?}
    target=$(echo "$stdio_json" | jq -c '{
        type: "stdio",
        command: .command[0],
        args: ((.command[1:]) + (.args // [])),
        env: (.env // {})
    }')
    # Always write; comparing the entry is more work than just letting
    # jq idempotently replace it. We only log on type transitions to
    # keep verbose output sane.
    jq --arg n "$name" --argjson e "$target" \
        '.mcpServers[$n] = $e' \
        "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
        && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
    if [ "$current" != "stdio" ]; then
        _vb_log "$name: fell back to stdio (host server unreachable)"
    fi
}

# Process the manifest if everything is in place. Skip silently
# otherwise — fresh containers may not have ~/.claude.json yet, and
# the harness will create it on first run.
if [ -f "$MANIFEST" ] && [ -f "$CLAUDE_JSON" ] \
        && command -v jq >/dev/null 2>&1 \
        && command -v curl >/dev/null 2>&1; then
    # Read VIBRATOR_HARNESS from the image (set in the runtime stage).
    # Falls back to "*" if missing so an older image with the new wrapper
    # still does something sensible.
    HARNESS="${VIBRATOR_HARNESS:-*}"

    # Iterate the manifest. Each entry is a single line of compact JSON
    # so `read -r` works cleanly. We filter to this harness or "*".
    jq -c --arg h "$HARNESS" \
        '.[] | select(.harness == $h or .harness == "*")' \
        "$MANIFEST" 2>/dev/null | while IFS= read -r entry; do
        name=$(echo "$entry" | jq -r '.mcp.name // empty')

        # MCP wiring: try http, fall back to stdio.
        if [ -n "$name" ]; then
            http_url=$(echo "$entry" | jq -r '.mcp.http.url // empty')
            stdio=$(echo "$entry" | jq -c '.mcp.stdio // empty')

            if [ -n "$http_url" ] && _vb_probe_http "$http_url"; then
                _vb_write_mcp_http "$name" "$http_url"
            elif [ -n "$stdio" ] && [ "$stdio" != "null" ]; then
                echo "$stdio" | _vb_write_mcp_stdio "$name"
            fi
        fi

        # EnvVars wiring: export each key=value into the current shell.
        # Persists into the exec'd shell because env is inherited.
        envs=$(echo "$entry" | jq -r '.env // {} | to_entries[] | "\(.key)=\(.value)"')
        if [ -n "$envs" ]; then
            for kv in $envs; do
                export "$kv"
            done
        fi
    done
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
