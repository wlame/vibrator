---
name: pi-mcp-adapter
kind: plugin
default: true
size_mb: 4
category: harness-specific
deps:
  features: [node]
install: |
  # pi-mcp-adapter by nicobailon — the de facto MCP bridge for Pi.
  # Pi ships with NO built-in MCP support; this adapter solves it via a
  # single proxy tool (~200 tokens) with lazy server loading. Imports
  # configs from cursor / claude-code / claude-desktop / vscode /
  # windsurf / codex automatically.
  pi install npm:pi-mcp-adapter
source: https://github.com/nicobailon/pi-mcp-adapter
---

# pi-mcp-adapter

**The MCP layer for Pi.** Pi (`@earendil-works/pi-coding-agent`) ships
deliberately minimal — no Model Context Protocol support out of the box.
`pi-mcp-adapter` by `nicobailon` (728+★) is the community's accepted
answer, and it is the foundation every other MCP-based entry in this
catalogue depends on.

## Why it matters

The naive approach — exposing every MCP server's tools directly — eats
10k+ tokens of context per server. `pi-mcp-adapter` instead registers a
**single proxy tool** (~200 tokens) and lazily routes calls to the right
upstream server. You get the entire MCP ecosystem without the token
tax.

## What it imports

On first run, the adapter looks for existing MCP configs at:

- Cursor (`~/.cursor/mcp.json`)
- Claude Code (`~/.claude.json`, project-level too)
- Claude Desktop
- VS Code (`settings.json`)
- Windsurf
- Codex (`~/.codex/config.toml`)

Whichever it finds, it ingests, so users with an existing MCP setup get
zero-config Pi integration.

## Config

Edit `~/.pi/agent/mcp.json` (or use the wizard) to add servers:

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "directTools": ["search_repositories", "get_file_contents"]
    }
  }
}
```

`directTools` registers a few high-value tools directly (full token
cost) while everything else stays behind the proxy.

## Default

On by default — without this, the MCP entries in vibrator's catalogue
cannot install. Disable only if you're going entirely MCP-free.
