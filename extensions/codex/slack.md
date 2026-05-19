---
name: Slack
kind: plugin
default: false
size_mb: 1
auth:
  env: SLACK_BOT_TOKEN
install: |
  codex plugins install slack
source: https://developers.openai.com/codex/plugins
---

# Slack (Codex official)

Send messages, read threads, manage channels from Codex. Useful for
"announce this release in #releases" or "summarize the last hour of
#oncall" automations.

## Auth

Requires a Slack bot token (`xoxb-...`) with appropriate scopes:
`chat:write`, `channels:history`, `groups:history`, etc.

Create a workspace app at [api.slack.com/apps](https://api.slack.com/apps),
install it to the workspace, copy the bot token, then set
`SLACK_BOT_TOKEN` on the host.

## Risk note

Slack bots can broadcast to channels. Be careful with prompts that include
"send a message to <channel>" — the agent will do exactly that. Consider
running with `--non-interactive` only against test workspaces.
