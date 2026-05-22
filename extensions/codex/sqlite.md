---
name: SQLite MCP
kind: mcp
default: false
size_mb: 10
category: databases
deps:
  features: [python]
install: |
  # Anthropic's official sqlite MCP server lives in the
  # modelcontextprotocol/servers monorepo and ships as a Python package
  # (mcp-server-sqlite on PyPI). uvx handles the install + venv on first
  # use. Default DB path is /tmp/scratch.db inside the container — point
  # at a real DB per-project via .codex/config.toml or
  # `codex mcp add` override.
  codex mcp add sqlite -- uvx mcp-server-sqlite --db-path /tmp/scratch.db
source: https://github.com/modelcontextprotocol/servers/tree/main/src/sqlite
host_aliases: [sqlite]
---

# SQLite MCP

Query SQLite databases via natural language. Default DB is
`/tmp/scratch.db` inside the container — use that for one-off
exploration.

For a project database, register a per-project MCP override after
starting Codex:

```bash
codex mcp add sqlite -- uvx mcp-server-sqlite --db-path=./mydb.sqlite
```

Codex will write the override into `<repo>/.codex/config.toml` (trusted
directories only).

## When to enable

Pair with workflows that need to slice findings tables, prototype a
schema before promoting it to Postgres, or do analytics-style SQL on
local files. For ephemeral DBs created by tests/CI, the default scratch
path is fine.
