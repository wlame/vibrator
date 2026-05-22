---
name: Notion MCP
kind: mcp
default: false
size_mb: 8
category: project-management
host_aliases: [notion]
deps:
  features: [node]
auth:
  env: NOTION_API_KEY
runtime_needs:
  third_party_api: "Notion"
  outbound_net: true
install: |
  # Notion's official MCP server. stdio transport via npx — the
  # NOTION_API_KEY env var carries an internal integration token
  # (Notion calls these "internal integrations"; mint at
  # https://www.notion.so/profile/integrations).
  claude mcp add notion \
    --scope user \
    --transport stdio \
    -- npx -y @notionhq/notion-mcp-server
source: https://github.com/makenotion/notion-mcp-server
---

# Notion MCP

Notion's official MCP server. Search pages, create / update docs,
manage databases, append blocks, query workspace content — the full
Notion API surface as tool calls.

## Auth

Mint an internal integration token at <https://www.notion.so/profile/integrations>
and export it as `NOTION_API_KEY`. The integration also needs to be
explicitly granted access to each page / database you want Claude to
touch — Notion's permission model is opt-in per page, not workspace-wide.

That share-each-page step trips people up: if Claude says "I can't find
the page", it almost always means the integration wasn't added to that
page's share list.

## Why opt-in

Requires manual token minting and per-page permission grants. Default
off so users only enable it after they've done the Notion-side setup.

## Workflow

Common uses: pull a spec from a Notion page into context, append
meeting notes to a journal database, search across a research workspace
for prior decisions. The plugin marketplace also has `notion@claude-plugins-official`
which bundles this MCP with workflow skills — worth a look if you live
in Notion.
