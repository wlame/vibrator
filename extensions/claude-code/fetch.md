---
name: Fetch MCP
kind: mcp
default: true
size_mb: 5
category: web-browser
host_aliases: [fetch]
deps:
  features: [python]
runtime_needs:
  outbound_net: true
install: |
  # Despite the "@modelcontextprotocol/server-fetch" naming common in
  # blog posts, the canonical fetch reference server is published to
  # PyPI as `mcp-server-fetch`, not npm. uvx runs it isolated.
  claude mcp add fetch \
    --scope user \
    --transport stdio \
    -- uvx mcp-server-fetch
source: https://github.com/modelcontextprotocol/servers/tree/main/src/fetch
---

# Fetch MCP

Anthropic's official reference MCP server for fetching arbitrary URLs
and converting HTML to clean markdown for the model. One tool, one job
— grab a URL, return readable text.

## Why on by default

No API key, no third-party service, lightweight install, and broadly
useful. Claude Code's built-in WebFetch is similar but goes through a
small summarization model first — this MCP returns the raw markdown,
which is often what you actually want when reading docs or articles.

## What it doesn't do

No JavaScript rendering, no crawling, no search. If you need any of
those, layer Brave Search MCP (discovery), Firecrawl (deep scrape with
JS), or Playwright (full browser automation).

## Naming caveat

Many tutorials reference `@modelcontextprotocol/server-fetch` as an npm
package — that name is not published. The canonical implementation is
the Python `mcp-server-fetch` on PyPI, which is what this extension
installs via `uvx`.
