---
name: Brave Search MCP
kind: mcp
default: false
size_mb: 8
category: web-browser
host_aliases: [brave]
deps:
  features: [node]
auth:
  env: BRAVE_API_KEY
runtime_needs:
  third_party_api: "Brave"
  outbound_net: true
install: |
  # Brave's own package supersedes the archived @modelcontextprotocol/
  # server-brave-search reference (the old npm name is no longer
  # supported). Default stdio transport, BRAVE_API_KEY taken from env.
  claude mcp add brave-search \
    --scope user \
    --transport stdio \
    -- npx -y @brave/brave-search-mcp-server
source: https://github.com/brave/brave-search-mcp-server
---

# Brave Search MCP

Web search through the Brave Search API. Covers web, local POI, image,
video, news, and an LLM-context summarization mode. The free tier gives
2000 queries/month which is more than enough for typical "look this up
while coding" usage.

## Auth

Mint a key at <https://api.search.brave.com/app/keys> and export it as
`BRAVE_API_KEY` before launching the harness. Vibrator passes the env
through to the container automatically when the variable is set.

## When to prefer over fetch

`fetch` only retrieves a known URL — Brave Search is the discovery
layer. Pair the two: search with Brave, then fetch the most promising
result.

## Naming caveat

The original `@modelcontextprotocol/server-brave-search` lives in the
archived reference repo and is no longer maintained. Brave's own
package (`@brave/brave-search-mcp-server`) is the current canonical
implementation — this extension installs that one.
