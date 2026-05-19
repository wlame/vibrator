---
name: Notion
kind: plugin
default: false
size_mb: 1
auth:
  env: NOTION_API_KEY
install: |
  codex plugins install notion
source: https://developers.openai.com/codex/plugins
---

# Notion (Codex official)

Read, search, and update Notion pages and databases from Codex.

## Auth

Notion uses integration tokens. Create an internal integration at
[notion.so/my-integrations](https://www.notion.so/my-integrations), then
share the relevant pages/databases with the integration. Set the resulting
secret as `NOTION_API_KEY` on the host.

## Common workflows

- "Append today's standup notes to the Engineering page"
- "Search Notion for design docs mentioning <topic>"
- "Sync this issue's status to the Roadmap database"

Default = off; opt-in based on team's docs platform.
