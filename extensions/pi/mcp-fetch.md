---
name: Fetch MCP (via pi-mcp-adapter)
kind: mcp
default: true
size_mb: 4
category: web-browser
deps:
  features: [node]
install: |
  # Fetch MCP — official @modelcontextprotocol/server-fetch. Lets the
  # model GET URLs and read their content (HTML stripped to markdown).
  # Routed via pi-mcp-adapter.
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs');
  const path = require('path');
  const cfgPath = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfgPath)
    ? JSON.parse(fs.readFileSync(cfgPath, 'utf8'))
    : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.fetch = {
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-fetch']
  };
  fs.writeFileSync(cfgPath, JSON.stringify(data, null, 2));
  JS
source: https://github.com/modelcontextprotocol/servers/tree/main/src/fetch
---

# Fetch MCP

Official MCP fetch server — a simple, well-trusted way for the agent to
pull a URL and read it. Strips HTML to markdown so the result is
token-efficient.

## Why default on

Pi has no built-in HTTP tool. Without something here, the model resorts
to `curl` via bash and then has to parse raw HTML. Fetch MCP avoids
both — clean markdown output, no shell escape headaches.

## Tools provided

- `fetch(url, [max_length])` — GET the URL, return readable content

## Privacy considerations

The MCP server runs locally in the container; nothing about your code
leaves the box unless the model decides to fetch an external URL.
There's no proxy or third-party intermediary.

## Default on

~4 MB, no auth, ubiquitous use case.
