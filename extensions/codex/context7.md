---
name: context7
description: Up-to-date library docs MCP — fetches API references on demand
kind: mcp
default: true
size_mb: 1
category: documentation
deps:
  features: [node]
runtime_needs:
  outbound_net: true
install: |
  # Stdio transport — the official Codex recommendation. npm package is
  # pulled lazily by npx on first session, so the bake step here is
  # essentially a no-op but registers the config.toml entry.
  codex mcp add context7 -- npx -y @upstash/context7-mcp
source: https://github.com/upstash/context7
host_aliases: [context7]
---

# context7

Up-to-date, version-pinned library documentation injected into Codex
sessions on demand. Built by Upstash. Two core tools:

- `resolve-library-id` — look up a library by name
- `query-docs` — fetch specific documentation for the resolved ID

Useful instead of relying on Codex's training-data snapshot for
React/Next.js/Prisma/Django APIs, which can lag months behind upstream.

## Why on by default

It is the single most-recommended Codex MCP in every "best of" list,
trivial to install (just a stdio entry), and has no auth requirement.
Disable with `--no=context7` if you want one less child process per
session.

## Auth (optional)

For higher rate limits, Upstash issues API keys. Set `CONTEXT7_API_KEY`
on the host and Codex picks it up at MCP launch. Not required for casual
use.
