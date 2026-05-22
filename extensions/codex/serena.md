---
name: Serena
kind: mcp
default: true
size_mb: 100
category: code-intelligence
deps:
  features: [python]
install: |
  # Codex MCP add — writes the [mcp_servers.serena] table to ~/.codex/config.toml.
  # uvx fetches Serena from git on first run; the binary then runs as a stdio
  # subprocess per Codex session.
  codex mcp add serena -- uvx --from git+https://github.com/oraios/serena serena start-mcp-server --project-from-cwd
source: https://github.com/oraios/serena
host_aliases: [serena]
---

# Serena

Semantic code analysis MCP server, LSP-backed. Provides symbol lookup,
references, find-implementations, rename, and diagnostic inspection
across ~20 languages.

Serena has an explicit `--context codex` flag for Codex-specific tweaks
(prompt tone, tool surface), but the `--project-from-cwd` mode used here
is robust to both. Codex's `/mcp` panel may briefly show "failed" while
`uvx` downloads on first run — known cosmetic bug, tools work once the
subprocess is up.

## Why on by default

Workhorse MCP for any non-trivial code editing — symbol lookup beats
grep for "where is X used?" in unfamiliar codebases. Python via `uv` is
already on the image for most profiles, so the install footprint is the
Serena git clone (~100 MB including indexes).

## Project memories

Serena writes per-project memories to `.serena/memories/` inside the
workspace. The directory is gitignored by default. For large repos run
`serena project index` once to prime the symbol cache before relying on
find-references / rename.
