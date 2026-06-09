---
name: "Profile: Indie hacker"
kind: plugin
default: false
size_mb: 180
category: harness-specific
deps:
  features: [node, python]
auth:
  env: STRIPE_API_KEY
install: |
  # Suggested archetype — solo founder shipping fast. Stripe + Vercel +
  # Postmark + GitHub + analytics + cost dashboard + UI polish.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. Business MCPs — Stripe / Vercel / Postmark / GitHub
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.stripe = {
    command: 'npx', args: ['-y', '@stripe/mcp-server@latest'],
    env: { STRIPE_API_KEY: '${STRIPE_API_KEY}' }
  };
  data.mcpServers.vercel = {
    command: 'npx', args: ['-y', '@vercel/mcp@latest'],
    env: { VERCEL_TOKEN: '${VERCEL_TOKEN}' }
  };
  data.mcpServers.postmark = {
    command: 'npx', args: ['-y', 'postmark-mcp@latest'],
    env: { POSTMARK_SERVER_TOKEN: '${POSTMARK_SERVER_TOKEN}' }
  };
  data.mcpServers.github = {
    command: 'npx', args: ['-y', '@modelcontextprotocol/server-github'],
    env: { GITHUB_PERSONAL_ACCESS_TOKEN: '${GITHUB_PERSONAL_ACCESS_TOKEN}' }
  };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 3. task-factory — queue-first orchestrator with web UI
  npm install -g task-factory || pi install git:github.com/patleeman/task-factory

  # 4. Cost dashboard — Python web UI at port 8753
  pip install --user pi-cost-dashboard || \
    pip install --user git+https://github.com/mrexodia/pi-cost-dashboard@74c2d52b5614c737b0b8bad8396d29f4369f5d66

  # 5. UI polish + whimsy
  pi install git:github.com/mitsuhiko/agent-stuff  # frontend-design, commit, mermaid, summarize, whimsical
  pi install npm:pi-screenshots-picker
  pi install npm:pi-powerline-footer

  # 6. Theme
  pi install npm:@zenobius/pi-rose-pine

source: https://github.com/patleeman/task-factory
---

# Profile: Indie hacker

Pre-curated Pi stack for solo founders who need to ship fast across
the entire stack — landing page, payment, transactional email,
deploys, analytics. Optimised for breadth, not depth.

## What's installed

| Layer            | Package                                              |
|------------------|------------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                     |
| Payments (MCP)   | `@stripe/mcp-server`                                 |
| Deploys (MCP)    | `@vercel/mcp`                                        |
| Email (MCP)      | `postmark-mcp`                                       |
| GitHub (MCP)     | `@modelcontextprotocol/server-github`                |
| Work queue       | `task-factory` (web UI on a chosen port)             |
| Cost dashboard   | `pi-cost-dashboard` (port 8753)                      |
| Skills           | `agent-stuff` (frontend-design / commit / mermaid /  |
|                  |   summarize / whimsical)                             |
| Screenshots      | `pi-screenshots-picker`                              |
| Footer           | `pi-powerline-footer`                                |
| Theme            | `@zenobius/pi-rose-pine`                             |

## Required env vars

- `STRIPE_API_KEY` — for the Stripe MCP
- `VERCEL_TOKEN` — for the Vercel MCP
- `POSTMARK_SERVER_TOKEN` — for the Postmark MCP
- `GITHUB_PERSONAL_ACCESS_TOKEN` — for the GitHub MCP

Vibrator's wizard prompts for each.

## Workflow it supports

- **Drop a Figma screenshot** (`Ctrl+Shift+S`) → `frontend-design`
  skill drafts the landing page
- **Stripe MCP** to create products, prices, payment links
- **Postmark MCP** to wire welcome emails / drip campaigns
- **Vercel MCP** to inspect deployments + env vars
- **task-factory** to queue 5–10 small jobs and tackle them between
  meetings — Telegram / Discord notifications when ready for review
- **pi-cost-dashboard** at `http://localhost:8753` to track AI spend
  across all your indie projects

## Whimsy

`agent-stuff/whimsical.ts` adds themed loading messages — small thing
but it makes a long shipping day feel less lonely.

## Caveat

Stripe / Vercel / Postmark MCPs are evolving — package names may
differ. Adjust `~/.pi/agent/mcp.json` to your preferred implementation.
