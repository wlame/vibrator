---
name: Postgres MCP
kind: mcp
default: false
size_mb: 8
category: databases
deps:
  features: [node]
auth:
  env: POSTGRES_CONNECTION_STRING
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "postgres" \
     --arg conn "${POSTGRES_CONNECTION_STRING:-postgresql://user:pass@host:5432/db}" \
     --argjson entry '{"type":"local","command":["npx","-y","@modelcontextprotocol/server-postgres"],"enabled":true}' \
     '.mcp[$name] = ($entry | .command += [$conn])' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"postgres":{"type":"local","command":["npx","-y","@modelcontextprotocol/server-postgres","'"${POSTGRES_CONNECTION_STRING:-postgresql://user:pass@host:5432/db}"'"],"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/modelcontextprotocol/servers/tree/main/src/postgres
---

# Postgres MCP

Official Model Context Protocol server for **read-only** PostgreSQL
access. Lets OpenCode inspect schema, run SELECTs, explore tables, and
draft migrations — without giving the agent direct write access to your
database.

Works with any reachable Postgres URL: local dev databases, Supabase,
Neon, RDS, Cloud SQL, self-hosted.

## Auth

The server takes the connection string as a positional argument. We
pull it from `POSTGRES_CONNECTION_STRING` so credentials never sit in
the config file directly.

```
POSTGRES_CONNECTION_STRING=postgresql://user:pass@host:5432/db
```

For the agent to perform writes you'd need a different server (e.g.,
`@anatine/mcp-postgres` or a custom wrapper); we keep the official
read-only server here for safety.

## Why off by default

Most workspaces won't have a Postgres URL on hand. Enable per project
once the connection string is in the workspace's env file.
