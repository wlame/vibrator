---
name: "Profile: Backend (Go + Postgres)"
kind: plugin
default: false
size_mb: 180
category: harness-specific
deps:
  features: [go, node, python]
auth:
  env: POSTGRES_CONNECTION_STRING
install: |
  # Suggested archetype — Go backend with Postgres. Heavy on code
  # intelligence and SQL safety.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. Postgres + GitHub MCPs
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  data.mcpServers.postgres = {
    command: 'npx', args: ['-y', '@modelcontextprotocol/server-postgres', '${POSTGRES_CONNECTION_STRING}']
  };
  data.mcpServers.github = {
    command: 'npx', args: ['-y', '@modelcontextprotocol/server-github'],
    env: { GITHUB_PERSONAL_ACCESS_TOKEN: '${GITHUB_PERSONAL_ACCESS_TOKEN}' }
  };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 3. Serena MCP for semantic Go code navigation
  pip install --user uv
  python - <<'PY'
  import json, pathlib
  cfg = pathlib.Path.home() / ".pi/agent/mcp.json"
  data = json.loads(cfg.read_text())
  data["mcpServers"]["serena"] = {
      "command": "uvx",
      "args": ["--from", "git+https://github.com/oraios/serena@1d020b96069435310613d07211ced178e1fdaf78", "serena-mcp-server"],
      "directTools": ["find_symbol", "find_referencing_symbols", "get_symbols_overview"]
  }
  cfg.write_text(json.dumps(data, indent=2))
  PY

  # 4. Pi hooks (LSP routed to gopls, checkpoint, permission)
  pi install npm:pi-hooks
  go install golang.org/x/tools/gopls@v0.22.0

  # 5. Skills bundle — commit, github, update-changelog, tmux
  pi install git:github.com/mitsuhiko/agent-stuff

  # 6. Tool audit log (catches accidental psql/migrate/goose writes)
  pi install git:github.com/kcosr/pi-extensions

  # 7. SCIP for compiler-accurate cross-file refs
  pi install git:github.com/qualisero/rhubarb-pi

source: https://github.com/oraios/serena
---

# Profile: Backend (Go + Postgres)

Pre-curated Pi stack for engineers building Go services backed by
Postgres. Trades flash for safety — heavy on code intelligence, SQL
guardrails, and tool auditing.

## What's installed

| Layer            | Package                                              |
|------------------|------------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                     |
| Database (MCP)   | `@modelcontextprotocol/server-postgres` (read-only)  |
| GitHub (MCP)     | `@modelcontextprotocol/server-github`                |
| Semantic search  | `serena-mcp-server` via uvx                          |
| LSP + hooks      | `pi-hooks` routed to `gopls`                         |
| Skills           | `agent-stuff` (commit/github/update-changelog/tmux)  |
| Audit            | `kcosr/pi-extensions/toolwatch` (SQLite log)         |
| Code nav         | `qualisero/rhubarb-pi/pi-agent-scip`                 |

## Required env vars

- `POSTGRES_CONNECTION_STRING` — libpq URL for the Postgres MCP
- `GITHUB_PERSONAL_ACCESS_TOKEN` — for the GitHub MCP

Vibrator's wizard will prompt for both.

## Postgres safety

The MCP server is configured **read-only**. The model can introspect
schema and run `SELECT` queries; writes always happen through code and
migration tooling (`goose`, `golang-migrate`, etc.) audited by
`toolwatch`.

## Subagent-friendly

Pair with `pi-subagents` (in the catalogue separately) for
worktree-parallel migration drafting + integration tests.
