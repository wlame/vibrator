---
name: Slack MCP
kind: mcp
default: false
size_mb: 10
category: communication
host_aliases: [slack]
deps:
  features: [node]
auth:
  env: SLACK_BOT_TOKEN
runtime_needs:
  third_party_api: "Slack"
  outbound_net: true
install: |
  # The original @modelcontextprotocol/server-slack lives in the
  # archived reference repo (servers-archived) as of May 2025. The
  # community fork korotovsky/slack-mcp-server is actively maintained,
  # adds search + DMs + user groups, and keeps write operations off
  # by default — safer for production workspaces.
  claude mcp add slack \
    --scope user \
    --transport stdio \
    -- npx -y slack-mcp-server@latest
source: https://github.com/korotovsky/slack-mcp-server
---

# Slack MCP

Slack workspace integration. Search messages, list channels and
threads, read history, look up users, optionally post replies. Useful
for "what did we decide about X last week?" style context retrieval
from team conversations.

## Auth

Needs a Slack bot token in `SLACK_BOT_TOKEN`. Create a Slack app at
<https://api.slack.com/apps>, install it to your workspace, and grant
the scopes the server's README recommends (`channels:history`,
`channels:read`, `users:read`, `chat:write` if you want posting, etc.).
Some workspaces require workspace-admin approval — talk to your Slack
admin first.

## Which fork

The original Anthropic implementation was archived in May 2025. We
default to **korotovsky/slack-mcp-server** because:

- Actively maintained, 1.4k+ stars
- 15 tools vs. the archived 8
- Write operations off by default — defense in depth against accidental
  channel-wide messages
- Search-by-keyword across the workspace

If your team has standardized on a different fork (e.g. Anthropic's
official `slack@claude-plugins-official` OAuth-based plugin), install
that one instead from the plugin marketplace.

## Privacy considerations

Slack workspace content is sensitive. Treat the bot token like a
credential — scope it minimally, never commit it, rotate if exposed.
