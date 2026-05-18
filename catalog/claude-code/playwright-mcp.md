---
name: Playwright MCP
kind: mcp
default: false
size_mb: 500
deps:
  features: [playwright]
install: |
  # @playwright/mcp is Microsoft's official Playwright MCP server. The old
  # @modelcontextprotocol/server-playwright package is deprecated — do NOT
  # use that one.
  npm install -g @playwright/mcp
  claude mcp add playwright \
    --scope user \
    --transport stdio \
    -- npx @playwright/mcp
source: https://github.com/microsoft/playwright-mcp
---

# Playwright MCP

Browser automation via Microsoft's official Playwright MCP server. Lets the
agent interact with web pages using structured accessibility trees (cheaper
and more reliable than screenshot-based approaches).

Typical use: "go to localhost:3000, log in as `testuser`, and verify the
dashboard loads". Great for catching "looks fine in the DOM but the UI is
broken" cases that pure code review can't surface.

## Why opt-in

Heavy install (~500 MB for Chromium + its system dependencies). Only enable
if you actually do frontend / web testing work.

## Naming caveat

`@modelcontextprotocol/server-playwright` was the original package name and
is now deprecated. Always use `@playwright/mcp` (this catalog entry installs
the correct one).
