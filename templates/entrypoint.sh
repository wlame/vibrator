#!/bin/sh
# Container entrypoint: runs once at container creation.
# Merges host Claude config, sets up GPG, detects Serena.

# --- Merge Claude config from host ---
if [ -f "$HOME/.claude.host.json" ]; then
  CONFIG_KEYS="oauthAccount hasSeenTasksHint userID hasCompletedOnboarding lastOnboardingVersion subscriptionNoticeCount hasAvailableSubscription s1mAccessCache"

  JQ_EXPR=""
  for key in $CONFIG_KEYS; do
    if [ -n "$JQ_EXPR" ]; then JQ_EXPR="$JQ_EXPR, "; fi
    JQ_EXPR="$JQ_EXPR\"$key\": .$key"
  done

  HOST_CONFIG=$(jq -c "{$JQ_EXPR, \"bypassPermissionsModeAccepted\": true}" "$HOME/.claude.host.json" 2>/dev/null || echo "")

  if [ -n "$HOST_CONFIG" ] && [ "$HOST_CONFIG" != "null" ] && [ "$HOST_CONFIG" != "{}" ]; then
    if [ -f "$HOME/.claude.json" ]; then
      jq ". * $HOST_CONFIG" "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
    else
      echo "$HOST_CONFIG" | jq . > "$HOME/.claude.json"
    fi
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude config merged from host file"
  else
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "No valid config found in host file"
  fi
else
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "No host Claude config file mounted"
fi

# --- Merge Claude rules: host (read-only) + container-specific ---
mkdir -p "$HOME/.claude/rules"

# Copy host rules if mounted (read-only at rules-host)
if [ -d "$HOME/.claude/rules-host" ]; then
  cp -r "$HOME/.claude/rules-host/"*.md "$HOME/.claude/rules/" 2>/dev/null || true
  # shellcheck disable=SC2012
  RULES_COUNT=$(ls -1 "$HOME/.claude/rules-host/"*.md 2>/dev/null | wc -l)
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: copied $RULES_COUNT host rules"
fi

# Add container-specific rules
if [ -d /opt/container-rules ]; then
  cp /opt/container-rules/*.md "$HOME/.claude/rules/" 2>/dev/null || true
  # shellcheck disable=SC2012
  CONTAINER_RULES_COUNT=$(ls -1 /opt/container-rules/*.md 2>/dev/null | wc -l)
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: added $CONTAINER_RULES_COUNT container rules"
fi

# shellcheck disable=SC2012
TOTAL_RULES=$(ls -1 "$HOME/.claude/rules/"*.md 2>/dev/null | wc -l)
[ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: $TOTAL_RULES total rules loaded"

# Generate user identity rule so Claude uses the real username in code
if [ -n "$CONTAINER_USER" ] && [ "$CONTAINER_USER" != "claude-user" ]; then
  cat > "$HOME/.claude/rules/user-identity.md" <<USERRULE
# User Identity

The user's system username is: $CONTAINER_USER
Use "$CONTAINER_USER" whenever a username, login, author name, or owner is needed in generated code — for example in package.json "author", Go module paths, GitHub references, shebang comments, copyright headers, git config examples, or placeholder values. Never use generic placeholders like "user", "username", "your-name", or "your-username" when this real username is available.
USERRULE
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: generated user-identity rule for $CONTAINER_USER"
fi

# --- Initialize settings.json from host (allows runtime modification) ---
# After copying, rewrite macOS-style absolute hook paths (/Users/<user>/.claude/)
# to the container home path. This prevents "no such file" errors when hooks
# were configured on macOS but the hooks directory only exists there.
#
# Hooks merge: the image's settings.json may carry baked-in plugin hooks
# (e.g., from build-time `claude-mem install`). A wholesale copy from host
# would wipe those. We snapshot the baked .hooks BEFORE the copy and merge
# them back AFTER, with host's hook entries appended per-event so both fire.
_copy_and_fix_settings() {
  local src="$1"
  # 1. Snapshot any pre-existing baked hooks (e.g., from claude-mem)
  local baked_hooks="{}"
  if [ -f "$HOME/.claude/settings.json" ]; then
    baked_hooks=$(jq -c '.hooks // {}' "$HOME/.claude/settings.json" 2>/dev/null || echo "{}")
  fi
  # 2. Replace with host's settings (with path rewrites)
  cp "$src" "$HOME/.claude/settings.json"
  sed -i "s|/Users/[^/]*/.claude/|$HOME/.claude/|g" "$HOME/.claude/settings.json" 2>/dev/null || true
  # 3. Merge baked hooks back in alongside host hooks — per-event arrays
  #    are concatenated so both sets of handlers fire.
  if [ "$baked_hooks" != "{}" ] && [ "$baked_hooks" != "null" ]; then
    jq --argjson baked "$baked_hooks" '
      .hooks = (
        (.hooks // {}) as $host |
        (($baked + $host) | keys) as $all_keys |
        reduce $all_keys[] as $k ({};
          .[$k] = (($baked[$k] // []) + ($host[$k] // []))
        )
      )
    ' "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" \
      && mv "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Settings: merged baked plugin hooks with host hooks"
  fi
}

if [ -f "$HOME/.claude/settings.host.json" ]; then
  _copy_and_fix_settings "$HOME/.claude/settings.host.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Settings: copied from host settings.json (hook paths rewritten for container)"
elif [ -f "$HOME/.claude-full/settings.json" ]; then
  _copy_and_fix_settings "$HOME/.claude-full/settings.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Settings: copied from full-mount settings.json (hook paths rewritten for container)"
fi

# --- Link GPG agent socket if forwarded ---
if [ -S "/gpg-agent-extra" ]; then
  EXPECTED_SOCKET=$(gpgconf --list-dirs agent-socket 2>/dev/null)
  if [ -n "$EXPECTED_SOCKET" ]; then
    mkdir -p "$(dirname "$EXPECTED_SOCKET")"
    chmod 700 "$(dirname "$EXPECTED_SOCKET")"
    ln -sf /gpg-agent-extra "$EXPECTED_SOCKET"
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "GPG agent socket linked at $EXPECTED_SOCKET"
  else
    mkdir -p ~/.gnupg && chmod 700 ~/.gnupg
    ln -sf /gpg-agent-extra ~/.gnupg/S.gpg-agent
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "GPG agent socket linked at ~/.gnupg/S.gpg-agent (fallback)"
  fi
fi

# --- Helper: is FEATURE_NAME in the build-time feature list? ---
# Reads $VIBRATOR_FEATURES_LIST (space-separated, baked into the image at
# build time by image::build via --build-arg VIBRATOR_FEATURES). Used to
# gate runtime MCP wiring and prune leftover entries that the host
# settings/baked image left behind for features the profile disabled.
_vb_feature_on() {
  case " ${VIBRATOR_FEATURES_LIST:-} " in
    *" $1 "*) return 0 ;;
    *)        return 1 ;;
  esac
}

# --- Detect host Serena server and configure MCP transport ---
SERENA_PORT="${SERENA_PORT:-8765}"

if [ ! -f "$HOME/.claude.json" ]; then
  echo '{"mcpServers":{}}' > "$HOME/.claude.json"
fi

if _vb_feature_on serena; then
  if curl -sf --connect-timeout 0.3 --max-time 0.5 "http://host.docker.internal:$SERENA_PORT/mcp" 2>/dev/null | grep -q "mcp-session-id\|jsonrpc" || \
     curl -sf --connect-timeout 0.3 --max-time 0.5 -I "http://host.docker.internal:$SERENA_PORT/mcp" 2>/dev/null | grep -q "mcp-session-id"; then
    jq --arg url "http://host.docker.internal:$SERENA_PORT/mcp" \
      '.mcpServers.serena = {type: "http", url: $url}' \
      "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && \
      mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Serena: connected to host server at host.docker.internal:$SERENA_PORT"
  else
    jq '.mcpServers.serena = {
      type: "stdio",
      command: "uvx",
      args: ["--from", "git+https://github.com/oraios/serena", "serena", "start-mcp-server", "--project-from-cwd"]
    }' "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && \
      mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Serena: no host server detected, using built-in stdio mode"
  fi
fi

# --- Prune MCP servers for features the active profile disabled ---
# The image bakes mcpServers entries for the full feature set, and host
# settings.json may carry additional ones. Without pruning, Claude launches
# every entry at session start — so a "backend" profile would still spawn
# `npm exec @playwright/mcp` even though playwright isn't in the profile.
# The pair format is "feature_name|mcp_server_key".
if command -v jq >/dev/null 2>&1 && [ -f "$HOME/.claude.json" ]; then
  for _pair in \
      "playwright|playwright" \
      "serena|serena" \
      "claude-mem|claude-mem"; do
    _feat="${_pair%|*}"
    _mcp_key="${_pair#*|}"
    _vb_feature_on "$_feat" && continue
    if jq -e --arg k "$_mcp_key" '.mcpServers[$k]' "$HOME/.claude.json" >/dev/null 2>&1; then
      jq --arg k "$_mcp_key" 'del(.mcpServers[$k])' "$HOME/.claude.json" \
        > "$HOME/.claude.json.tmp" && mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
      [ "$VIBRATOR_VERBOSE" = "1" ] && echo "MCP: removed '$_mcp_key' (feature '$_feat' not in profile)"
    fi
  done
fi

# --- claude-mem server-beta wiring (container-side, runtime config only) ---
# Vibrator's docker_cmd already resolved the project-scoped API key, team_id,
# and project_id on the HOST (see claude_mem_bootstrap.sh) and forwarded
# them as env vars. The container now just needs to:
#   1. write them into ~/.claude-mem/settings.json so the plugin's
#      runtime-selector reads RUNTIME from disk (env vars alone aren't
#      enough — the plugin loads from settings.json at session-start)
#   2. probe /healthz so VIBRATOR_VERBOSE users see whether the host stack
#      is up
#   3. probe POST /v1/events for auth — distinguishes "forwarding broken"
#      from "key revoked"
#
# All SQL was moved host-side, so there is NO psql in this container, NO
# DATABASE_URL forwarded, and the entrypoint cannot reach the canonical store.
if [ "$CLAUDE_MEM_RUNTIME" = "server-beta" ] && [ -n "$CLAUDE_MEM_SERVER_BETA_URL" ]; then
  mkdir -p "$HOME/.claude-mem"
  [ -f "$HOME/.claude-mem/settings.json" ] || echo '{}' > "$HOME/.claude-mem/settings.json"
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
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "claude-mem: settings.json bootstrapped (runtime=server-beta)"

  if curl -sf --connect-timeout 0.5 --max-time 1 "$CLAUDE_MEM_SERVER_BETA_URL/healthz" >/dev/null 2>&1; then
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "claude-mem: server-beta reachable at $CLAUDE_MEM_SERVER_BETA_URL"

    # Auth probe: POST an empty body to /v1/events. Server responds:
    #   200/201/202 → key valid, event accepted (best case)
    #   400/422     → key valid, body invalid (auth still good — what we care about)
    #   401/403     → key rejected or wrong scope
    # Distinguishes "vibrator's forwarding is correct" from "plugin isn't using it".
    _CM_PROBE_HTTP=$(curl -s -o /tmp/claude-mem-auth.out -w "%{http_code}" \
      -X POST \
      -H "Authorization: Bearer ${CLAUDE_MEM_SERVER_BETA_API_KEY:-}" \
      -H "Content-Type: application/json" \
      -d '{}' \
      "$CLAUDE_MEM_SERVER_BETA_URL/v1/events" 2>/dev/null)
    case "$_CM_PROBE_HTTP" in
      200|201|202)
        echo "claude-mem: auth OK (POST /v1/events → $_CM_PROBE_HTTP)"
        ;;
      400|422)
        [ "$VIBRATOR_VERBOSE" = "1" ] && echo "claude-mem: auth OK (POST /v1/events → $_CM_PROBE_HTTP, server rejected empty body — expected)"
        ;;
      401|403)
        echo "[vibrator] claude-mem: WARNING — auth REJECTED (POST /v1/events → $_CM_PROBE_HTTP)"
        echo "           The cached project-scoped key in $WORKSPACE_PATH/.vb.env may have"
        echo "           been revoked. Delete the CLAUDE_MEM_SERVER_BETA_* lines from .vb.env"
        echo "           and re-run \`vibrate\` to mint a fresh key. Body:"
        cat /tmp/claude-mem-auth.out 2>/dev/null
        ;;
      *)
        [ "$VIBRATOR_VERBOSE" = "1" ] && echo "claude-mem: auth probe got HTTP $_CM_PROBE_HTTP (unexpected — body in /tmp/claude-mem-auth.out)"
        ;;
    esac

    if [ -n "$CLAUDE_MEM_SERVER_BETA_PROJECT_ID" ] && [ "$VIBRATOR_VERBOSE" = "1" ]; then
      echo "claude-mem: project_id=$CLAUDE_MEM_SERVER_BETA_PROJECT_ID (resolved host-side)"
    fi
  else
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "claude-mem: WARNING — server-beta URL set ($CLAUDE_MEM_SERVER_BETA_URL) but /healthz unreachable; hooks will fail per-event"
  fi
fi

mkdir -p "$HOME/.claude/hooks"
mkdir -p "$HOME/.claude"
[ ! -f "$HOME/.claude/settings.json" ] && echo '{}' > "$HOME/.claude/settings.json"

# Ensure plugin hook scripts are executable (host-installed plugins may lack +x)
find "$HOME/.claude/plugins" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true

# claude-mem hooks are wired in two steps now:
#   1. Build time: `npx claude-mem install` populates the image's
#      ~/.claude/settings.json with hook entries (Dockerfile Stage 3)
#   2. Runtime: _copy_and_fix_settings above snapshots those baked hooks
#      before copying host settings.json and merges them back in afterward
# So no runtime re-install needed — and rerunning the installer at
# entrypoint hits the "Overwrite? No" default of non-interactive mode,
# which silently cancels. Confirm wiring with:
#   grep -c worker-service.cjs ~/.claude/settings.json   # expect ≥ 6

# Re-enable baked-in plugins after settings copy (host settings.json doesn't have them)
if [ -f "$HOME/.claude/plugins/installed_plugins.json" ]; then
  BAKED_PLUGINS=$(jq -r '.plugins // {} | keys | map({(.): true}) | add // {}' \
    "$HOME/.claude/plugins/installed_plugins.json" 2>/dev/null)
  if [ -n "$BAKED_PLUGINS" ] && [ "$BAKED_PLUGINS" != "null" ] && [ "$BAKED_PLUGINS" != "{}" ]; then
    jq --argjson p "$BAKED_PLUGINS" '.enabledPlugins = ((.enabledPlugins // {}) + $p)' \
      "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
      mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
  fi
fi

# --- Create workspace parent directories if needed ---
if [ -n "$WORKSPACE_PATH" ]; then
  WORKSPACE_PARENT=$(dirname "$WORKSPACE_PATH")

  # Create parent directories if they don't exist
  if [ ! -d "$WORKSPACE_PARENT" ]; then
    sudo mkdir -p "$WORKSPACE_PARENT" 2>/dev/null || true
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Workspace: created parent directory $WORKSPACE_PARENT"
  fi

  # Ensure container user can access the workspace
  # The workspace itself is mounted, but we need access to parent dirs
  if [ -d "$WORKSPACE_PARENT" ] && [ ! -w "$WORKSPACE_PARENT" ]; then
    sudo chown "$CONTAINER_USER:$(id -gn)" "$WORKSPACE_PARENT" 2>/dev/null || true
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Workspace: adjusted permissions on $WORKSPACE_PARENT"
  fi
fi

# --- Change to workspace ---
if [ -n "$WORKSPACE_PATH" ] && [ -d "$WORKSPACE_PATH" ]; then
  cd "$WORKSPACE_PATH" || exit 1
fi

exec "$@"
