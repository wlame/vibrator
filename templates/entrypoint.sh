#!/bin/sh
# Container entrypoint: runs once at container creation.
# Merges host Claude config, sets up GPG, detects Serena, optionally starts agent-browser.

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
# Claude Code hooks format (PascalCase event names, nested structure):
#   {"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "..."}]}]}}
LANGFUSE_PORT="${LANGFUSE_PORT:-3050}"
LANGFUSE_DETECTED=0
LANGFUSE_MODE=""
CONTAINER_HOOK_CMD="python3 /opt/langfuse-hook.py"

mkdir -p "$HOME/.claude/hooks"
mkdir -p "$HOME/.claude"
[ ! -f "$HOME/.claude/settings.json" ] && echo '{}' > "$HOME/.claude/settings.json"

# Helper: remove langfuse Stop hooks from settings.json
# Filters out hook entries whose command contains "langfuse" (case-insensitive)
langfuse_remove_hooks() {
  jq '
    if .hooks.Stop then
      .hooks.Stop = [
        .hooks.Stop[] |
        .hooks = [.hooks[] | select(.command | test("langfuse"; "i") | not)] |
        select(.hooks | length > 0)
      ]
    else . end |
    if .hooks.Stop == [] then del(.hooks.Stop) else . end |
    if .hooks == {} then del(.hooks) else . end
  ' "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
    mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
}

# Helper: remove langfuse env vars from settings.json
langfuse_remove_env() {
  jq '
    if .env then
      .env |= with_entries(select(.key | test("LANGFUSE|TRACE_TO_LANGFUSE"; "i") | not))
    else . end |
    if .env == {} then del(.env) else . end
  ' "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
    mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
}

# Helper: set langfuse Stop hook in settings.json with proper Claude Code format
langfuse_set_hook() {
  local hook_cmd="$1"
  local langfuse_url="$2"

  # First remove any existing langfuse hooks
  langfuse_remove_hooks

  # Add new hook in correct Claude Code format and set env vars
  # Also rewrite LANGFUSE_HOST: localhost -> host.docker.internal for container networking
  jq --arg cmd "$hook_cmd" --arg url "$langfuse_url" '
    .hooks.Stop = ((.hooks.Stop // []) + [{"hooks": [{"type": "command", "command": $cmd}]}]) |
    .env.TRACE_TO_LANGFUSE = "true" |
    .env.LANGFUSE_HOST = $url |
    if .env.LANGFUSE_HOST then
      .env.LANGFUSE_HOST = (.env.LANGFUSE_HOST | gsub("localhost"; "host.docker.internal") | gsub("127\\.0\\.0\\.1"; "host.docker.internal"))
    else . end
  ' "$HOME/.claude/settings.json" > "$HOME/.claude/settings.json.tmp" && \
    mv -f "$HOME/.claude/settings.json.tmp" "$HOME/.claude/settings.json"
}

# Mode 1 (host): User has Langfuse configured in host settings.json
# Detect by checking for TRACE_TO_LANGFUSE env var AND langfuse-related Stop hooks
if [ -f "$HOME/.claude/settings.json" ] && \
   jq -e '.env.TRACE_TO_LANGFUSE == "true"' "$HOME/.claude/settings.json" >/dev/null 2>&1; then

  # Extract LANGFUSE_HOST from host settings, default to host.docker.internal
  HOST_LANGFUSE_URL=$(jq -r '.env.LANGFUSE_HOST // ""' "$HOME/.claude/settings.json")
  if [ -z "$HOST_LANGFUSE_URL" ]; then
    HOST_LANGFUSE_URL="http://host.docker.internal:$LANGFUSE_PORT"
  fi

  # Rewrite localhost/127.0.0.1 to host.docker.internal for container networking
  CONTAINER_LANGFUSE_URL=$(echo "$HOST_LANGFUSE_URL" | sed -e 's|localhost|host.docker.internal|g' -e 's|127\.0\.0\.1|host.docker.internal|g')

  # Check if langfuse server is actually reachable
  if curl -sf --connect-timeout 0.5 --max-time 1 "$CONTAINER_LANGFUSE_URL" -o /dev/null 2>/dev/null; then
    LANGFUSE_DETECTED=1
    LANGFUSE_MODE="host"

    # Try to copy host's hook file if available (may have custom logic)
    HOST_HOOK=""
    if [ -d "$HOME/.claude/hooks-host" ]; then
      for f in "$HOME/.claude/hooks-host"/langfuse*hook*.py "$HOME/.claude/hooks-host"/langfuse_hook*.py; do
        [ -f "$f" ] && HOST_HOOK="$f" && break
      done
    fi

    if [ -n "$HOST_HOOK" ]; then
      HOOK_BASENAME=$(basename "$HOST_HOOK")
      cp "$HOST_HOOK" "$HOME/.claude/hooks/$HOOK_BASENAME" 2>/dev/null && \
        chmod +x "$HOME/.claude/hooks/$HOOK_BASENAME" 2>/dev/null
      CONTAINER_HOOK_CMD="python3 $HOME/.claude/hooks/$HOOK_BASENAME"
      [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: copied host hook ($HOOK_BASENAME)"
    fi

    # Replace host hook command with container-local command
    langfuse_set_hook "$CONTAINER_HOOK_CMD" "$CONTAINER_LANGFUSE_URL"

    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: using host config, server at $CONTAINER_LANGFUSE_URL"
  else
    # Langfuse server not reachable, clean up hooks
    langfuse_remove_hooks
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: host config found but server unreachable at $CONTAINER_LANGFUSE_URL"
  fi

# Mode 2 (auto): No host config, but detect Langfuse server running on host
elif curl -sf --connect-timeout 0.3 --max-time 0.5 "http://host.docker.internal:$LANGFUSE_PORT" -o /dev/null 2>/dev/null; then
  LANGFUSE_DETECTED=1
  LANGFUSE_MODE="auto"
  LANGFUSE_URL="http://host.docker.internal:$LANGFUSE_PORT"

  langfuse_set_hook "$CONTAINER_HOOK_CMD" "$LANGFUSE_URL"

  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: auto-detected server at $LANGFUSE_URL"

else
  # No Langfuse detected: clean up any stale hooks and env vars from host settings
  langfuse_remove_hooks
  langfuse_remove_env

  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Langfuse: not configured (hooks removed from settings)"
fi

# Export for use in welcome message
export LANGFUSE_DETECTED
export LANGFUSE_MODE

# --- Start agent-browser MCP hub in background (only with --mcp flag) ---
if [ "$VIBRATOR_MCP_HUB" = "1" ] && command -v agent-browser >/dev/null 2>&1; then
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
elif [ "$VIBRATOR_VERBOSE" = "1" ]; then
  echo "Agent Browser: skipped (use --mcp to enable)"
fi

# --- Configure Playwright MCP server (stdio mode with playwright-mcp binary) ---
# Playwright MCP is installed globally during image build but needs runtime configuration
# since ~/.claude.json may be overwritten by host settings merge.
# Uses the globally installed playwright-mcp binary directly (node -> bun symlink
# resolves the #!/usr/bin/env node shebang). This avoids bunx tempdir permission issues.
if command -v playwright-mcp >/dev/null 2>&1; then
  PLAYWRIGHT_CMD=$(jq -r '.mcpServers.playwright.command // ""' "$HOME/.claude.json" 2>/dev/null)
  if [ "$PLAYWRIGHT_CMD" != "playwright-mcp" ]; then
    # Resolve Chrome/Chromium executable path for --executable-path flag.
    # The wrapper at /opt/google/chrome/chrome (created during build) exec's the
    # real Chromium binary with --no-sandbox flags. This ensures playwright-mcp
    # works in containers without unprivileged user namespaces.
    CHROME_PATH="/opt/google/chrome/chrome"
    if [ ! -x "$CHROME_PATH" ]; then
      CHROME_PATH=$(find /ms-playwright -name chrome -path '*/chrome-linux/chrome' 2>/dev/null | head -1)
    fi
    PLAYWRIGHT_ARGS='["--headless"]'
    if [ -n "$CHROME_PATH" ] && [ -x "$CHROME_PATH" ]; then
      PLAYWRIGHT_ARGS="[\"--headless\", \"--executable-path\", \"$CHROME_PATH\"]"
    fi
    jq --argjson args "$PLAYWRIGHT_ARGS" '.mcpServers.playwright = {
      type: "stdio",
      command: "playwright-mcp",
      args: $args
    }' "$HOME/.claude.json" > "$HOME/.claude.json.tmp" && \
      mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json"
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Playwright MCP: configured (stdio mode with playwright-mcp)"
  else
    [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Playwright MCP: already configured"
  fi
else
  [ "$VIBRATOR_VERBOSE" = "1" ] && echo "Playwright MCP: playwright-mcp not available, skipping"
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
