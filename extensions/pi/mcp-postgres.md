---
name: Postgres MCP (via pi-mcp-adapter)
kind: mcp
default: false
size_mb: 6
category: databases
deps:
  features: [node]
auth:
  env: POSTGRES_CONNECTION_STRING
runtime_needs:
  outbound_net: true
install: |
  # Official @modelcontextprotocol/server-postgres. The model gets a
  # read-only SQL surface, schema introspection, and a single
  # parameterized query tool.
  if [ -z "${POSTGRES_CONNECTION_STRING:-}" ]; then
    echo "WARN: POSTGRES_CONNECTION_STRING not set. Server will install but cannot connect until you set it."
  fi
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs');
  const path = require('path');
  const cfgPath = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfgPath)
    ? JSON.parse(fs.readFileSync(cfgPath, 'utf8'))
    : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.postgres = {
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-postgres', '${POSTGRES_CONNECTION_STRING}']
  };
  fs.writeFileSync(cfgPath, JSON.stringify(data, null, 2));
  JS
source: https://github.com/modelcontextprotocol/servers/tree/main/src/postgres
---

# Postgres MCP

Official MCP server for read-only Postgres access. The model gets typed
schema introspection plus a single parameterised query tool — no shell
escape into `psql`, no risk of running an `ALTER` it shouldn't.

## What it does

- `query(sql, [params])` — read-only query (server enforces read-only)
- Schema resources for inspecting tables, columns, indexes, etc.

## Connection

Set `POSTGRES_CONNECTION_STRING` to a libpq-style URL:

```
postgresql://user:password@host:5432/dbname
```

The server connects on demand; idle connections close.

## Why read-only

Letting the model run arbitrary `UPDATE`/`DELETE`/`DROP` is a footgun
nobody asked for. If you genuinely need write access, the recommended
pattern is to expose write paths via a custom MCP server that knows
your invariants (or a CLI wrapper) rather than handing the model raw
write SQL.

## Pair with

- `mcp-serena` for schema-aware code refactors (column rename touches
  both code and DB)
- `mcp-github` for opening migration PRs from the same session

Default off — needs a real connection string, which is environment-
specific.
