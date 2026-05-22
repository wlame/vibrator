---
name: Linear MCP
kind: mcp
default: false
size_mb: 0
category: project-management
auth:
  env: LINEAR_API_KEY
runtime_needs:
  third_party_api: Linear
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "linear" \
     --argjson entry '{"type":"remote","url":"https://mcp.linear.app/sse","enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"linear":{"type":"remote","url":"https://mcp.linear.app/sse","enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://linear.app/docs/mcp
---

# Linear MCP

Official Linear MCP — search/create/update issues, manage cycles,
query projects, link PRs to tickets. Lets OpenCode pick up work
straight from a triage queue or push status updates without leaving
the session.

This is a **remote** MCP — OpenCode handles the OAuth flow on first
use and caches tokens in `~/.local/share/opencode/mcp-auth.json`.

## Auth

Two paths:
1. **OAuth (recommended)** — run `opencode mcp auth linear` after
   installation; the browser flow stores tokens automatically.
2. **API key** — set `LINEAR_API_KEY` if the OAuth flow can't reach a
   browser (headless container). Some endpoints require OAuth scope
   regardless, so OAuth is preferred when feasible.

## Why off by default

Requires a Linear workspace plus user-supplied auth. Enable for
sessions that touch project management.
