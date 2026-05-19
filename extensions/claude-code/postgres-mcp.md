---
name: PostgreSQL MCP (read-only)
kind: mcp
default: false
size_mb: 15
host_aliases: [postgres]
deps:
  features: [node, postgres-client]
auth:
  env: DATABASE_URL
install: |
  npm install -g @modelcontextprotocol/server-postgres
  claude mcp add postgres \
    --scope user \
    --transport stdio \
    -- mcp-server-postgres
source: https://github.com/modelcontextprotocol/servers/tree/main/src/postgres
---

# PostgreSQL MCP (Anthropic, read-only)

Anthropic's official Postgres MCP server. Deliberately limited to read-only
queries and schema inspection — that constraint is the feature. Point at a
production database without worrying about accidental writes.

Schema inspection lets Claude understand your data model before composing
queries, which improves query accuracy materially.

## Security advisory (2026)

A SQL injection issue was filed against this server in early 2026. Even in
read-only mode, treat it as a defense-in-depth tool — point it at a least-
privilege read role on the DB, never the superuser.

Set `DATABASE_URL` to a `postgres://readonly:...@host/db` form.

For richer features (index tuning, query optimization) consider
`postgres-mcp-pro` (CrystalDBA) — a separate extension.
