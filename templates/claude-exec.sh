#!/bin/zsh
# Wrapper for docker exec sessions.
# Re-checks Serena availability on every entry (host server may start/stop).

SERENA_PORT="${SERENA_PORT:-8765}"
SERENA_MESSAGE=""

if curl -sf --connect-timeout 0.3 --max-time 0.5 "http://host.docker.internal:$SERENA_PORT/mcp" 2>/dev/null | grep -q "mcp-session-id\|jsonrpc" || \
   curl -sf --connect-timeout 0.3 --max-time 0.5 -I "http://host.docker.internal:$SERENA_PORT/mcp" 2>/dev/null | grep -q "mcp-session-id"; then
  if [ -f "$HOME/.claude.json" ]; then
    CURRENT_TYPE=$(jq -r '.mcpServers.serena.type // "unknown"' "$HOME/.claude.json" 2>/dev/null)
    if [ "$CURRENT_TYPE" != "http" ]; then
      jq --arg url "http://host.docker.internal:$SERENA_PORT/mcp" \
        '.mcpServers.serena = {type: "http", url: $url}' \
        "$HOME/.claude.json" > "$HOME/.claude.json.tmp" 2>/dev/null && \
        mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json" 2>/dev/null
      SERENA_MESSAGE="Serena: Connected to host server at host.docker.internal:$SERENA_PORT"
    else
      SERENA_MESSAGE="Serena: Using host server at host.docker.internal:$SERENA_PORT"
    fi
  fi
else
  if [ -f "$HOME/.claude.json" ]; then
    CURRENT_TYPE=$(jq -r '.mcpServers.serena.type // "unknown"' "$HOME/.claude.json" 2>/dev/null)
    if [ "$CURRENT_TYPE" = "http" ]; then
      jq '.mcpServers.serena = {
        type: "stdio",
        command: "uvx",
        args: ["--from", "git+https://github.com/oraios/serena", "serena", "start-mcp-server", "--project-from-cwd"]
      }' "$HOME/.claude.json" > "$HOME/.claude.json.tmp" 2>/dev/null && \
        mv -f "$HOME/.claude.json.tmp" "$HOME/.claude.json" 2>/dev/null
      SERENA_MESSAGE="Serena: Host server unavailable, using built-in stdio mode"
    else
      SERENA_MESSAGE="Serena: Using built-in stdio mode"
    fi
  fi
fi

# Change to workspace
if [[ -n "$WORKSPACE_PATH" && -d "$WORKSPACE_PATH" ]]; then
  cd "$WORKSPACE_PATH"
else
  cd ~
fi

# Execute command or start interactive shell
if [[ $# -gt 0 ]]; then
  [[ "$VIBRATOR_VERBOSE" == "1" && -n "$SERENA_MESSAGE" ]] && echo "$SERENA_MESSAGE" >&2
  [[ -f ~/.zshenv ]] && source ~/.zshenv
  [[ -f ~/.zshrc ]] && source ~/.zshrc
  exec "$@"
else
  if [[ -n "$SERENA_MESSAGE" ]]; then
    echo -e "\033[1;36m$SERENA_MESSAGE\033[0m"
    echo ""
  fi
  exec /bin/zsh
fi
