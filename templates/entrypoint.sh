#!/bin/sh
# Container entrypoint: runs once at container creation.
# Merges host Claude config, sets up GPG, detects Serena, starts agent-browser.

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
  RULES_COUNT=$(ls -1 "$HOME/.claude/rules-host/"*.md 2>/dev/null | wc -l)
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: copied $RULES_COUNT host rules"
fi

# Add container-specific rules
if [ -d /opt/container-rules ]; then
  cp /opt/container-rules/*.md "$HOME/.claude/rules/" 2>/dev/null || true
  CONTAINER_RULES_COUNT=$(ls -1 /opt/container-rules/*.md 2>/dev/null | wc -l)
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: added $CONTAINER_RULES_COUNT container rules"
fi

TOTAL_RULES=$(ls -1 "$HOME/.claude/rules/"*.md 2>/dev/null | wc -l)
[ "$VIBRATOR_VERBOSE" = "1" ] && echo "Claude rules: $TOTAL_RULES total rules loaded"

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

# --- Detect and configure Langfuse observability ---
LANGFUSE_PORT="${LANGFUSE_PORT:-3050}"
LANGFUSE_DETECTED=0
LANGFUSE_MODE=""

mkdir -p "$HOME/.claude/hooks"
mkdir -p "$HOME/.claude"
[ ! -f "$HOME/.claude/settings.json" ] && echo '{}' > "$HOME/.claude/settings.json"

# Mode 1 (host): User has a Langfuse hook on host with TRACE_TO_LANGFUSE configured
HOST_HOOK=""
if [ -d "$HOME/.claude/hooks-host" ]; then
  for f in "$HOME/.claude/hooks-host"/langfuse*hook*.py "$HOME/.claude/hooks-host"/langfuse_hook*.py; do
    [ -f "$f" ] && HOST_HOOK="$f" && break
  done
fi

if [ -n "$HOST_HOOK" ] && \
   [ -f "$HOME/.claude/settings.json" ] && \
   jq -e '.env.TRACE_TO_LANGFUSE == "true"' "$HOME/.claude/settings.json" >/dev/null 2>&1; then
  LANGFUSE_DETECTED=1
  LANGFUSE_MODE="host"

  # Copy host's hook file into container hooks dir
  HOOK_BASENAME=$(basename "$HOST_HOOK")
  cp "$HOST_HOOK" "$HOME/.claude/hooks/$HOOK_BASENAME"
  chmod +x "$HOME/.claude/hooks/$HOOK_BASENAME"

  # Fix hook paths in settings.json: replace host user's home path with container user's home
  # Host settings may reference /Users/foo/.claude/hooks/... or /home/foo/.claude/hooks/...
  # We need these to point to /home/$CONTAINER_USER/.claude/hooks/...
  if jq -e '.hooks' "$HOME/.claude/settings.json" >/dev/null 2>&1; then
    jq --arg home "$HOME" \
      'walk(if type == "string" and test("/(?:Users|home)/[^/]+/\\.claude/hooks/") then gsub("/(?:Users|home)/[^/]+/\\.claude/hooks/"; ($home + "/.claude/hooks/")) else . end)' \
      "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
      mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
  fi

  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: using host hook ($HOOK_BASENAME)"

# Mode 2 (auto): Langfuse server detected on host, use embedded hook
elif curl -sf --connect-timeout 0.3 --max-time 0.5 "http://localhost:$LANGFUSE_PORT" -o /dev/null 2>/dev/null; then
  LANGFUSE_DETECTED=1
  LANGFUSE_MODE="auto"
  LANGFUSE_URL="http://localhost:$LANGFUSE_PORT"

  if [ -f /opt/langfuse-hook.py ]; then
    cp /opt/langfuse-hook.py "$HOME/.claude/hooks/langfuse-hook.py"
    chmod +x "$HOME/.claude/hooks/langfuse-hook.py"

    jq --arg hook "$HOME/.claude/hooks/langfuse-hook.py" --arg url "$LANGFUSE_URL" \
      '.hooks.stop = [$hook] | .env.TRACE_TO_LANGFUSE = "true" | .env.LANGFUSE_HOST = $url' \
      "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
      mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
  fi

  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: auto-detected server at $LANGFUSE_URL"

elif curl -sf --connect-timeout 0.3 --max-time 0.5 "http://host.docker.internal:$LANGFUSE_PORT" -o /dev/null 2>/dev/null; then
  LANGFUSE_DETECTED=1
  LANGFUSE_MODE="auto"
  LANGFUSE_URL="http://host.docker.internal:$LANGFUSE_PORT"

  if [ -f /opt/langfuse-hook.py ]; then
    cp /opt/langfuse-hook.py "$HOME/.claude/hooks/langfuse-hook.py"
    chmod +x "$HOME/.claude/hooks/langfuse-hook.py"

    jq --arg hook "$HOME/.claude/hooks/langfuse-hook.py" --arg url "$LANGFUSE_URL" \
      '.hooks.stop = [$hook] | .env.TRACE_TO_LANGFUSE = "true" | .env.LANGFUSE_HOST = $url' \
      "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
      mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
  fi

  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: auto-detected server at $LANGFUSE_URL"

else
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: not configured"
fi

# Export for use in welcome message
export LANGFUSE_DETECTED
export LANGFUSE_MODE

# --- Start agent-browser MCP hub in background ---
if command -v agent-browser >/dev/null 2>&1; then
  if ! pgrep -x agent-browser >/dev/null 2>&1; then
    mkdir -p "$HOME/.agent-browser"
    agent-browser > "$HOME/.agent-browser/agent-browser.log" 2>&1 &

    for i in $(seq 1 10); do
      if curl -sf --connect-timeout 0.5 --max-time 0.5 "http://localhost:8087/sse" -o /dev/null 2>/dev/null; then
        if ! jq -e '.mcpServers["agent-browser"]' "$HOME/.claude.json" >/dev/null 2>&1; then
          jq '.mcpServers["agent-browser"] = {type: "sse", url: "http://localhost:8087/sse"}' \
            "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && \
            mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
        fi
        [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Agent Browser: started (Web UI: http://localhost:8080/ui/)"
        break
      fi
      sleep 0.1
    done
  else
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Agent Browser: already running"
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
  cd "$WORKSPACE_PATH"
fi

exec "$@"
