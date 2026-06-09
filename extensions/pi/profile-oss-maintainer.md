---
name: "Profile: OSS maintainer"
kind: plugin
default: false
size_mb: 150
category: version-control
deps:
  features: [node, gh]
auth:
  env: GITHUB_PERSONAL_ACCESS_TOKEN
install: |
  # Suggested archetype — GitHub-heavy OSS maintainer dealing with
  # issues, PRs, releases, changelogs.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. GitHub + Linear MCPs
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.github = {
    command: 'npx', args: ['-y', '@modelcontextprotocol/server-github'],
    env: { GITHUB_PERSONAL_ACCESS_TOKEN: '${GITHUB_PERSONAL_ACCESS_TOKEN}' },
    directTools: ['search_repositories', 'get_file_contents', 'create_or_update_file', 'create_pull_request']
  };
  data.mcpServers.linear = {
    command: 'npx', args: ['-y', 'linear-mcp@1.2.0'],
    env: { LINEAR_API_KEY: '${LINEAR_API_KEY}' }
  };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 3. PSPDFKit-labs/pi-skills — github/github-repo-search/gh-address-comments/multi-review
  pi install git:github.com/PSPDFKit-labs/pi-skills

  # 4. agent-stuff — commit (gitmoji/conventional), update-changelog, pi-share
  pi install git:github.com/mitsuhiko/agent-stuff

  # 5. PiSwarm — parallel issue/PR processing via git worktrees
  git clone https://github.com/lsj5031/PiSwarm.git ~/.pi/agent/piswarm || true
  echo 'export PATH=~/.pi/agent/piswarm/bin:$PATH' >> ~/.bashrc

  # 6. Rewind hook — safe checkpoints across PRs
  pi install npm:pi-rewind-hook

  # 7. tab-status to track parallel runs
  pi install git:github.com/tmustier/pi-extensions

  # 8. semantic-release helper — Suggested archetype (not Pi-specific)
  npm install -g semantic-release @semantic-release/changelog @semantic-release/git

source: https://github.com/PSPDFKit-labs/pi-skills
---

# Profile: OSS maintainer

Pre-curated Pi stack for maintainers running GitHub-heavy repos —
inbound issues, PR triage, releases, changelogs.

## What's installed

| Layer            | Package                                              |
|------------------|------------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                     |
| GitHub (MCP)     | `@modelcontextprotocol/server-github`                |
| Linear (MCP)     | `linear-mcp`                                         |
| GitHub skills    | `PSPDFKit-labs/pi-skills` (github, repo-search,      |
|                  |   gh-address-comments, multi-review)                 |
| Commits          | `agent-stuff/commit` (gitmoji + conventional)        |
| Changelogs       | `agent-stuff/update-changelog`                       |
| Share sessions   | `agent-stuff/pi-share`                               |
| Parallel triage  | `PiSwarm` (Commander → Captain → Swarm)              |
| Checkpoints      | `pi-rewind-hook` (safe rollback across PRs)          |
| Status           | `tmustier/pi-extensions/tab-status`                  |
| Release tooling  | `semantic-release` + plugins                         |

## Required env vars

- `GITHUB_PERSONAL_ACCESS_TOKEN` — `repo` + `workflow` + `read:org`
- `LINEAR_API_KEY` — only if you mirror issues from Linear

## PR workflow

1. `pi-skills/github` skill walks through open PRs, prioritises by
   labels / assignment / staleness
2. `gh-address-comments` skill lands atomic commits per reviewer
   comment — one-comment-per-commit hygiene without manual splitting
3. `multi-review` skill spawns three sub-agents (each on a different
   model) to do a parallel multi-perspective review — useful when a
   single model misses things

## Parallel triage with PiSwarm

`PiSwarm` is shell-based, not a Pi extension. It uses `gh` + `jq` +
your Pi binary to:

1. Pull a list of open issues / PRs needing attention
2. Spawn isolated git worktrees (one per item)
3. Launch a Pi subprocess per worktree to draft a response / fix
4. Aggregate into a markdown summary

Run with `piswarm dispatch --type=issues --limit=10`.

## Release workflow

`semantic-release` is the standard JS-ecosystem release tool. The
profile installs it globally so it's available in any project that
wires it up. Pi can drive `semantic-release` invocations directly via
bash — no MCP needed.
