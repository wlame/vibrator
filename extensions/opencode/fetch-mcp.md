---
name: Fetch MCP
kind: mcp
default: true
size_mb: 3
category: web-browser
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "fetch" \
     --argjson entry '{"type":"local","command":["npx","-y","@modelcontextprotocol/server-fetch"],"enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"fetch":{"type":"local","command":["npx","-y","@modelcontextprotocol/server-fetch"],"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/modelcontextprotocol/servers/tree/main/src/fetch
---

# Fetch MCP

Official MCP server for fetching arbitrary web content and converting
it to clean markdown for the agent. Removes the HTML noise that wrecks
`curl`-based fallbacks and produces consistent, token-efficient pages.

Compared to Playwright MCP: Fetch is for **read-only static content**.
If a page needs JavaScript rendering or interaction (login, click,
fill), reach for Playwright. For docs, blog posts, GitHub READMEs,
RFCs, and changelog mining, Fetch is faster and far cheaper.

## Install notes

- Pure-Node implementation, light footprint.
- Outbound HTTPS only; no auth required.

## Why on by default

Pairs naturally with Context7: Context7 covers library reference docs,
Fetch covers everything else (blog posts, GitHub issue threads, RFCs,
release notes). Cheap to ship and frequently useful.
