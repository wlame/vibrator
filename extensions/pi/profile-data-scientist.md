---
name: "Profile: Data scientist (Python / Jupyter)"
kind: plugin
default: false
size_mb: 220
category: harness-specific
deps:
  features: [python, node]
install: |
  # Suggested archetype — Python data scientist: Jupyter / pandas /
  # matplotlib. Tuned for notebook-heavy, exploration-heavy workflows.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. uv-based Python tooling
  pip install --user uv

  # 3. Skills: uv management, summarize, librarian (clone repos), tmux
  pi install git:github.com/mitsuhiko/agent-stuff

  # 4. uv extension (intercepted pip/poetry → uv rewrite)
  # included in agent-stuff but also expose intercepted-commands
  if [ -d "$HOME/.pi/agent/git/github.com/mitsuhiko/agent-stuff/intercepted-commands" ]; then
    export PATH="$HOME/.pi/agent/git/github.com/mitsuhiko/agent-stuff/intercepted-commands:$PATH"
  fi

  # 5. LSP routed to Ruff + ty (narumitw's per-extension routing)
  pi install npm:@narumitw/pi-lsp
  uv tool install ruff
  uv tool install ty || true

  # 6. Web search + scraping for paper/blog research
  pi install npm:@benvargas/pi-firecrawl
  pi install npm:pi-amplike

  # 7. Rewind: notebooks have heavy state, so per-turn checkpoints matter
  pi install npm:pi-rewind-hook

  # 8. Dynamic context pruning for long EDA sessions
  git clone https://github.com/zenobi-us/pi-dcp.git ~/.pi/agent/extensions/pi-dcp || true

  # 9. Serena MCP for navigating data pipelines / library source
  python - <<'PY'
  import json, pathlib
  cfg = pathlib.Path.home() / ".pi/agent/mcp.json"
  data = json.loads(cfg.read_text()) if cfg.exists() else {"mcpServers": {}}
  data.setdefault("mcpServers", {})
  data["mcpServers"]["serena"] = {
      "command": "uvx",
      "args": ["--from", "git+https://github.com/oraios/serena@1d020b96069435310613d07211ced178e1fdaf78", "serena-mcp-server"]
  }
  cfg.write_text(json.dumps(data, indent=2))
  PY
source: https://github.com/mitsuhiko/agent-stuff
---

# Profile: Data scientist (Python / Jupyter)

Pre-curated Pi stack for Python data work — pandas, matplotlib,
scikit-learn, occasional ML. Tuned for notebook-heavy, exploratory
sessions where state is large and context is the constraint.

## What's installed

| Layer             | Package                                              |
|-------------------|------------------------------------------------------|
| MCP bridge        | `pi-mcp-adapter`                                     |
| Python toolchain  | `uv` (canonical Python package manager)              |
| Linter / types    | `ruff`, `ty` via `uv tool install`                   |
| LSP routing       | `@narumitw/pi-lsp` (Biome/Ruff/ty per-extension)     |
| Skills            | `agent-stuff` (uv, summarize, librarian, tmux)       |
| Web research      | `@benvargas/pi-firecrawl`, `pi-amplike`              |
| Code intelligence | `serena-mcp-server`                                  |
| Rewind            | `pi-rewind-hook` (notebooks = heavy state)           |
| Context pruning   | `pi-dcp` (dedup, supersede, error-purge)             |

## Why uv

The user's global Python tooling rule says **always use uv, never pip**.
This profile honours that:

- `pip install --user uv` bootstraps once
- `agent-stuff/intercepted-commands` rewrites `pip` / `poetry` /
  `python` invocations to their uv equivalents

## Web research

Two web tools because data scientists need different things at
different times:

- `@benvargas/pi-firecrawl` — best at structured scraping (paper
  abstracts, API docs, leaderboards)
- `pi-amplike` — Jina-powered, best at "webpage as markdown" with
  image extraction

## Context pruning

EDA sessions easily run to 100+ turns. `pi-dcp` deduplicates tool
outputs (same `df.describe()` twice = drop the older copy), supersedes
old file writes, and protects the last N messages.
