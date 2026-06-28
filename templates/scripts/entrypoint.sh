#!/bin/sh
# Vibrator entrypoint — runs once on `docker run`, before exec'ing the
# user's command (usually their shell). Performs the runtime wiring that
# makes the container feel like an extension of the host:
#
#   1. cd to the workspace
#   2. Merge host Claude config (~/.claude.json) — OAuth + onboarding state
#   3. Copy host Claude rules into the container's rules dir
#   4. Generate a "user-identity" rule so Claude uses the real username
#      in generated code (not "user" or "your-name" placeholders)
#   5. Merge host Claude settings.json with macOS-path rewrite, preserving
#      any plugin hooks baked into the image (claude-mem, etc.)
#
# POSIX sh — no bash-isms (this script is executed via Docker ENTRYPOINT
# which uses /bin/sh, typically dash on debian-family bases). Tools
# assumed installed in the base image: jq, sed, cp, mkdir, mv.
#
# All steps tolerate missing inputs (host files not mounted, jq missing,
# malformed JSON) — entrypoint never blocks container startup.
# Set VIBRATOR_VERBOSE=1 to see what was applied.

set -e

log() {
    # The explicit `return 0` is REQUIRED. Without it, this function
    # returns the exit status of `[ -n "$VIBRATOR_VERBOSE" ] && printf`
    # — which is non-zero whenever VIBRATOR_VERBOSE is unset (the
    # common case). Combined with `set -e` at the top of the script,
    # that means the FIRST `log` call silently kills the entire
    # entrypoint with exit 1 and no output. Classic shell gotcha;
    # symptom is "container exits immediately with no logs".
    [ -n "$VIBRATOR_VERBOSE" ] && printf '[vibrator] %s\n' "$*" >&2
    return 0
}
log_err() {
    printf '[vibrator] %s\n' "$*" >&2
    return 0
}

# --- 0. workspace parent dir creation (C11) --------------------------------
# Docker creates intermediate parents automatically when materializing
# bind mounts, but only as root with mode 0755. That works for traversal
# but not for tools that want to *write* sibling paths (e.g. `git clone`
# into the workspace's parent dir, or `cd ..` then mkdir). If the parent
# is missing entirely (the workspace mount is the only thing under
# /Users/wlame/), pre-create it so those flows don't blow up.
if [ -n "$WORKSPACE_PATH" ]; then
    WORKSPACE_PARENT=$(dirname "$WORKSPACE_PATH")
    if [ ! -d "$WORKSPACE_PARENT" ]; then
        # sudo because the parent path is typically /Users/<user>/...
        # which the unprivileged container user can't mkdir directly.
        # `|| true` because failure here is non-fatal — the container
        # still works for in-workspace operations.
        sudo mkdir -p "$WORKSPACE_PARENT" 2>/dev/null || true
        log "workspace: created parent dir $WORKSPACE_PARENT"
    fi
fi

# --- 1. cd to workspace ----------------------------------------------------
# WORKSPACE_PATH is forwarded by `docker run -e`; we also set --workdir,
# so this is belt-and-suspenders for sub-invocations that reset PWD.
if [ -n "$WORKSPACE_PATH" ] && [ -d "$WORKSPACE_PATH" ]; then
    cd "$WORKSPACE_PATH" || log_err "WORKSPACE_PATH=$WORKSPACE_PATH cd failed"
fi

# --- 2. Host Claude config merge (~/.claude.json) --------------------------
# Read the OAuth/onboarding subset from the read-only host mount and
# merge it into the container's writable copy. Without this, claude
# inside the container thinks the user hasn't authenticated/onboarded
# and re-prompts for everything on first launch.
HOST_CLAUDE_JSON="$HOME/.claude.host.json"
CONTAINER_CLAUDE_JSON="$HOME/.claude.json"

if [ -f "$HOST_CLAUDE_JSON" ] && command -v jq >/dev/null 2>&1; then
    # The keys that carry auth + onboarding state — extracted with jq
    # rather than blanket-copied so we don't drag in host-specific paths
    # or settings the container shouldn't inherit. Null-valued keys are
    # filtered out so they don't overwrite container-set values.
    HOST_SUBSET=$(jq -c '{
        oauthAccount,
        hasSeenTasksHint,
        userID,
        hasCompletedOnboarding,
        lastOnboardingVersion,
        subscriptionNoticeCount,
        hasAvailableSubscription,
        s1mAccessCache,
        bypassPermissionsModeAccepted: true
    } | with_entries(select(.value != null))' "$HOST_CLAUDE_JSON" 2>/dev/null || echo "")

    if [ -n "$HOST_SUBSET" ] && [ "$HOST_SUBSET" != "null" ] && [ "$HOST_SUBSET" != "{}" ]; then
        if [ -f "$CONTAINER_CLAUDE_JSON" ]; then
            jq ". * $HOST_SUBSET" "$CONTAINER_CLAUDE_JSON" \
                > "$CONTAINER_CLAUDE_JSON.tmp" \
                && mv "$CONTAINER_CLAUDE_JSON.tmp" "$CONTAINER_CLAUDE_JSON"
        else
            echo "$HOST_SUBSET" | jq . > "$CONTAINER_CLAUDE_JSON"
        fi
        log "Claude config: merged auth/onboarding state from host"
    fi
fi

# --- 2b. Identity override ([identity] in .vb) -----------------------------
# When the user sets an alias, force it everywhere the agent might read a
# "contact" email so the real Anthropic-account email never reaches git
# commits or outbound HTTP headers:
#   - rewrite oauthAccount.emailAddress/displayName in ~/.claude.json (the
#     field the model reads and reuses), and
#   - pin git's global identity (commits already use the GIT_*_EMAIL env
#     vars vibrator forwards; this makes `git config user.email` agree).
if [ -n "$VIBRATOR_IDENTITY_EMAIL" ]; then
    if [ -f "$CONTAINER_CLAUDE_JSON" ] && command -v jq >/dev/null 2>&1; then
        jq --arg email "$VIBRATOR_IDENTITY_EMAIL" --arg name "$VIBRATOR_IDENTITY_NAME" '
            if .oauthAccount then
                .oauthAccount.emailAddress = $email
                | (if $name != "" then .oauthAccount.displayName = $name else . end)
            else . end
        ' "$CONTAINER_CLAUDE_JSON" > "$CONTAINER_CLAUDE_JSON.tmp" \
            && mv "$CONTAINER_CLAUDE_JSON.tmp" "$CONTAINER_CLAUDE_JSON"
        log "Identity: rewrote Claude account email/display name to the configured alias"
    fi
    if command -v git >/dev/null 2>&1; then
        git config --global user.email "$VIBRATOR_IDENTITY_EMAIL"
        [ -n "$VIBRATOR_IDENTITY_NAME" ] && git config --global user.name "$VIBRATOR_IDENTITY_NAME"
        log "Identity: pinned git global user.email/user.name to the configured alias"
    fi
fi

# --- 3. Host rules → container rules ---------------------------------------
# Host's ~/.claude/rules/ is mounted read-only at ~/.claude/rules-host;
# copy each *.md into the writable container rules dir. Done on every
# entrypoint run, so editing rules on the host takes effect on the next
# `vibrate` (no rebuild needed).
mkdir -p "$HOME/.claude/rules"

if [ -d "$HOME/.claude/rules-host" ]; then
    # 'cp *.md' fails noisily when the glob matches nothing; the
    # `2>/dev/null || true` guards both the missing-files case and any
    # individual copy failure (we don't want a permission glitch on one
    # rule to abort container startup).
    cp "$HOME/.claude/rules-host/"*.md "$HOME/.claude/rules/" 2>/dev/null || true
    RULES_COUNT=$(ls -1 "$HOME/.claude/rules-host/"*.md 2>/dev/null | wc -l | tr -d ' ')
    log "Claude rules: copied $RULES_COUNT from host"
fi

# --- 4. Auto user-identity rule --------------------------------------------
# Tell Claude to use the real host username in generated code instead
# of "user"/"your-name" placeholders. Regenerated on every entry so a
# manual edit (or upstream rule changes) get a fresh copy.
USERNAME=$(whoami 2>/dev/null || echo "")
if [ -n "$USERNAME" ]; then
    cat > "$HOME/.claude/rules/user-identity.md" <<RULE
# User Identity

The user's system username is: $USERNAME

Use "$USERNAME" whenever a username, login, author name, or owner is
needed in generated code — for example in package.json "author", Go
module paths, GitHub references, shebang comments, copyright headers,
git config examples, or placeholder values. Never use generic
placeholders like "user", "username", "your-name", or "your-username"
when this real username is available.
RULE
    log "Claude rules: generated user-identity rule for $USERNAME"
fi

# --- 5. Settings.json merge with macOS-path rewrite ------------------------
# This is the trickiest step. The image's settings.json may carry baked
# plugin hooks (e.g. claude-mem registers six hook entries during
# install). A wholesale copy from the host would wipe those.
#
# Sequence:
#   a. Snapshot baked hooks into a variable BEFORE the copy
#   b. Copy host settings.json verbatim
#   c. Rewrite macOS-style absolute hook paths (/Users/.../.claude/) to
#      the container's $HOME/.claude/ — otherwise hooks point at host
#      paths that don't exist inside the container
#   d. Merge baked hooks back in per-event (so both host hooks AND
#      plugin hooks fire on the same event)
HOST_SETTINGS="$HOME/.claude/settings.host.json"
CONTAINER_SETTINGS="$HOME/.claude/settings.json"
mkdir -p "$HOME/.claude"

if [ -f "$HOST_SETTINGS" ] && command -v jq >/dev/null 2>&1; then
    BAKED_HOOKS="{}"
    if [ -f "$CONTAINER_SETTINGS" ]; then
        BAKED_HOOKS=$(jq -c '.hooks // {}' "$CONTAINER_SETTINGS" 2>/dev/null || echo "{}")
    fi

    cp "$HOST_SETTINGS" "$CONTAINER_SETTINGS"
    # `sed -i` here uses GNU semantics (no backup-suffix arg) — fine
    # because Ubuntu's coreutils ship GNU sed. If you test this script
    # on macOS host, BSD sed needs `sed -i ''` instead and will fail
    # silently here (the `|| true` swallows it). End-to-end test in
    # the actual container, not on macOS.
    sed -i "s|/Users/[^/]*/.claude/|$HOME/.claude/|g" "$CONTAINER_SETTINGS" 2>/dev/null || true

    if [ "$BAKED_HOOKS" != "{}" ] && [ "$BAKED_HOOKS" != "null" ]; then
        # Concatenate per-event arrays: result[event] = baked[event] + host[event]
        # Both arrays preserved so both sets of handlers fire.
        jq --argjson baked "$BAKED_HOOKS" '
            .hooks = (
                (.hooks // {}) as $host |
                (($baked + $host) | keys) as $all_keys |
                reduce $all_keys[] as $k ({};
                    .[$k] = (($baked[$k] // []) + ($host[$k] // []))
                )
            )
        ' "$CONTAINER_SETTINGS" > "$CONTAINER_SETTINGS.tmp" \
            && mv "$CONTAINER_SETTINGS.tmp" "$CONTAINER_SETTINGS"
        log "Settings: merged host settings with baked plugin hooks"
    else
        log "Settings: copied from host (no baked hooks to preserve)"
    fi
fi

# --- baseline files --------------------------------------------------------
# Make sure the bare minimum exists so claude doesn't bail on first run
# (an entirely empty home is a rare case but possible if no host mounts).
[ -f "$CONTAINER_CLAUDE_JSON" ] || echo '{"mcpServers":{}}' > "$CONTAINER_CLAUDE_JSON"
[ -f "$CONTAINER_SETTINGS" ] || echo '{}' > "$CONTAINER_SETTINGS"

# --- 8. claude-mem runtime auth probe (C8) ---------------------------------
# Vibrator's launch helper resolved the project-scoped API key, team_id
# and project_id on the host (see internal/prereq/claudemem.go) and
# forwarded them as CLAUDE_MEM_SERVER_BETA_* env vars. The plugin's
# runtime-selector reads from ~/.claude-mem/settings.json (NOT directly
# from env), so we write the values to disk here. Then probe /healthz
# and POST /v1/events to surface auth distinction:
#   200/201/202 → key valid (best case)
#   400/422     → key valid, server rejected our empty body — fine
#   401/403     → key rejected; surface to user with rotation hint
if [ "$CLAUDE_MEM_RUNTIME" = "server-beta" ] && [ -n "$CLAUDE_MEM_SERVER_BETA_URL" ] \
        && command -v jq >/dev/null 2>&1; then
    mkdir -p "$HOME/.claude-mem"
    [ -f "$HOME/.claude-mem/settings.json" ] || echo '{}' > "$HOME/.claude-mem/settings.json"
    # Each (if key=="" then . else ... end) clause keeps the existing
    # value when the env var is empty — so a half-bootstrapped pin
    # doesn't blank out previously-good fields.
    jq --arg rt  "$CLAUDE_MEM_RUNTIME" \
       --arg url "$CLAUDE_MEM_SERVER_BETA_URL" \
       --arg key "${CLAUDE_MEM_SERVER_BETA_API_KEY:-}" \
       --arg tid "${CLAUDE_MEM_SERVER_BETA_TEAM_ID:-}" \
       --arg pid "${CLAUDE_MEM_SERVER_BETA_PROJECT_ID:-}" \
       '.CLAUDE_MEM_RUNTIME = $rt
        | .CLAUDE_MEM_SERVER_URL = $url
        | .CLAUDE_MEM_SERVER_BETA_URL = $url
        | (if $key == "" then . else .CLAUDE_MEM_SERVER_BETA_API_KEY = $key end)
        | (if $tid == "" then . else .CLAUDE_MEM_SERVER_BETA_TEAM_ID = $tid end)
        | (if $pid == "" then . else .CLAUDE_MEM_SERVER_BETA_PROJECT_ID = $pid end)' \
       "$HOME/.claude-mem/settings.json" > "$HOME/.claude-mem/settings.json.tmp" \
       && mv "$HOME/.claude-mem/settings.json.tmp" "$HOME/.claude-mem/settings.json"
    log "claude-mem: settings.json bootstrapped (runtime=server-beta)"

    # Reachability probe — short timeouts because we don't want to
    # block container startup on a slow host. If /healthz is down,
    # skip the auth probe entirely (no signal to be had).
    if curl -sf --connect-timeout 0.5 --max-time 1 \
            "$CLAUDE_MEM_SERVER_BETA_URL/healthz" >/dev/null 2>&1; then
        log "claude-mem: server-beta reachable at $CLAUDE_MEM_SERVER_BETA_URL"

        # Auth probe: empty POST. The server's exact response varies but
        # any 2xx/400/422 means "your bearer token was accepted, we just
        # didn't like the body." 401/403 = key rejected — that's what
        # the user needs to know about.
        _CM_PROBE_HTTP=$(curl -s -o /tmp/claude-mem-auth.out -w "%{http_code}" \
            -X POST \
            -H "Authorization: Bearer ${CLAUDE_MEM_SERVER_BETA_API_KEY:-}" \
            -H "Content-Type: application/json" \
            -d '{}' \
            "$CLAUDE_MEM_SERVER_BETA_URL/v1/events" 2>/dev/null)
        case "$_CM_PROBE_HTTP" in
            200|201|202)
                # Show this unconditionally — it's a positive
                # confirmation users like to see on first launch.
                printf '[vibrator] claude-mem: auth OK (POST /v1/events -> %s)\n' "$_CM_PROBE_HTTP" >&2
                ;;
            400|422)
                log "claude-mem: auth OK (POST /v1/events -> $_CM_PROBE_HTTP, server rejected empty body — expected)"
                ;;
            401|403)
                # Always surface auth failures — silent failure here
                # leaves the user wondering why claude-mem hooks fire
                # but nothing reaches the dashboard.
                printf '[vibrator] claude-mem: WARNING — auth REJECTED (POST /v1/events -> %s)\n' "$_CM_PROBE_HTTP" >&2
                printf '           The cached project-scoped key in %s/.vb may have been revoked.\n' "$WORKSPACE_PATH" >&2
                printf '           Re-run: vibrate prereqs bootstrap %s\n' "claude-mem-server-beta" >&2
                ;;
            *)
                log "claude-mem: auth probe got HTTP $_CM_PROBE_HTTP (unexpected — body in /tmp/claude-mem-auth.out)"
                ;;
        esac
    else
        log "claude-mem: WARNING — server-beta URL set ($CLAUDE_MEM_SERVER_BETA_URL) but /healthz unreachable; hooks will fail per-event"
    fi
fi

# --- 6. Plugin hook re-permission (C9) -------------------------------------
# Host-installed plugins (mounted into ~/.claude/plugins or copied from
# host settings) often lose the +x bit when they travel through bind
# mounts — macOS APFS preserves it, but the way Docker projects the
# mount can strip it in some edge cases (network-mounted homes, certain
# bind-propagation modes). Hooks are SHELL SCRIPTS that claude invokes
# via fork+exec, so without +x they fail silently — the user sees
# "hooks didn't run" with no log line explaining why.
#
# Blanket chmod on every entry: cheap (find walks a few dirs) and
# self-healing.
if [ -d "$HOME/.claude/plugins" ]; then
    find "$HOME/.claude/plugins" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true
    log "Plugins: re-permissioned hook scripts under ~/.claude/plugins"
fi

# --- 5b. GPG agent socket symlink (C5) -------------------------------------
# Vibrator's launch helper bind-mounts the host's gpg-agent extra-socket
# (when configured) to /gpg-agent-extra. gpg-inside-the-container looks
# for its socket at a different path — typically
# /run/user/<uid>/gnupg/S.gpg-agent or ~/.gnupg/S.gpg-agent depending
# on gpg version + distro defaults. Use `gpgconf --list-dirs
# agent-socket` to ask gpg itself where to put the symlink, then make
# it point at the mounted host socket.
#
# Effect: `git commit -S`, `gpg --sign`, etc. inside the container
# transparently use the HOST's private key without that key ever
# crossing the container boundary.
if [ -S "/gpg-agent-extra" ] && command -v gpgconf >/dev/null 2>&1; then
    EXPECTED_SOCKET=$(gpgconf --list-dirs agent-socket 2>/dev/null)
    if [ -n "$EXPECTED_SOCKET" ]; then
        # mkdir + chmod 700: gpg refuses to use a socket dir with
        # permissions wider than 700 (security paranoia is the whole
        # point of gpg). 2>/dev/null because the dir may already exist.
        mkdir -p "$(dirname "$EXPECTED_SOCKET")" 2>/dev/null || true
        chmod 700 "$(dirname "$EXPECTED_SOCKET")" 2>/dev/null || true
        ln -sf /gpg-agent-extra "$EXPECTED_SOCKET"
        log "GPG: agent socket linked at $EXPECTED_SOCKET"
    else
        # Fallback: gpgconf returned nothing useful (rare, but possible
        # on broken installs). Symlink to the default location.
        mkdir -p "$HOME/.gnupg" && chmod 700 "$HOME/.gnupg"
        ln -sf /gpg-agent-extra "$HOME/.gnupg/S.gpg-agent"
        log "GPG: agent socket linked at ~/.gnupg/S.gpg-agent (fallback)"
    fi
fi

# --- 6b. Per-profile MCP pruning (C7) --------------------------------------
# The image bakes mcpServers entries for the FULL feature set (every
# MCP an extension might want). Host settings.json can carry yet more.
# When the active profile is a subset (e.g. "backend" without playwright),
# Claude would still spawn those MCPs at session-start — wasting startup
# time and surfacing tools the user didn't ask for.
#
# Read VIBRATOR_FEATURES_LIST (comma-separated, baked into the image at
# build time) and delete any mcpServers entry whose feature isn't in
# the list. The pair table maps a feature ID to the mcpServers key it
# registered as. Extend it when new MCP-bearing features land.
_vb_feature_on() {
    # VIBRATOR_FEATURES_LIST is comma-separated (the Go generator's
    # featureIDsCSV format). Surround both haystack and needle with
    # commas so substring matching can't false-match (e.g. "node" must
    # not match "nodemon").
    case ",${VIBRATOR_FEATURES_LIST:-}," in
        *",$1,"*) return 0 ;;
        *)        return 1 ;;
    esac
}

if command -v jq >/dev/null 2>&1 && [ -f "$CONTAINER_CLAUDE_JSON" ]; then
    for _pair in \
            "playwright|playwright" \
            "serena|serena" \
            "claude-mem|claude-mem"; do
        _feat="${_pair%|*}"
        _mcp_key="${_pair#*|}"
        _vb_feature_on "$_feat" && continue
        # Only rewrite the file if the key is actually present — avoids
        # a spurious file mutation (and the resulting mtime bump) when
        # there's nothing to prune.
        if jq -e --arg k "$_mcp_key" '.mcpServers[$k]' "$CONTAINER_CLAUDE_JSON" >/dev/null 2>&1; then
            jq --arg k "$_mcp_key" 'del(.mcpServers[$k])' "$CONTAINER_CLAUDE_JSON" \
                > "$CONTAINER_CLAUDE_JSON.tmp" \
                && mv "$CONTAINER_CLAUDE_JSON.tmp" "$CONTAINER_CLAUDE_JSON"
            log "MCP: pruned '$_mcp_key' (feature '$_feat' not in profile)"
        fi
    done
fi

# --- 7. Re-enable baked plugins (C10) --------------------------------------
# Claude tracks "which plugins are enabled" in settings.json
# `.enabledPlugins`. The host's settings.json has no knowledge of
# plugins baked into THIS image (e.g. claude-mem, cc-thingz installed
# by extensions in Stage 4) — so step 5's wholesale copy from host
# silently disables them.
#
# Re-read the image's installed_plugins.json (written by `claude plugin
# install` at build time) and merge each plugin name back into
# settings.json's enabledPlugins map. Existing entries from host
# settings.json are preserved (the `+` merge is host-side wins on
# duplicates, but baked plugins won't appear in host so it's additive).
INSTALLED_PLUGINS_JSON="$HOME/.claude/plugins/installed_plugins.json"
if [ -f "$INSTALLED_PLUGINS_JSON" ] && command -v jq >/dev/null 2>&1; then
    # `.plugins | keys` yields the plugin IDs as an array; map to
    # {id: true} objects and combine into one — that's the shape
    # .enabledPlugins expects. Null guard for malformed inputs.
    BAKED_PLUGINS=$(jq -r '.plugins // {} | keys | map({(.): true}) | add // {}' \
        "$INSTALLED_PLUGINS_JSON" 2>/dev/null || echo "{}")

    if [ -n "$BAKED_PLUGINS" ] && [ "$BAKED_PLUGINS" != "null" ] && [ "$BAKED_PLUGINS" != "{}" ]; then
        jq --argjson p "$BAKED_PLUGINS" \
            '.enabledPlugins = ((.enabledPlugins // {}) + $p)' \
            "$CONTAINER_SETTINGS" > "$CONTAINER_SETTINGS.tmp" \
            && mv "$CONTAINER_SETTINGS.tmp" "$CONTAINER_SETTINGS"
        log "Plugins: re-enabled baked plugins in settings.json"
    fi
fi

# --- 7b. De-duplicate integration-managed MCPs (C10b) ----------------------
# Some MCP servers are owned by vibrator's integration layer: claude-exec
# writes a single host-aware ~/.claude.json entry (http when the host
# server is up, stdio otherwise). If the SAME server is ALSO present as a
# Claude Code plugin (e.g. `serena@claude-plugins-official`, pulled in by
# host-config mirroring), the harness loads BOTH — a redundant, host-blind
# duplicate. The integration is the single source of truth, so disable any
# enabled plugin whose id collides with an integration MCP name.
INTEGRATIONS_MANIFEST=/etc/vibrator/integrations.json
if [ -f "$INTEGRATIONS_MANIFEST" ] && [ -f "$CONTAINER_SETTINGS" ] \
        && command -v jq >/dev/null 2>&1; then
    # The plugin key shape is "<id>@<marketplace>"; the part before '@'
    # is what we compare against the integration MCP names.
    MANAGED_MCP_NAMES=$(jq -r '[.[].mcp.name // empty] | unique | .[]' \
        "$INTEGRATIONS_MANIFEST" 2>/dev/null)
    for _mcp_name in $MANAGED_MCP_NAMES; do
        jq --arg n "$_mcp_name" \
            '.enabledPlugins = ((.enabledPlugins // {})
                | with_entries(select((.key | split("@")[0]) != $n)))' \
            "$CONTAINER_SETTINGS" > "$CONTAINER_SETTINGS.tmp" 2>/dev/null \
            && mv "$CONTAINER_SETTINGS.tmp" "$CONTAINER_SETTINGS" \
            && log "Plugins: dropped plugin '$_mcp_name@*' (owned by integration layer)"
    done
fi

# --- 7c. Skip hooks needing a missing tool (C12) ---------------------------
# A Claude hook that shells out to e.g. `node` on an image without node fails
# on EVERY matching event with a noisy "node: not found". The hook can't do
# anything useful anyway, so drop it (logging what we skipped) to spare the
# agent the spam. We check real PATH availability (command -v) rather than the
# baked feature list, so a tool installed by other means is respected.
#
# The host-side launch prompt (internal/app/hooks.go) offers to INSTALL the
# tool instead; this guard is the always-on safety net that also covers
# plugin-installed hooks and non-interactive (CI) runs. Keep the tool list in
# sync with internal/hooktools.toolFeature.
if command -v jq >/dev/null 2>&1 && [ -f "$CONTAINER_SETTINGS" ]; then
    _vb_hook_tools=$(jq -r '.hooks // {} | .[]? | .[]? | .hooks[]? | .command // empty' \
        "$CONTAINER_SETTINGS" 2>/dev/null \
        | grep -oE '\b(node|npm|npx|bun|python3?|pip3?|uvx?|go|gh|psql|pg_dump|pg_restore|aider|ralphex|codex|playwright)\b' \
        | sort -u)
    _vb_missing=""
    for _t in $_vb_hook_tools; do
        command -v "$_t" >/dev/null 2>&1 || _vb_missing="$_vb_missing $_t"
    done
    _vb_missing=$(printf '%s' "$_vb_missing" | sed 's/^ *//; s/ *$//')
    if [ -n "$_vb_missing" ]; then
        # Build a word-boundary alternation of the missing tools, e.g.
        # \b(node|python3)\b — used by jq's test() to find offending commands.
        _vb_re="\\b($(printf '%s' "$_vb_missing" | tr ' ' '|'))\\b"
        # Remove inner hooks whose command references a missing tool, then drop
        # now-empty groups and events. Falls back to leaving settings untouched
        # if jq errors (malformed input) — never block startup over a hook.
        if jq --arg re "$_vb_re" '
            .hooks = (
                (.hooks // {})
                | with_entries(.value |= (
                      map(.hooks = ((.hooks // []) | map(select((.command // "") | test($re) | not))))
                      | map(select((.hooks | length) > 0))
                  ))
                | with_entries(select((.value | length) > 0))
            )
        ' "$CONTAINER_SETTINGS" > "$CONTAINER_SETTINGS.tmp" 2>/dev/null; then
            mv "$CONTAINER_SETTINGS.tmp" "$CONTAINER_SETTINGS"
            printf '[vibrator] hooks: skipped hook(s) needing missing tool(s): %s\n' \
                "$(printf '%s' "$_vb_missing" | tr ' ' ',')" >&2
        else
            rm -f "$CONTAINER_SETTINGS.tmp" 2>/dev/null || true
        fi
    fi
fi

# --- 9. Codex config materialization ----------------------------------------
# Codex mounts the host config.toml to a .host sidecar; reconcile it with the
# vibrator-baked MCP servers here (see codex-materialize.sh). Gated on the
# harness + the script's presence (only codex images ship it), so this is a
# silent no-op on every other harness.
if [ "$VIBRATOR_HARNESS" = "codex" ] && [ -x /usr/local/bin/codex-materialize ]; then
    /usr/local/bin/codex-materialize
    log "codex: config materialized"
fi

# --- 10. OpenCode config materialization -------------------------------------
# OpenCode mounts the host ~/.config/opencode to a .host sidecar dir;
# reconcile it with the baked extension artifacts here (see
# opencode-materialize.sh). Gated on the harness + the script's presence
# (only opencode images ship it), so this is a silent no-op elsewhere.
if [ "$VIBRATOR_HARNESS" = "opencode" ] && [ -x /usr/local/bin/opencode-materialize ]; then
    /usr/local/bin/opencode-materialize
    log "opencode: config materialized"
fi

# --- 11. Pi config materialization --------------------------------------------
# Pi mounts the host ~/.pi to a .host sidecar tree (with rw carve-outs for
# agent/auth.json and agent/sessions); reconcile it with the baked extension
# artifacts here (see pi-materialize.sh). Gated on the harness + the
# script's presence (only pi images ship it), so this is a silent no-op
# elsewhere.
if [ "$VIBRATOR_HARNESS" = "pi" ] && [ -x /usr/local/bin/pi-materialize ]; then
    /usr/local/bin/pi-materialize
    log "pi: config materialized"
fi

# --- readiness signal -------------------------------------------------------
# Drop a sentinel file so `vibrate --login` can poll-wait for the full
# entrypoint setup (config merge, rules copy, settings merge) to finish
# before injecting `claude auth login` via docker exec. Without this,
# there is a small race between the login exec and the config-merge step
# above. The file is created just before exec so only one process ever
# writes it, and exec replaces us anyway so it is effectively immutable.
touch /tmp/.vibrator-entrypoint-done 2>/dev/null || true

# --- exec the user's command -----------------------------------------------
# `exec "$@"` replaces the entrypoint shell with the user's process so
# the container's PID 1 is the user's shell, not us — required for
# proper signal handling (Ctrl-C reaches the shell, not a wrapper).
exec "$@"
