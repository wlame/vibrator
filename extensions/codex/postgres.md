---
name: PostgreSQL MCP (read-only)
kind: mcp
default: false
size_mb: 15
category: databases
deps:
  features: [node, postgres-client]
auth:
  env: POSTGRES_CONNECTION_STRING
runtime_needs:
  outbound_net: true
install: |
  # Anthropic's official Postgres MCP. Connection string is passed as a
  # CLI argument; we read it from the env var at runtime via a wrapper.
  # In a container, prefer host.docker.internal or a docker-network
  # hostname over localhost.
  codex mcp add postgres -- npx -y @modelcontextprotocol/server-postgres "$POSTGRES_CONNECTION_STRING"
source: https://github.com/modelcontextprotocol/servers/tree/main/src/postgres
host_aliases: [postgres]
---

# PostgreSQL MCP (Anthropic, read-only)

Anthropic's official Postgres MCP server. Deliberately limited to
read-only queries and schema inspection — that constraint is the
feature. Point at a production database without worrying about
accidental writes.

Schema inspection lets Codex understand your data model before composing
queries, which improves query accuracy materially.

## Auth

Set `POSTGRES_CONNECTION_STRING` on the host before running `vibrate`.
Use a `postgres://readonly:...@host/db` form against a least-privilege
read role; never embed superuser credentials.

In Docker contexts, prefer `host.docker.internal` (macOS/Windows) or an
explicit docker-network hostname over `localhost`, which inside the
container resolves to the container itself.

## Security advisory (2026)

A SQL injection issue was filed against this server in early 2026. Even
in read-only mode, treat it as defense-in-depth: scope DB credentials,
audit query patterns, and avoid pointing it at sensitive prod tables.
