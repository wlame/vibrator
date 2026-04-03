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
if [ -f "$HOME/.claude/settings.host.json" ]; then
  cp "$HOME/.claude/settings.host.json" "$HOME/.claude/settings.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Settings: copied from host settings.json"
elif [ -f "$HOME/.claude-full/settings.json" ]; then
  cp "$HOME/.claude-full/settings.json" "$HOME/.claude/settings.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Settings: copied from full-mount settings.json"
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

# --- Detect host Serena server and configure MCP transport ---
SERENA_PORT="${SERENA_PORT:-8765}"

if [ ! -f "$HOME/.claude.json" ]; then
  echo '{"mcpServers":{"serena":{"type":"stdio","command":"uvx","args":["--from","git+https://github.com/oraios/serena","serena","start-mcp-server","--project-from-cwd"]}}}' > "$HOME/.claude.json"
fi

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

mkdir -p "$HOME/.claude/hooks"
mkdir -p "$HOME/.claude"
[ ! -f "$HOME/.claude/settings.json" ] && echo '{}' > "$HOME/.claude/settings.json"

# Ensure plugin hook scripts are executable (host-installed plugins may lack +x)
find "$HOME/.claude/plugins" -name "*.sh" -exec chmod +x {} \; 2>/dev/null || true

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
