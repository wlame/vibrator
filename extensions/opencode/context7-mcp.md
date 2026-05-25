---
name: Context7 MCP
description: Up-to-date library docs MCP
kind: mcp
default: true
size_mb: 5
category: documentation
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "context7" \
     --argjson entry '{"type":"local","command":["npx","-y","@upstash/context7-mcp"],"enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"context7":{"type":"local","command":["npx","-y","@upstash/context7-mcp"],"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/upstash/context7-mcp
---

# Context7 MCP

Live, up-to-date library documentation for thousands of frameworks.
Resolves a library reference (e.g., `react`, `fastapi`, `prisma`,
`tailwind`) and returns API/syntax pulled fresh from each project's
source — not the LLM's training snapshot.

This is the single highest-leverage MCP for an opinionated install:
it prevents the agent from hallucinating function signatures or
recommending APIs that were renamed/removed two years ago.

## Install notes

- Runs `npx -y @upstash/context7-mcp` per session. First launch pulls
  the package; subsequent launches hit npm's local cache.
- Free tier requires no API key. Outbound traffic to `context7.com`.
- For an even lighter install, swap to the **remote** flavor:
  `{ "type": "remote", "url": "https://mcp.context7.com/mcp" }` — same
  data, no local Node process.

## Why on by default

Mandatory-feeling for any modern development container. The token cost
is small relative to the quality improvement on library-specific code.
