---
name: Fetch
kind: mcp
default: true
size_mb: 5
category: web-browser
deps:
  features: [python]
runtime_needs:
  outbound_net: true
install: |
  # Official mcp-server-fetch is Python (PyPI), launched via uvx. Codex
  # has a built-in browse/fetch tool; this MCP gives the agent explicit
  # tool calls for chunked URL fetches with HTML->markdown conversion.
  codex mcp add fetch -- uvx mcp-server-fetch
source: https://github.com/modelcontextprotocol/servers/tree/main/src/fetch
host_aliases: [fetch]
---

# Fetch (official MCP)

Anthropic-maintained MCP server for fetching URLs and converting HTML to
clean markdown. Lets Codex pull web pages page-by-page with
`start_index` chunking for long documents.

The package is `mcp-server-fetch` on PyPI — not a Node module. uvx
handles the venv automatically so the install footprint stays small.

## When to use

Codex already has native browsing. Keep this MCP enabled when you want:

- Deterministic chunked reads (the `start_index` parameter)
- HTML-to-markdown that respects `robots.txt` (configurable)
- An explicit, auditable tool call rather than the built-in browser

## Risk note

The server can fetch local/internal IPs from inside the container by
default. If the container has access to a host network or a private
service mesh, set `--proxy-url` or restrict outbound at the container
firewall.
