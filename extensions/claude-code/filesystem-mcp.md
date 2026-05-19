---
name: Filesystem MCP
kind: mcp
default: false
size_mb: 5
host_aliases: [filesystem]
deps:
  features: [node]
install: |
  npm install -g @modelcontextprotocol/server-filesystem
  claude mcp add filesystem \
    --scope user \
    --transport stdio \
    -- mcp-server-filesystem /workspace
source: https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem
---

# Filesystem MCP

Official MCP server granting structured filesystem access to a configured
allowlist of paths.

## When to enable

Claude Code already has file access built in via its own tools — this MCP
is most useful when you want **finer-grained access control** (e.g., expose
only `/workspace/data/` to Claude, hide everything else) or when running
Claude in a workflow where the built-in file tools are restricted.

Most users do not need this entry. Default = off.
