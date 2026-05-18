---
name: Sequential Thinking
kind: mcp
default: true
size_mb: 5
deps:
  features: [node]
install: |
  npm install -g @modelcontextprotocol/server-sequential-thinking
  claude mcp add sequential-thinking \
    --scope user \
    --transport stdio \
    -- mcp-server-sequential-thinking
source: https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking
---

# Sequential Thinking

Official reference MCP server for chain-of-thought decomposition. Exposes
a single `sequentialthinking` tool that lets the agent break a complex
problem into ordered steps before executing.

Useful for: deep-dive analysis, multi-step refactors, security audits where
each finding needs justification.

## Why on by default

Trivial install (one npm package), no auth, no host services. Net-positive
for almost every Claude Code session.
