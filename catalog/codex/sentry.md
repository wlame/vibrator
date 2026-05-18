---
name: Sentry
kind: plugin
default: false
size_mb: 1
auth:
  env: SENTRY_AUTH_TOKEN
install: |
  codex plugins install sentry
source: https://developers.openai.com/codex/plugins
---

# Sentry (Codex official)

Read error tracking and performance data from Sentry inside Codex sessions.
Particularly useful when triaging "what broke?" right after a deploy.

## Auth

Sentry uses auth tokens (per-org). Create at
[sentry.io/settings/account/api/auth-tokens](https://sentry.io/settings/account/api/auth-tokens)
with `org:read` + `project:read` + `event:read` scopes.

Set `SENTRY_AUTH_TOKEN` on the host.

## Common workflows

- "Show top 10 errors in the last hour for project foo"
- "Group these stack traces by root file"
- "Open the issue causing the highest error count"
