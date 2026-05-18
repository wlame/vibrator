---
name: Cloudflare
kind: plugin
default: false
size_mb: 1
auth:
  env: CLOUDFLARE_API_TOKEN
install: |
  codex plugins install cloudflare
source: https://developers.openai.com/codex/plugins
---

# Cloudflare (Codex official)

Manage Cloudflare Workers, Pages, DNS, and infrastructure from Codex.

## Auth

Generate a scoped API token at
[dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens).
Use the "Edit Cloudflare Workers" template for Worker deploys, or hand-craft
a token with minimal scope for read-only DNS access.

Set `CLOUDFLARE_API_TOKEN` on the host.

## Common workflows

- "Deploy this Worker as `my-app-staging`"
- "List DNS records for example.com"
- "Show R2 buckets and their sizes"

## Risk note

This plugin can mutate production infrastructure. Scope tokens narrowly,
and prefer the `--non-interactive` mode against staging environments only.
