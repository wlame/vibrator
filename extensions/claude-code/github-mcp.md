---
name: GitHub MCP
kind: mcp
default: false
size_mb: 25
host_aliases: [github]
deps:
  features: [go]
auth:
  env: GITHUB_PERSONAL_ACCESS_TOKEN
install: |
  # Install the official GitHub MCP server (written in Go, maintained by GitHub).
  GOBIN=/usr/local/bin go install github.com/github/github-mcp-server@v1.2.0
  claude mcp add github \
    --scope user \
    --transport stdio \
    -- github-mcp-server
source: https://github.com/github/github-mcp-server
---

# GitHub MCP

Official GitHub MCP server. Covers the full GitHub API surface from Claude
Code: repos, pull requests, issues, code search, file operations, releases.
Written in Go and maintained by GitHub itself.

## Auth

Requires a GitHub personal access token in `GITHUB_PERSONAL_ACCESS_TOKEN`.
Scope it minimally — `repo` is plenty for most workflows; add `workflow` if
you'll be reading Actions logs.

## Why opt-in (not default)

Needs the user to mint and configure a PAT before it's useful. Vibrator
can't fully verify the token works at launch (only that the var is set),
so we surface this in the wizard and let the user decide.
