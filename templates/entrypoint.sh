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
      jq ". * $HOST_CONFIG" "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && mv "$HOME/.claude.json.tmp" "$HOME/.claude.json"
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
    mv "$HOME/.claude.json.tmp" "$HOME/.claude.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Serena: connected to host server at host.docker.internal:$SERENA_PORT"
else
  jq '.mcpServers.serena = {
    type: "stdio",
    command: "uvx",
    args: ["--from", "git+https://github.com/oraios/serena", "serena", "start-mcp-server", "--project-from-cwd"]
  }' "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && \
    mv "$HOME/.claude.json.tmp" "$HOME/.claude.json"
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Serena: no host server detected, using built-in stdio mode"
fi

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
            mv "$HOME/.claude.json.tmp" "$HOME/.claude.json"
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

# --- Change to workspace ---
if [ -n "$WORKSPACE_PATH" ] && [ -d "$WORKSPACE_PATH" ]; then
  cd "$WORKSPACE_PATH"
fi

exec "$@"
