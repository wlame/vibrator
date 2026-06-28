#!/bin/sh
# Vibrator opencode config materializer — runs at container startup (called
# by entrypoint.sh, gated on VIBRATOR_HARNESS=opencode). Reconciles the
# user's host ~/.config/opencode directory with vibrator's baked extension
# artifacts using jq — the same tool the extensions bake with.
#
# Strategy:
#   1. Seed (guarded on the sidecar dir): restore the pristine baked
#      snapshot into ~/.config/opencode, then copy the read-only host
#      sidecar's files over it — host wins per-file.
#   2. Merge: jq deep-merge (baked * host) the two extension-managed JSON
#      files, config.json and tui.json — host wins per-key, baked .mcp
#      entries and theme keys survive unless the host names the same key.
#      Keep jq's -c (compact) output: the verification gate matches
#      compact substrings against the merged files.
#   3. Replay manifest: apply integration-manifest opencode entries (off
#      removes, other modes with an http url add). Gated on the seed so an
#      unseeded baked config is never mutated. No opencode integration
#      wiring ships today -> no-op.
#
# Restart semantics: the merge base is always the snapshot, never a prior
# merge result, so container restarts are idempotent; in-container edits
# under ~/.config/opencode are rebuilt from snapshot x host on each start
# when a host sidecar exists (same behavior as the codex/claude seeds).
#
# Env: XDG_CONFIG_HOME (config root; default ~/.config),
# VIBRATOR_OPENCODE_BAKED (snapshot dir; default ~/.vibrator-opencode-baked),
# VIBRATOR_MANIFEST (integration manifest; default
# /etc/vibrator/integrations.json).
#
# POSIX sh, no `set -e` — a materialization failure must never abort
# container startup (see entrypoint.sh's `log` rationale for the same rule).

command -v jq >/dev/null 2>&1 || exit 0
# Refuse to run with no home anchor at all — CONF_DIR would otherwise
# resolve to /.config/opencode and the seed would rm -rf a root-level path.
[ -n "${XDG_CONFIG_HOME:-}${HOME:-}" ] || exit 0

CONF_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
HOST_DIR="$CONF_DIR.host"
BAKED="${VIBRATOR_OPENCODE_BAKED:-$HOME/.vibrator-opencode-baked}"
MANIFEST="${VIBRATOR_MANIFEST:-/etc/vibrator/integrations.json}"

_vb_log() { [ -n "${VIBRATOR_VERBOSE:-}" ] && printf '[vibrator] %s\n' "$*" >&2; return 0; }

# --- 1. Seed from the host sidecar (armed only once the mount provides it) ---
seeded=false
if [ -d "$HOST_DIR" ]; then
    rm -rf "$CONF_DIR"
    if [ -d "$BAKED" ]; then
        cp -a "$BAKED" "$CONF_DIR" 2>/dev/null
    fi
    mkdir -p "$CONF_DIR"
    cp -a "$HOST_DIR/." "$CONF_DIR/" 2>/dev/null
    seeded=true
    _vb_log "opencode: seeded config dir from host sidecar"
fi

# --- 2. jq deep-merge the extension-managed JSON files -----------------------
# `*` is jq's recursive object merge: host wins per-key, baked keys survive.
# On jq failure (malformed host JSON) the step-1 host copy stays in place —
# host-wins degradation, never a broken half-write (tmp + mv).
_vb_merge_json() {
    f="$1"
    { [ -f "$BAKED/$f" ] && [ -f "$HOST_DIR/$f" ]; } || return 0
    if jq -cs '.[0] * .[1]' "$BAKED/$f" "$HOST_DIR/$f" > "$CONF_DIR/$f.tmp" 2>/dev/null; then
        mv "$CONF_DIR/$f.tmp" "$CONF_DIR/$f"
        _vb_log "opencode: merged $f (baked * host)"
    else
        rm -f "$CONF_DIR/$f.tmp"
    fi
    return 0
}
if [ "$seeded" = true ]; then
    _vb_merge_json config.json
    _vb_merge_json tui.json
fi

# --- 3. Replay integration manifest (opencode/* entries; no-op today) --------
# Gated on the seed: an unseeded baked config is never mutated, so an `off`
# entry can never delete a baked MCP from a fresh-host image (the flaw the
# codex materializer still carries as a deferred fix).
if [ "$seeded" = true ] && [ -f "$MANIFEST" ]; then
    HARNESS_ID="${VIBRATOR_HARNESS:-opencode}"
    CFG="$CONF_DIR/config.json"
    [ -f "$CFG" ] || printf '{}' > "$CFG"
    jq -c --arg h "$HARNESS_ID" '.[]? | select(.harness == $h or .harness == "*")' "$MANIFEST" 2>/dev/null | while IFS= read -r entry; do
        name=$(printf '%s' "$entry" | jq -r '.mcp.name // empty')
        [ -z "$name" ] && continue
        var="VIBRATOR_INTEGRATION_MODE_$(printf '%s' "$name" | tr 'a-z-' 'A-Z_')"
        eval "mode=\${$var:-auto}"
        http_url=$(printf '%s' "$entry" | jq -r '.mcp.http.url // empty')
        case "$mode" in
            off)
                jq --arg n "$name" 'del(.mcp[$n])' "$CFG" > "$CFG.tmp" 2>/dev/null \
                    && mv "$CFG.tmp" "$CFG" || rm -f "$CFG.tmp"
                _vb_log "opencode: manifest removed MCP $name"
                ;;
            *)
                [ -n "$http_url" ] || continue
                jq --arg n "$name" --arg u "$http_url" \
                    '.mcp[$n] = {"type":"remote","url":$u,"enabled":true}' \
                    "$CFG" > "$CFG.tmp" 2>/dev/null \
                    && mv "$CFG.tmp" "$CFG" || rm -f "$CFG.tmp"
                _vb_log "opencode: manifest registered MCP $name"
                ;;
        esac
    done
fi

exit 0
