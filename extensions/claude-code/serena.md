---
name: Serena
kind: mcp
default: true
size_mb: 100
deps:
  features: [python]
install: |
  claude mcp add serena \
    --scope user \
    -- uvx --from git+https://github.com/oraios/serena serena start-mcp-server --project-from-cwd
source: https://github.com/oraios/serena
---

# Serena

Semantic code analysis MCP server, LSP-backed. Provides symbol lookup,
references, find-implementations, rename, and diagnostic inspection across
~12 languages.

Vibrator's entrypoint optionally probes for a host-side Serena server at
`host.docker.internal:8765/mcp` first; if it's running, the container uses
the HTTP transport. Otherwise it falls back to the stdio mode declared here,
which spawns Serena per-session via `uvx`.

## Why on by default

It's the workhorse MCP for any non-trivial code editing — symbol lookup
beats grep for understanding "where is X used" in unfamiliar codebases.
Python via `uv` is cheap (already in most profiles).

## Project memories

Serena writes per-project memories to `.serena/memories/`. The directory is
gitignored by default — see CLAUDE.md / project-level rules for write
guidance.
