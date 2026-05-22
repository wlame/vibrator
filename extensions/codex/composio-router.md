---
name: Composio Router
kind: mcp
default: false
size_mb: 1
category: harness-specific
auth:
  env: COMPOSIO_API_KEY
runtime_needs:
  third_party_api: Composio
  outbound_net: true
install: |
  # Composio's MCP bridge is a hosted Streamable HTTP endpoint that
  # multiplexes 1000+ SaaS connectors behind a single MCP. Configure
  # with API key in an http_headers entry under config.toml.
  mkdir -p "$HOME/.codex"
  cat >> "$HOME/.codex/config.toml" <<'EOF'

  [mcp_servers.composio]
  url = "https://connect.composio.dev/mcp"
  http_headers = { "x-consumer-api-key" = "${COMPOSIO_API_KEY}" }
  EOF
source: https://composio.dev/toolkits/composio/framework/codex
host_aliases: [composio]
---

# Composio Router

Composio wraps 1000+ SaaS integrations (Asana, Bitbucket, Confluence,
HubSpot, Shopify, ClickUp, …) behind a single Streamable HTTP MCP
endpoint. Configure connections in the Composio dashboard, then call
them by name from Codex.

The trade-off vs. installing a native MCP per service:

- **Pro:** one config entry, central auth, no per-service install.
- **Con:** a third party sees every tool call's payload, and the
  surface depends on Composio's coverage of each underlying API.

## Auth

Create an API key in the
[Composio dashboard](https://app.composio.dev/developers) and set
`COMPOSIO_API_KEY` on the host. Then connect each service (OAuth or PAT)
once in Composio's UI; subsequent Codex sessions reuse those auth grants
via the API key.

## When to enable

- You want to try several SaaS integrations without baking each into the
  image.
- An integration you need doesn't have a first-party MCP yet.
- You're prototyping cross-app workflows ("create a Linear issue and
  post a Slack message and update Notion").

For stable production workflows on a single service, prefer that
service's native MCP (Linear MCP, Notion MCP, etc.) — fewer hops, less
to trust.

## Risk note

Each Composio-connected service inherits the auth scopes you granted in
Composio's UI. Treat the `COMPOSIO_API_KEY` like a master key — rotate
on suspicion of compromise.
