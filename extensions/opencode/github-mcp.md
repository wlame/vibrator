---
name: GitHub MCP
kind: mcp
default: false
size_mb: 10
category: version-control
deps:
  features: [node]
auth:
  env: GITHUB_PERSONAL_ACCESS_TOKEN
runtime_needs:
  third_party_api: GitHub
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --arg name "github" \
     --argjson entry '{"type":"local","command":["npx","-y","@modelcontextprotocol/server-github"],"environment":{"GITHUB_PERSONAL_ACCESS_TOKEN":"'"${GITHUB_PERSONAL_ACCESS_TOKEN:-}"'"},"enabled":true}' \
     '.mcp[$name] = $entry' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","mcp":{"github":{"type":"local","command":["npx","-y","@modelcontextprotocol/server-github"],"environment":{"GITHUB_PERSONAL_ACCESS_TOKEN":"'"${GITHUB_PERSONAL_ACCESS_TOKEN:-}"'"},"enabled":true}}}' > "$HOME/.config/opencode/config.json"
source: https://github.com/modelcontextprotocol/servers/tree/main/src/github
---

# GitHub MCP

Adds first-class GitHub access to OpenCode: search repos and code,
read/create/comment on issues and pull requests, push files, manage
releases, browse user/org metadata. Useful when the agent is doing
cross-repo refactors, triaging issues, or assembling a PR review.

## Auth

Set `GITHUB_PERSONAL_ACCESS_TOKEN` in the container environment before
launching opencode. A fine-grained PAT with `repo` and `read:org` scopes
is sufficient for most workflows; broader scopes (write, admin) let the
agent perform releases and webhook management.

## Token-budget warning

The official OpenCode docs explicitly call this MCP out as
**token-heavy** — listing issues and walking PR threads pulls a lot of
markup into context. Gate behind per-agent `permission` rules and only
enable for sessions that need it.

Two ways to scope:
- Set `enabled: false` by default and let users flip per session.
- Use OpenCode `agent.permission` to require ask-confirmation on the
  noisy verbs (`list_issues`, `search_code`, `get_pull_request_files`).

## Why off by default

Requires user-supplied PAT plus the token-cost downside above. Easy to
flip on once you have credentials configured.
