---
name: Linear MCP
kind: mcp
default: false
size_mb: 1
category: project-management
host_aliases: [linear]
auth:
  env: LINEAR_API_KEY
runtime_needs:
  third_party_api: "Linear"
  outbound_net: true
install: |
  # Linear's hosted MCP server. HTTP transport against the official
  # endpoint — OAuth flow runs inside Claude Code on first /mcp call
  # in the session, no long-lived token needs to be wrangled at the
  # vibrator layer. LINEAR_API_KEY is only used as a fallback for
  # non-interactive setups (CI / agents).
  claude mcp add --transport http linear https://mcp.linear.app/mcp \
    --scope user
source: https://linear.app/docs/mcp
---

# Linear MCP

Linear's official hosted MCP server. Create issues, manage projects,
update statuses, search workspaces, comment on tickets — anything you'd
do in the Linear web UI, but expressed as tool calls Claude can drive.

## Auth

The hosted endpoint authenticates per workspace via OAuth. After
launching the harness, run `/mcp` inside Claude Code — it'll open the
Linear OAuth flow in your browser. There's nothing to mint or manage
manually.

For headless / agent use cases without an interactive browser, set
`LINEAR_API_KEY` and Linear will accept it as a bearer credential.

## Why opt-in

Not everyone uses Linear, and the OAuth handshake needs a real human
the first time. Default-off so the wizard doesn't surface an OAuth
modal you weren't expecting.

## Workflow

Typical pattern: ask Claude to "find the open issue about <bug>",
"create a ticket for this refactor in the `backend` project", or "list
all my issues this sprint". The streaming HTTP transport keeps the
session responsive even on workspaces with thousands of issues.
