---
name: Serena MCP
kind: mcp
default: true
size_mb: 120
category: code-intelligence
deps:
  features: [python]
runtime_needs:
  local_only: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "serena" \
     --argjson entry '{"type":"local","command":["uvx","--from","git+https://github.com/oraios/serena","serena","start-mcp-server","--project-from-cwd"],"enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"serena":{"type":"local","command":["uvx","--from","git+https://github.com/oraios/serena","serena","start-mcp-server","--project-from-cwd"],"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/oraios/serena
---

# Serena MCP

LSP-backed semantic code navigation for OpenCode. Adds symbol-aware
tools the agent can call directly: `find_symbol`, `find_references`,
`replace_symbol_body`, `get_diagnostics`, and a per-project memory
store under `.serena/memories/`.

OpenCode already ships with ~35 built-in LSP servers for the common
languages, plus `grep`/`glob`/`read` for navigation. Serena layers
**symbol-level edits** on top — the agent picks the right LSP for the
file, jumps to definitions/references, and rewrites by symbol instead
of by line range. Far more reliable on large codebases than diff-based
edits.

## Install notes

- Uses `uvx` (from the `uv` Python package manager) to fetch and run
  Serena from its git source on demand. First invocation downloads and
  caches everything under `~/.cache/uv/`.
- `--project-from-cwd` makes Serena auto-activate whichever project
  you launched `opencode` from. Drop the flag if you'd rather call
  `mcp__serena__activate_project` manually.
- Heavyweight (~120 MB resident once the LSPs spin up). Disable on
  small workspaces if memory is tight.

## Why on by default

Almost every OpenCode session benefits from symbol-aware editing, and
the Serena memory store doubles as a per-project notebook the agent
can carry across sessions.
