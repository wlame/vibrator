#!/bin/sh
# Vibrator codex config materializer — runs at container startup (called by
# entrypoint.sh, gated on VIBRATOR_HARNESS=codex). Reconciles the user's host
# codex config with vibrator's baked MCP servers WITHOUT a TOML processor, by
# letting `codex mcp add` own the format.
#
# Strategy:
#   1. Seed: if the read-only host sidecar config.host.toml exists, copy it to
#      the writable container config.toml — the user's model/settings/their
#      own MCP servers become the base.
#   2. Replay baked: re-add each vibrator-baked MCP (snapshotted at build) on
#      top, via `codex mcp add`.
#   3. Replay manifest: apply the integration manifest's codex entries
#      (host/local/off/auto). No codex integration wiring ships today → no-op.
#
# Env: CODEX_HOME (config dir; default ~/.codex), VIBRATOR_CODEX_BAKED_MCP
# (baked snapshot; default ~/.vibrator-codex-baked-mcp.json), VIBRATOR_MANIFEST
# (integration manifest; default /etc/vibrator/integrations.json).
#
# POSIX sh, no `set -e` — a probe/registration failure must never abort
# container startup (see entrypoint.sh's `log` rationale for the same rule).

command -v jq >/dev/null 2>&1 || exit 0
command -v codex >/dev/null 2>&1 || exit 0

CODEX_DIR="${CODEX_HOME:-$HOME/.codex}"
HOST_TOML="$CODEX_DIR/config.host.toml"
CONT_TOML="$CODEX_DIR/config.toml"
BAKED="${VIBRATOR_CODEX_BAKED_MCP:-$HOME/.vibrator-codex-baked-mcp.json}"
MANIFEST="${VIBRATOR_MANIFEST:-/etc/vibrator/integrations.json}"

_vb_log() { [ -n "$VIBRATOR_VERBOSE" ] && printf '[vibrator] %s\n' "$*" >&2; return 0; }

mkdir -p "$CODEX_DIR"

# --- 1. Seed from the host sidecar (armed only once the mount provides it) ---
seeded=false
if [ -f "$HOST_TOML" ]; then
    cp "$HOST_TOML" "$CONT_TOML" 2>/dev/null && seeded=true
    _vb_log "codex: seeded config.toml from host sidecar"
fi

# _vb_add_mcp: read one MCP object on stdin ({name,transport:{...}}) and
# register it with `codex mcp add`. stdio -> command/args/env; http -> --url.
#
# The stdio branch builds the argv by accumulating positional params via
# `set --` across two sequential `while read` loops fed by heredocs (NOT
# pipes — a `cmd | while read` runs the loop in a subshell, so any `set --`
# inside it is lost the moment the loop exits; a heredoc redirection runs
# in THIS shell, so the accumulation sticks). Each loop appends one
# well-quoted argv entry per iteration, so env values / args containing
# spaces still land as a single argument.
_vb_add_mcp() {
    obj=$(cat)
    name=$(printf '%s' "$obj" | jq -r '.name // empty')
    [ -z "$name" ] && return 0
    ttype=$(printf '%s' "$obj" | jq -r '.transport.type // empty')
    case "$ttype" in
        streamable_http|http)
            url=$(printf '%s' "$obj" | jq -r '.transport.url // empty')
            [ -n "$url" ] && codex mcp add "$name" --url "$url" >/dev/null 2>&1 \
                && _vb_log "codex: registered http MCP $name"
            ;;
        stdio)
            cmd=$(printf '%s' "$obj" | jq -r '.transport.command // empty')
            [ -z "$cmd" ] && return 0

            # Phase 1: --env K=V flags, one per env entry (order-insensitive,
            # codex accepts any number of repeated --env flags).
            set --
            while IFS= read -r kv; do
                [ -n "$kv" ] && set -- "$@" --env "$kv"
            done <<EOF
$(printf '%s' "$obj" | jq -r '(.transport.env // {}) | to_entries[]? | "\(.key)=\(.value)"')
EOF
            # Phase 2: `-- <command>` then the server's own argv, appended
            # one element at a time so multi-word args stay intact.
            set -- "$@" -- "$cmd"
            while IFS= read -r a; do
                [ -n "$a" ] && set -- "$@" "$a"
            done <<EOF
$(printf '%s' "$obj" | jq -r '(.transport.args // [])[]?')
EOF
            codex mcp add "$name" "$@" >/dev/null 2>&1 \
                && _vb_log "codex: registered stdio MCP $name"
            ;;
    esac
    return 0
}

# --- 2. Replay baked MCPs (only meaningful after a host seed clobbered them) --
# Name collisions with a user's own [mcp_servers.<name>] resolve to whichever
# `codex mcp add` writes last — the user's host entry survives if `codex mcp
# add` refuses to overwrite it; that host-wins precedence is intentional.
if [ "$seeded" = true ] && [ -f "$BAKED" ]; then
    baked_count=$(jq 'length' "$BAKED" 2>/dev/null || echo 0)
    # A seeded config with an empty baked snapshot means the seed clobbered the
    # baked MCPs and there is nothing to restore — surface it (verbose) so the
    # "empty snapshot ate my MCPs" case is diagnosable without a rebuild.
    [ "$baked_count" = 0 ] && _vb_log "codex: baked MCP snapshot is empty after host seed — no MCPs to restore"
    jq -c '.[]?' "$BAKED" 2>/dev/null | while IFS= read -r obj; do
        printf '%s' "$obj" | _vb_add_mcp
    done
fi

# --- 3. Replay integration manifest (codex/* entries; no-op today) -----------
if [ -f "$MANIFEST" ]; then
    HARNESS_ID="${VIBRATOR_HARNESS:-codex}"
    jq -c --arg h "$HARNESS_ID" '.[]? | select(.harness == $h or .harness == "*")' "$MANIFEST" 2>/dev/null | while IFS= read -r entry; do
        name=$(printf '%s' "$entry" | jq -r '.mcp.name // empty')
        [ -z "$name" ] && continue
        var="VIBRATOR_INTEGRATION_MODE_$(printf '%s' "$name" | tr 'a-z-' 'A-Z_')"
        eval "mode=\${$var:-auto}"
        http_url=$(printf '%s' "$entry" | jq -r '.mcp.http.url // empty')
        case "$mode" in
            off) codex mcp remove "$name" >/dev/null 2>&1 ;;
            *)   [ -n "$http_url" ] && codex mcp add "$name" --url "$http_url" >/dev/null 2>&1 ;;
        esac
    done
fi

exit 0
