#!/bin/sh
# Vibrator pi config materializer — runs at container startup (called by
# entrypoint.sh, gated on VIBRATOR_HARNESS=pi). Reconciles the user's host
# ~/.pi tree with vibrator's baked extension artifacts using jq.
#
# Strategy:
#   1. Seed (guarded on the sidecar dir, NON-destructive): copy the baked
#      snapshot's files over ~/.pi, then the read-only host sidecar's
#      files over that (host wins per-file). NO rm -rf: agent/auth.json
#      and agent/sessions are live rw bind mounts INSIDE ~/.pi — a
#      destructive reset would fail on the mount roots and could delete
#      host session data through the bind. Both copies skip those mounted
#      carve-outs AND agent/bin (pi's managed fd/rg binaries are
#      arch/OS-specific and pi self-updates them; the materializer never
#      touches bin — the one deliberate host-wins exception).
#   2. Merge: jq deep-merge agent/mcp.json (host wins per-key, baked
#      .mcpServers survive) and agent/settings.json (host wins for scalar
#      keys; .packages is the UNION of baked+host — a plain `*` merge
#      would let the host array clobber the baked extension registry).
#      Keep jq's -c (compact) output: the verification gate matches
#      compact substrings against the merged files.
#   3. Replay manifest: apply integration-manifest pi entries to
#      .mcpServers (off removes; other modes with an http url add). Gated
#      on the seed so an unseeded baked config is never mutated. No pi
#      integration wiring ships today -> no-op.
#
# Restart semantics: merges always recompute from snapshot x host (never a
# prior merge result), so restarts are deterministic. Because the seed is
# non-destructive, container-created files in ~/.pi outside the rw mounts
# may linger across restarts; containers are recreated on config change,
# so this is acceptable.
#
# Env: VIBRATOR_PI_BAKED (snapshot dir; default ~/.vibrator-pi-baked),
# VIBRATOR_MANIFEST (integration manifest; default
# /etc/vibrator/integrations.json). Test isolation is via HOME (pi derives
# ~/.pi from homedir(); PI_CODING_AGENT_DIR is only partially honored by
# pi and must not be relied on).
#
# POSIX sh, no `set -e` — a materialization failure must never abort
# container startup (see entrypoint.sh's `log` rationale for the same rule).

command -v jq >/dev/null 2>&1 || exit 0
[ -n "${HOME:-}" ] || exit 0

PI_DIR="$HOME/.pi"
HOST_DIR="$HOME/.pi.host"
BAKED="${VIBRATOR_PI_BAKED:-$HOME/.vibrator-pi-baked}"
MANIFEST="${VIBRATOR_MANIFEST:-/etc/vibrator/integrations.json}"

_vb_log() { [ -n "${VIBRATOR_VERBOSE:-}" ] && printf '[vibrator] %s\n' "$*" >&2; return 0; }

# _vb_copy_tree SRC — copy regular files and symlinks from SRC over
# $PI_DIR, creating parent dirs as needed. Skips the live rw mount
# carve-outs (agent/auth.json, agent/sessions/*) so the materializer never
# writes through a bind mount to the host, and agent/bin/* from EITHER
# source: pi manages those arch-specific binaries itself (fd, rg) — a host
# copy could be the wrong OS/arch, and a snapshot restore would revert
# binaries pi self-updated at runtime. The image's baked bin is already in
# ~/.pi, so never materializing bin loses nothing.
_vb_copy_tree() {
    src="$1"
    [ -d "$src" ] || return 0
    (cd "$src" && find . \( -type f -o -type l \) 2>/dev/null) | while IFS= read -r rel; do
        rel="${rel#./}"
        case "$rel" in
            agent/auth.json|agent/sessions/*|agent/bin/*) continue ;;
        esac
        mkdir -p "$PI_DIR/$(dirname "$rel")" 2>/dev/null
        cp -a "$src/$rel" "$PI_DIR/$rel" 2>/dev/null
    done
    return 0
}

# --- 1. Seed from the host sidecar (armed only once the mount provides it) ---
seeded=false
if [ -d "$HOST_DIR" ]; then
    mkdir -p "$PI_DIR"
    _vb_copy_tree "$BAKED"
    _vb_copy_tree "$HOST_DIR"
    seeded=true
    _vb_log "pi: seeded .pi tree from baked snapshot + host sidecar"
fi

# --- 2. jq merges for the two extension-managed JSON files -------------------
# Only when BOTH baked and host copies exist (otherwise step 1's host-wins
# copy already left the right file in place). tmp + mv, never a half-write;
# on jq failure (malformed host JSON) the step-1 host copy stays in place —
# host-wins degradation.
_vb_merge() {
    f="agent/$1"; expr="$2"
    { [ -f "$BAKED/$f" ] && [ -f "$HOST_DIR/$f" ]; } || return 0
    if jq -cs "$expr" "$BAKED/$f" "$HOST_DIR/$f" > "$PI_DIR/$f.tmp" 2>/dev/null; then
        mv "$PI_DIR/$f.tmp" "$PI_DIR/$f"
        _vb_log "pi: merged $f (baked x host)"
    else
        rm -f "$PI_DIR/$f.tmp"
    fi
    return 0
}
if [ "$seeded" = true ]; then
    _vb_merge mcp.json '.[0] * .[1]'
    # Named captures ($b/$h) keep the original baked/host docs addressable
    # after the pipe into the merged object below — a bare `.[0].packages`
    # after `(.[0] * .[1]) |` would index the *merged object* (not the
    # original two-element slurp array) and jq errors "Cannot index object
    # with number", silently degrading to the host-wins copy from step 1.
    _vb_merge settings.json '.[0] as $b | .[1] as $h | ($b * $h) | .packages = ((($b.packages // []) + ($h.packages // [])) | unique)'
fi

# --- 3. Replay integration manifest (pi/* entries; no-op today) --------------
# Gated on the seed: an unseeded baked config is never mutated, so an `off`
# entry can never delete a baked MCP from a fresh-host image.
if [ "$seeded" = true ] && [ -f "$MANIFEST" ]; then
    HARNESS_ID="${VIBRATOR_HARNESS:-pi}"
    CFG="$PI_DIR/agent/mcp.json"
    [ -f "$CFG" ] || { mkdir -p "$PI_DIR/agent"; printf '{}' > "$CFG"; }
    jq -c --arg h "$HARNESS_ID" '.[]? | select(.harness == $h or .harness == "*")' "$MANIFEST" 2>/dev/null | while IFS= read -r entry; do
        name=$(printf '%s' "$entry" | jq -r '.mcp.name // empty')
        [ -z "$name" ] && continue
        var="VIBRATOR_INTEGRATION_MODE_$(printf '%s' "$name" | tr 'a-z-' 'A-Z_')"
        eval "mode=\${$var:-auto}"
        http_url=$(printf '%s' "$entry" | jq -r '.mcp.http.url // empty')
        case "$mode" in
            off)
                jq -c --arg n "$name" 'del(.mcpServers[$n])' "$CFG" > "$CFG.tmp" 2>/dev/null \
                    && mv "$CFG.tmp" "$CFG" || rm -f "$CFG.tmp"
                _vb_log "pi: manifest removed MCP $name"
                ;;
            *)
                [ -n "$http_url" ] || continue
                jq -c --arg n "$name" --arg u "$http_url" '.mcpServers[$n] = {"url":$u}' "$CFG" > "$CFG.tmp" 2>/dev/null \
                    && mv "$CFG.tmp" "$CFG" || rm -f "$CFG.tmp"
                _vb_log "pi: manifest registered MCP $name"
                ;;
        esac
    done
fi

exit 0
