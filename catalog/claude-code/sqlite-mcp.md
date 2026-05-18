---
name: SQLite MCP
kind: mcp
default: false
size_mb: 10
host_aliases: [sqlite]
deps:
  features: [python]
install: |
  uv tool install mcp-server-sqlite
  claude mcp add sqlite \
    --scope user \
    --transport stdio \
    -- mcp-server-sqlite --db-path /tmp/scratch.db
source: https://github.com/modelcontextprotocol/servers/tree/main/src/sqlite
---

# SQLite MCP

Query SQLite databases via natural language. Default DB is `/tmp/scratch.db`
inside the container — use that for one-off exploration.

For a project database, register a per-project MCP override after starting:

```bash
claude mcp add sqlite --scope=project -- mcp-server-sqlite --db-path=./mydb.sqlite
```

## When to enable

Pair with `audit-toolkit` workflows that need to slice findings tables, or
any session that involves analytics-style SQL.
