---
name: Playwright MCP
kind: mcp
default: false
size_mb: 500
category: web-browser
deps:
  features: [playwright]
runtime_needs:
  outbound_net: true
install: |
  # Microsoft's official Playwright MCP. The npm package
  # @playwright/mcp ships the server; the playwright feature provisions
  # Chromium and its system deps in the base image.
  codex mcp add playwright -- npx -y @playwright/mcp@0.0.75
source: https://github.com/microsoft/playwright-mcp
host_aliases: [playwright]
---

# Playwright MCP

Browser automation via Microsoft's official Playwright MCP. Lets Codex
drive a real browser using structured accessibility trees — cheaper and
more reliable than screenshot-based UI testing.

Typical use: "go to localhost:3000, log in as `testuser`, and verify the
dashboard loads". Catches "looks fine in the DOM but the UI is broken"
cases that pure code review can't surface.

## Why opt-in

Heavy install (~500 MB for Chromium + its system dependencies, on top
of the `playwright` feature). Only enable for frontend / E2E work.

## Container caveats

Codex runs Playwright headless inside the container. Pass `--isolated`
or a profile-dir flag to keep cookies and session state out of any
host-mounted paths. For sites behind corporate auth, mint short-lived
test creds rather than sharing real cookies into the container.

## Naming caveat

`@modelcontextprotocol/server-playwright` was an earlier package name
and is deprecated. Always use `@playwright/mcp` (this extension installs
the correct one).
