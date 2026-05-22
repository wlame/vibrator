---
name: Brave Search MCP
kind: mcp
default: false
size_mb: 2
category: web-browser
deps:
  features: [node]
auth:
  env: BRAVE_API_KEY
runtime_needs:
  third_party_api: Brave
  outbound_net: true
install: |
  # Stdio transport. The npm package reads the API key from BRAVE_API_KEY
  # in the MCP server's process env, which Codex passes through.
  codex mcp add brave-search --env BRAVE_API_KEY="$BRAVE_API_KEY" -- npx -y @modelcontextprotocol/server-brave-search
source: https://github.com/modelcontextprotocol/servers/tree/main/src/brave-search
host_aliases: [brave-search, brave]
---

# Brave Search MCP

Anthropic's official MCP for Brave Search. Exposes:

- web search (general queries, news, articles) with pagination and
  freshness filters
- local search for businesses and points of interest

Codex has native browsing built in, so this MCP is most useful when you
want a **specific search provider** (Brave is a privacy-forward,
independently-indexed alternative to Google/Bing) or when you need
programmatic, rate-limited access from the agent.

## Auth

Free tier API keys are available at
[brave.com/search/api](https://brave.com/search/api/). Set
`BRAVE_API_KEY` on the host; vibrator forwards it into the container's
MCP launch env.

## Why opt-in

Search is already covered by Codex's built-ins. Enable only when:

- You want answers grounded in Brave's index specifically
- You need an audit trail of explicit search tool calls
- You're working under a privacy posture that rules out Google/Bing
