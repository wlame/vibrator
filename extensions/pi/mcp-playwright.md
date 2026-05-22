---
name: Playwright MCP (via pi-mcp-adapter)
kind: mcp
default: false
size_mb: 280
category: web-browser
deps:
  features: [node, playwright]
runtime_needs:
  outbound_net: true
install: |
  # Microsoft's Playwright MCP server. Headless browser automation
  # tools (click, type, navigate, snapshot, screenshot, etc.). The
  # browser binaries are large (~280 MB) — gated to the playwright
  # feature.
  npx -y @playwright/mcp@latest --help >/dev/null
  npx playwright install --with-deps chromium
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs');
  const path = require('path');
  const cfgPath = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfgPath)
    ? JSON.parse(fs.readFileSync(cfgPath, 'utf8'))
    : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.playwright = {
    command: 'npx',
    args: ['-y', '@playwright/mcp@latest', '--isolated']
  };
  fs.writeFileSync(cfgPath, JSON.stringify(data, null, 2));
  JS
source: https://github.com/microsoft/playwright-mcp
---

# Playwright MCP

Microsoft's official Playwright MCP server. The agent gets a full
headless browser through MCP tools — navigate, click, type, snapshot,
screenshot, evaluate JS, intercept network requests.

## Why over chrome-devtools-mcp

Playwright MCP works against Chromium, Firefox, and WebKit, and uses
**accessibility-tree snapshots** as the primary "see the page" tool
instead of screenshots. Snapshots are markdown-shaped, fast, and
deterministic — much cheaper than pixel screenshots for an LLM.

Use `chrome-devtools-mcp` instead when you need DevTools-specific
features (network throttling, CPU throttling, performance traces).

## Common tools

- `browser_navigate(url)`
- `browser_snapshot()` — a11y tree
- `browser_take_screenshot([format])`
- `browser_click(selector)`
- `browser_type(selector, text)`
- `browser_wait_for(condition)`
- `browser_evaluate(js)`

## --isolated

The install configures `--isolated` so each session gets a fresh
browser profile. No cookie / localStorage bleed between turns.

## Size warning

~280 MB once chromium installs. Behind the `playwright` feature so it
only lands in profiles that actually want it.
