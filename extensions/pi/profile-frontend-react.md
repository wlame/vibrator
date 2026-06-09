---
name: "Profile: Frontend (React/TypeScript)"
kind: plugin
default: false
size_mb: 320
category: harness-specific
deps:
  features: [node, playwright]
install: |
  # Suggested archetype — vibrator's pre-curated Pi setup for React /
  # Vite / TypeScript frontend engineers practising TDD.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. Browser automation via MCP
  npx playwright install --with-deps chromium
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.playwright = { command: 'npx', args: ['-y', '@playwright/mcp@0.0.75', '--isolated'] };
  data.mcpServers['chrome-devtools'] = { command: 'npx', args: ['-y', 'chrome-devtools-mcp@1.2.0'] };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 3. LSP + permission gates + checkpoint
  pi install npm:pi-hooks
  npm install -g typescript typescript-language-server @biomejs/biome

  # 4. Skills — Mario's frontend stack + Armin's curated kit
  pi install git:github.com/badlogic/pi-skills
  pi install git:github.com/mitsuhiko/agent-stuff

  # 5. TDD discipline skills (fgladisch port of Anthropic Superpowers)
  pi install git:github.com/fgladisch/pi-skills

  # 6. UI polish — powerline footer + screenshots picker
  pi install npm:pi-powerline-footer
  pi install npm:pi-screenshots-picker

  # 7. Theme
  pi install npm:@zenobius/pi-rose-pine

  # 8. Configure powerline preset
  pi config set powerline.preset full || true

source: https://github.com/badlogic/pi-skills
---

# Profile: Frontend (React/TypeScript)

Pre-curated Pi stack for React / Vite / TypeScript engineers who do
TDD. The largest-surface profile in the catalogue — gives you a
batteries-included Pi that looks closer to a Claude Code or Cursor
experience.

## What's installed

| Layer            | Package                                          |
|------------------|--------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                 |
| Browser (MCP)    | `@playwright/mcp` + `chrome-devtools-mcp`        |
| LSP + hooks      | `pi-hooks` (lsp/checkpoint/permission/...)       |
| TS toolchain     | `typescript-language-server`, `@biomejs/biome`   |
| Skills           | `badlogic/pi-skills` (full pack)                 |
| Skills           | `mitsuhiko/agent-stuff` (frontend-design, etc.)  |
| TDD discipline   | `fgladisch/pi-skills` (TDD, verification, etc.)  |
| Footer           | `pi-powerline-footer` (preset=`full`)            |
| Screenshots      | `pi-screenshots-picker`                          |
| Theme            | `@zenobius/pi-rose-pine`                         |

## Workflow it supports

1. Open vibrator, land in the project
2. `Ctrl+Shift+S` to attach a Figma mock screenshot
3. Agent invokes `frontend-design` skill to plan
4. `pi-hooks/lsp` runs Biome + tsserver after every edit
5. Playwright MCP drives the dev server for end-to-end smoke
6. `fgladisch/pi-skills/test-driven-development` keeps the agent
   honest about writing tests before code
7. `pi-hooks/checkpoint` per turn so you can rewind any wrong
   refactor with `/tree`

## Size

~320 MB once Chromium is in. Drop the `playwright` feature if you only
want chrome-devtools-mcp.
