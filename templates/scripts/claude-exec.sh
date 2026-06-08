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

# Always-visible warning (unlike _vb_log it ignores VIBRATOR_VERBOSE).
# Used when the user's explicit intent can't be honored — e.g. they asked
# for the host server but it's unreachable — so the situation isn't silent.
_vb_warn() {
    printf '[vibrator] warning: %s\n' "$*" >&2
    return 0
}

# Resolve the hosting mode for an integration by name. Reads
# VIBRATOR_INTEGRATION_MODE_<UPPERNAME> (set per-pin at container-run time;
# see internal/app/launch.go). Hyphens become underscores to match the Go
# side. Defaults to "auto" when unset/unknown.
_vb_integration_mode() {
    var="VIBRATOR_INTEGRATION_MODE_$(printf '%s' "$1" | tr 'a-z-' 'A-Z_')"
    eval "mode=\${$var:-auto}"
    case "$mode" in
        host|local|off|auto) printf '%s' "$mode" ;;
        *) printf 'auto' ;;
    esac
}

# Remove an MCP entry from ~/.claude.json (used for mode=off).
_vb_remove_mcp() {
    name="$1"
    jq --arg n "$name" 'if .mcpServers then .mcpServers |= del(.[$n]) else . end' \
        "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
        && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
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

# Auto-accept the workspace trust dialog.
# Claude Code checks projects[<path>].hasTrustDialogAccepted in
# ~/.claude.json before showing the "do you trust this folder?" prompt.
# The workspace is the whole point of this container — trust is implicit.
# The entrypoint guarantees ~/.claude.json exists before we run.
if [ -n "$WORKSPACE_PATH" ] && [ -f "$CLAUDE_JSON" ] && command -v jq >/dev/null 2>&1; then
    jq --arg ws "$WORKSPACE_PATH" \
        '.projects[$ws].hasTrustDialogAccepted = true' \
        "$CLAUDE_JSON" > "$CLAUDE_JSON.tmp" 2>/dev/null \
        && mv -f "$CLAUDE_JSON.tmp" "$CLAUDE_JSON" 2>/dev/null
    _vb_log "workspace trust accepted for $WORKSPACE_PATH"
fi

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

        # MCP wiring. The user's per-integration hosting mode decides
        # whether we prefer the host server (http) or a container-local
        # instance (stdio):
        #   host  — require http; warn loudly if unreachable (no fallback)
        #   local — stdio only; never probe the host
        #   off   — remove the entry entirely
        #   auto  — probe http, fall back to stdio with a visible warning
        if [ -n "$name" ]; then
            http_url=$(echo "$entry" | jq -r '.mcp.http.url // empty')
            stdio=$(echo "$entry" | jq -c '.mcp.stdio // empty')
            mode=$(_vb_integration_mode "$name")
            has_stdio=false
            [ -n "$stdio" ] && [ "$stdio" != "null" ] && has_stdio=true

            case "$mode" in
                off)
                    _vb_remove_mcp "$name"
                    _vb_log "$name: disabled (mode=off)"
                    ;;
                local)
                    if [ "$has_stdio" = true ]; then
                        echo "$stdio" | _vb_write_mcp_stdio "$name"
                    else
                        _vb_warn "$name: mode=local but no stdio command declared — leaving as-is"
                    fi
                    ;;
                host)
                    if [ -n "$http_url" ] && _vb_probe_http "$http_url"; then
                        _vb_write_mcp_http "$name" "$http_url"
                    elif [ -n "$http_url" ]; then
                        # Honor intent: keep the http entry so the failure is
                        # visible in the harness rather than silently masked
                        # by a local fallback the user explicitly opted out of.
                        _vb_write_mcp_http "$name" "$http_url"
                        _vb_warn "$name: host server unreachable at $http_url (mode=host — not falling back to local)"
                    else
                        _vb_warn "$name: mode=host but no http url declared"
                    fi
                    ;;
                *) # auto
                    if [ -n "$http_url" ] && _vb_probe_http "$http_url"; then
                        _vb_write_mcp_http "$name" "$http_url"
                    elif [ "$has_stdio" = true ]; then
                        echo "$stdio" | _vb_write_mcp_stdio "$name"
                        [ -n "$http_url" ] && _vb_warn "$name: host server unreachable at $http_url — using container-local instance"
                    fi
                    ;;
            esac
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
