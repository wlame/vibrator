---
name: context7
kind: mcp
default: true
size_mb: 1
install: |
  claude mcp add context7 \
    --scope user \
    --transport http \
    https://mcp.context7.com/mcp
source: https://github.com/upstash/context7
---

# context7

Up-to-date, version-specific library documentation injected directly into
Claude Code sessions. Built by Upstash. Two core tools:

- `resolve-library-id` — look up a library by name
- `query-docs` — fetch specific documentation for the resolved ID

Useful instead of relying on training-data snapshots for React/Next.js/Prisma
APIs, which can lag months behind upstream.

## Why it's enabled by default

Trivial install (a single MCP registration against a public HTTP endpoint —
no API key, no server, no container deps). Almost everyone benefits, no
downside to having it registered. Disable with `--no context7` if you'd
rather not have an extra HTTP fetch on session start.
