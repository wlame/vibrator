---
name: Sentry MCP
kind: mcp
default: false
size_mb: 10
category: observability
host_aliases: [sentry]
deps:
  features: [node]
auth:
  env: SENTRY_AUTH_TOKEN
runtime_needs:
  third_party_api: "Sentry"
  outbound_net: true
install: |
  # Official getsentry/sentry-mcp stdio package. The --access-token
  # flag accepts the token as an arg, but we pass it via env so the
  # value never lands in the shell history. The MCP server reads
  # SENTRY_ACCESS_TOKEN; we re-export from SENTRY_AUTH_TOKEN since
  # that's the conventional Sentry env var name developers already
  # have set.
  claude mcp add sentry \
    --scope user \
    --transport stdio \
    -- sh -c 'SENTRY_ACCESS_TOKEN="$SENTRY_AUTH_TOKEN" npx -y @sentry/mcp-server@0.36.0'
source: https://github.com/getsentry/sentry-mcp
---

# Sentry MCP

Error tracking integration. Pull stack traces, search issues by
fingerprint, inspect event details, browse releases, query performance
data. Useful for "this just paged, what's the root cause?" debugging
inside Claude Code.

## Auth

Mint a Sentry user auth token at
<https://sentry.io/settings/account/api/auth-tokens/> with at least
`event:read` and `project:read` scopes. Export it as
`SENTRY_AUTH_TOKEN`.

For self-hosted Sentry, also set `--host=sentry.example.com` in the
install command (and `--insecure-http` if your install isn't on HTTPS).

## Why opt-in

Requires a Sentry account + manual token minting. Not everyone uses
Sentry, and the alternative `sentry@claude-plugins-official` plugin in
the Anthropic marketplace handles OAuth interactively — if you prefer
OAuth-on-first-use over env-based auth, use that instead.

## Workflow

The model gets natural-language access to the error feed: "show me the
top 5 issues by frequency from the last 24 hours", "find traces for
the checkout flow that took longer than 2 seconds", "what's the
breadcrumb trail on event `<id>`?". Pairs well with `feature-dev` for
"investigate this Sentry issue and propose a fix" loops.
