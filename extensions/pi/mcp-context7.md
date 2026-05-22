---
name: Context7 MCP (via pi-mcp-adapter)
kind: mcp
default: true
size_mb: 5
category: documentation
deps:
  features: [node]
install: |
  # Context7 by Upstash — fetches current library documentation. Pi
  # routes it through pi-mcp-adapter so the proxy tool absorbs the
  # token cost (~200 tokens vs ~3k for direct registration).
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs');
  const path = require('path');
  const cfgPath = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfgPath)
    ? JSON.parse(fs.readFileSync(cfgPath, 'utf8'))
    : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.context7 = {
    command: 'npx',
    args: ['-y', '@upstash/context7-mcp']
  };
  fs.writeFileSync(cfgPath, JSON.stringify(data, null, 2));
  JS
source: https://github.com/upstash/context7
---

# Context7 MCP

[Context7](https://github.com/upstash/context7) by Upstash gives the
model **current** library documentation — i.e. it resolves your
specific library version and fetches API references on demand. Solves
the chronic "the model is using a deprecated API" problem.

## What it adds

Two tools (behind `pi-mcp-adapter`'s proxy):

- `resolve-library-id` — turns "react" into the canonical
  `facebook/react@19.0.0` id
- `query-docs` — pulls the documentation for that exact version

The model invokes these automatically when a user asks about a library,
framework, SDK, or CLI tool.

## When to use

Best for questions about:

- Library APIs (React, Next.js, Prisma, Express, Django, Spring Boot)
- Configuration (Tailwind, Vite, Webpack)
- Version migration
- CLI tool usage

Skip when refactoring, writing scripts from scratch, debugging business
logic, or general programming concepts — Context7 won't help.

## Default on

Cheap (~5 MB), high-value, no auth required — defaults to on.
