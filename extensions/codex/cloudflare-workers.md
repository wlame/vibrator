---
name: Cloudflare Workers MCP
kind: mcp
default: false
size_mb: 5
category: cloud-infrastructure
deps:
  features: [node]
auth:
  env: CLOUDFLARE_API_TOKEN
runtime_needs:
  third_party_api: Cloudflare
  outbound_net: true
install: |
  # workers-mcp is Cloudflare's package for exposing your Workers to MCP
  # clients. It ships an npx-launchable server. The API token is picked
  # up from CLOUDFLARE_API_TOKEN in the MCP launch env.
  codex mcp add cloudflare-workers --env CLOUDFLARE_API_TOKEN="$CLOUDFLARE_API_TOKEN" -- npx -y workers-mcp
source: https://github.com/cloudflare/workers-mcp
host_aliases: [cloudflare-workers, cf-workers]
---

# Cloudflare Workers MCP

Cloudflare's official MCP server for Workers. Deploy, tail logs,
manage routes, inspect KV/Durable Objects, and call your deployed
Workers' RPC methods directly from Codex.

Separate from the broader Cloudflare plugin (which covers DNS, R2,
Pages, etc.) — this entry is the Workers-focused MCP.

## Auth

Generate a scoped API token at
[dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens).
For deploys, use the "Edit Cloudflare Workers" template. For read-only
inspection, hand-craft a token limited to "Workers Scripts: Read" +
"Account: Read".

Set `CLOUDFLARE_API_TOKEN` on the host.

## Workflow examples

- "Deploy this Worker as `my-app-staging`"
- "Tail logs from production for the last 5 minutes"
- "Add a route `api.example.com/*` to the prod Worker"

## Risk note

Workers MCP can mutate live edge infrastructure. Scope tokens narrowly
and prefer staging targets in interactive sessions. For CI, use a
deploy-only token with no read access to secrets.
