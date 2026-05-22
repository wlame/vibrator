---
name: GitHub MCP (via pi-mcp-adapter)
kind: mcp
default: false
size_mb: 5
category: version-control
deps:
  features: [node]
auth:
  env: GITHUB_PERSONAL_ACCESS_TOKEN
runtime_needs:
  third_party_api: "GitHub"
  outbound_net: true
install: |
  # Official @modelcontextprotocol/server-github via pi-mcp-adapter.
  # Two high-value tools (search_repositories, get_file_contents) are
  # registered directly for low latency; everything else stays behind
  # the proxy.
  if [ -z "${GITHUB_PERSONAL_ACCESS_TOKEN:-}" ]; then
    echo "WARN: GITHUB_PERSONAL_ACCESS_TOKEN not set. Server will install but most tools will fail until you set it."
  fi
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs');
  const path = require('path');
  const cfgPath = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfgPath)
    ? JSON.parse(fs.readFileSync(cfgPath, 'utf8'))
    : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.github = {
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-github'],
    env: { GITHUB_PERSONAL_ACCESS_TOKEN: '${GITHUB_PERSONAL_ACCESS_TOKEN}' },
    directTools: ['search_repositories', 'get_file_contents']
  };
  fs.writeFileSync(cfgPath, JSON.stringify(data, null, 2));
  JS
source: https://github.com/modelcontextprotocol/servers/tree/main/src/github
---

# GitHub MCP

Official MCP server for the GitHub API, wired into Pi through
`pi-mcp-adapter`. Lets the model open issues, comment on PRs, read
code, list workflows, and search across the whole of GitHub.

## What it covers

Roughly mirrors the `gh` CLI surface but typed:

- Search repositories / code / issues / PRs / users
- Read file contents at any ref
- Create / update / close issues
- Create / update / merge PRs
- Manage labels, milestones, releases
- Read workflow runs and artifacts

## Auth

Requires a **GitHub Personal Access Token** with the scopes you need.
Set `GITHUB_PERSONAL_ACCESS_TOKEN` in the container environment before
the install script runs — vibrator's wizard will prompt for it.

For OSS work, a classic PAT with `repo` + `read:org` + `workflow` is
typical. For more locked-down setups, prefer a **fine-grained PAT**
scoped to specific repos.

## Direct tools

`search_repositories` and `get_file_contents` register directly (full
token cost — about 1k tokens). They're worth it because they're the
most-used tools and going through the proxy adds round-trips.

## Default off

Off by default because not every vibrator workspace touches GitHub and
the token requirement makes it awkward to ship enabled.
