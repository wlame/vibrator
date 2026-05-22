---
name: Chrome DevTools MCP
kind: mcp
default: false
size_mb: 200
category: web-browser
host_aliases: [chrome-devtools]
deps:
  features: [node, playwright]
runtime_needs:
  outbound_net: true
install: |
  # Google's official chrome-devtools-mcp. Drives a live Chromium
  # instance via the DevTools Protocol — depends on the playwright
  # feature for the browser binary + system libs.
  claude mcp add chrome-devtools \
    --scope user \
    --transport stdio \
    -- npx -y chrome-devtools-mcp
source: https://github.com/ChromeDevTools/chrome-devtools-mcp
---

# Chrome DevTools MCP

Google's official MCP server for controlling a live Chrome instance via
the Chrome DevTools Protocol. Exposes performance traces, console
messages, network requests, Lighthouse audits, and accessibility
snapshots.

Where Playwright MCP is the "agent acts" surface (click, fill, navigate),
this is the "agent observes" surface — pull a performance trace, find the
slow request, read the LCP debugger output. The two compose well: run a
flow with Playwright, then inspect the page state with chrome-devtools.

## When to enable

Frontend performance work, accessibility audits, Core Web Vitals
investigation, "why is this page slow?" debugging. Not useful for
backend-only sessions.

## Why opt-in

Pulls in the playwright feature (~500 MB Chromium + system deps).
That's a lot of image weight if you don't need it.

## Pairing

Often enabled alongside `playwright-mcp`. The `chrome-devtools-mcp`
skill bundled in the plugin marketplace adds workflow recipes
(`debug-optimize-lcp`, `a11y-debugging`, `memory-leak-debugging`).
