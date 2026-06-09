---
name: Serena MCP (via pi-mcp-adapter)
kind: mcp
default: true
size_mb: 80
category: code-intelligence
deps:
  features: [python, node]
install: |
  # Serena (oraios/serena) provides semantic code retrieval and editing
  # via LSP-backed MCP tools. Pi consumes it through pi-mcp-adapter.
  # Requires pi-mcp-adapter to already be installed (default in vibrator).
  pip install --user uv
  uvx --from git+https://github.com/oraios/serena@1d020b96069435310613d07211ced178e1fdaf78 serena-mcp-server --help >/dev/null
  # Register with pi-mcp-adapter
  mkdir -p ~/.pi/agent
  python - <<'PY'
  import json, os, pathlib
  cfg = pathlib.Path.home() / ".pi/agent/mcp.json"
  data = json.loads(cfg.read_text()) if cfg.exists() else {"mcpServers": {}}
  data.setdefault("mcpServers", {})
  data["mcpServers"]["serena"] = {
      "command": "uvx",
      "args": ["--from", "git+https://github.com/oraios/serena@1d020b96069435310613d07211ced178e1fdaf78", "serena-mcp-server"],
      "directTools": ["find_symbol", "find_referencing_symbols", "get_symbols_overview"]
  }
  cfg.write_text(json.dumps(data, indent=2))
  PY
source: https://github.com/oraios/serena
---

# Serena MCP (via pi-mcp-adapter)

[Serena](https://github.com/oraios/serena) is a free, open-source coding
agent toolkit by `oraios`. It hooks into existing language servers
(pyright, gopls, rust-analyzer, typescript-language-server, etc.) and
exposes symbol-level code retrieval and editing through MCP — the same
operations a human dev gets from "Find Usages" / "Go to Definition" in
their IDE, but driven by the model.

## Why it's high-signal for Pi

Pi's built-in `grep` / `find` are line-based. Serena adds **semantic**
operations:

- `find_symbol`, `find_referencing_symbols`
- `get_symbols_overview` (project + file scopes)
- `replace_symbol_body` (atomic, type-aware edits)
- `insert_before_symbol` / `insert_after_symbol`
- `find_declaration`, `find_implementations`

The model uses these instead of fishing through full file contents,
which both saves tokens and avoids wrong-edit mistakes.

## Direct tools

The install marks three high-value tools as `directTools` so they
register with full names instead of going through the proxy. The rest
stay behind `pi-mcp-adapter`'s 200-token proxy tool.

## Per-project priming

After install, Serena indexes the project on first activation. Expect
a 10-60s lag on first invocation depending on project size; subsequent
invocations are fast.

## Default on

Because Serena's value is so disproportionate — semantic search beats
ripgrep for any non-trivial refactor — vibrator turns it on by default
for Pi profiles that include `pi-mcp-adapter`.
