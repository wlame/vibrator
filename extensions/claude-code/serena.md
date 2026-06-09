---
name: Serena
description: LSP-backed code MCP — symbols, refs, find-implementations, rename across ~12 languages
kind: mcp
default: true
size_mb: 100
deps:
  features: [python]
install: |
  claude mcp add serena \
    --scope user \
    -- uvx --from git+https://github.com/oraios/serena@1d020b96069435310613d07211ced178e1fdaf78 serena start-mcp-server --project-from-cwd
source: https://github.com/oraios/serena
---

# Serena

Semantic code analysis MCP server, LSP-backed. Provides symbol lookup,
references, find-implementations, rename, and diagnostic inspection across
~12 languages.

## Hosting mode

How Serena is served is a per-workspace choice, set in the wizard (the
"Serena code-intelligence server" step) and stored in `.vb` under
`[integrations] serena = "<mode>"`:

- **`auto`** (default) — on each container entry the `claude-exec` wrapper
  probes the host server at `host.docker.internal:8765/mcp`. If it answers,
  the container uses the HTTP transport; otherwise it falls back to the
  container-local stdio server declared here (spawned per-session via
  `uvx`), printing a one-line warning.
- **`host`** — require the shared host server; if it's unreachable the
  wrapper warns loudly and does **not** fall back, so the misconfiguration
  is visible rather than silently masked.
- **`local`** — always run Serena inside the container; never probe the host.
- **`off`** — don't wire Serena at all.

The probe + transport switch run on every entry (including `docker exec`
re-entries), so the choice is honored without rebuilding.

To avoid a redundant, host-blind duplicate, the entrypoint drops any Claude
Code **plugin** whose id collides with an integration-managed MCP (e.g.
`serena@claude-plugins-official`) — vibrator's integration wiring is the
single source of truth for Serena.

## Why on by default

It's the workhorse MCP for any non-trivial code editing — symbol lookup
beats grep for understanding "where is X used" in unfamiliar codebases.
Python via `uv` is cheap (already in most profiles).

## Project memories

Serena writes per-project memories to `.serena/memories/`. The directory is
gitignored by default — see CLAUDE.md / project-level rules for write
guidance.
