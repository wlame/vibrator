---
name: Playwright MCP
kind: mcp
default: false
size_mb: 350
category: web-browser
deps:
  features: [playwright, node]
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "playwright" \
     --argjson entry '{"type":"local","command":["npx","-y","@playwright/mcp@0.0.75"],"enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"playwright":{"type":"local","command":["npx","-y","@playwright/mcp@0.0.75"],"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/microsoft/playwright-mcp
---

# Playwright MCP

Microsoft's official Playwright MCP — exposes full browser automation
to OpenCode: navigate, click, fill, screenshot, take an accessibility
tree snapshot, evaluate JavaScript, capture network requests, and run
Lighthouse audits.

The agent gets a real browser, not just `curl`. Use cases:
end-to-end test authoring, debugging a live web app, scraping
JS-rendered pages, accessibility audits, visual regression checks.

## Install notes

- Bundles Chromium on first run. The image footprint is roughly
  300-400 MB depending on platform.
- Outbound network required (the browser itself loads pages).
- Inside a vibrator container, you may want the headless flavor —
  add `"--headless"` to the args if a display server isn't available.

## Why off by default

Heavy install (~350 MB for Chromium) and most developer sessions don't
need a browser. Turn it on when web automation, scraping, or E2E test
work is expected for the workspace.
