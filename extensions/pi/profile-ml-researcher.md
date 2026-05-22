---
name: "Profile: ML researcher (arxiv / W&B)"
kind: plugin
default: false
size_mb: 200
category: harness-specific
deps:
  features: [python, node]
auth:
  env: WANDB_API_KEY
install: |
  # Suggested archetype — ML researcher reading arxiv, training models,
  # logging to W&B. Heavy on paper / transcript / image tooling.
  set -e

  # 1. MCP foundation
  pi install npm:pi-mcp-adapter

  # 2. uv-based Python tooling
  pip install --user uv

  # 3. arxiv + papers MCPs
  mkdir -p ~/.pi/agent
  node - <<'JS'
  const fs = require('fs'), path = require('path');
  const cfg = path.join(process.env.HOME, '.pi/agent/mcp.json');
  const data = fs.existsSync(cfg) ? JSON.parse(fs.readFileSync(cfg, 'utf8')) : { mcpServers: {} };
  data.mcpServers ||= {};
  # Suggested archetype: arxiv-mcp-server is a community package
  data.mcpServers.arxiv = { command: 'npx', args: ['-y', 'arxiv-mcp-server@latest'] };
  data.mcpServers.fetch = { command: 'npx', args: ['-y', '@modelcontextprotocol/server-fetch'] };
  # W&B MCP — community package, may need substitution if unavailable
  data.mcpServers.wandb = {
    command: 'npx', args: ['-y', 'wandb-mcp-server@latest'],
    env: { WANDB_API_KEY: '${WANDB_API_KEY}' }
  };
  fs.writeFileSync(cfg, JSON.stringify(data, null, 2));
  JS

  # 4. Skills — uv, summarize (papers → markdown), youtube-transcript (talks)
  pi install git:github.com/mitsuhiko/agent-stuff
  pi install git:github.com/badlogic/pi-skills  # transcribe (Groq Whisper)

  # 5. Web search + scraping (Jina-based, image extraction)
  pi install npm:pi-amplike

  # 6. Image generation for figure drafting
  pi install npm:@benvargas/pi-antigravity-image-gen

  # 7. Subagents — heavy parallel paper-reading workflow
  pi install npm:@tintinweb/pi-subagents

  # 8. Notifications for long training runs
  pi install npm:pi-notify

source: https://github.com/mitsuhiko/agent-stuff
---

# Profile: ML researcher (arxiv / W&B)

Pre-curated Pi stack for ML / AI research. Optimised for paper
reading, model experimentation, and rapid figure drafting.

## What's installed

| Layer            | Package                                              |
|------------------|------------------------------------------------------|
| MCP bridge       | `pi-mcp-adapter`                                     |
| Papers (MCP)     | `arxiv-mcp-server` + `server-fetch`                  |
| Experiments      | `wandb-mcp-server` (suggested archetype)             |
| Skills           | `agent-stuff` (uv, summarize)                        |
| Skills           | `badlogic/pi-skills/transcribe` (Whisper)            |
| Web research     | `pi-amplike` (Jina search + extract w/ images)       |
| Image gen        | `@benvargas/pi-antigravity-image-gen` (Gemini 3 Pro) |
| Subagents        | `@tintinweb/pi-subagents`                            |
| Notifications    | `pi-notify` (long training runs)                     |

## Workflow it supports

1. Drop an arxiv URL — `arxiv-mcp-server` pulls + parses, then
   `agent-stuff/summarize` produces a 2-paragraph distillation
2. Pull lecture transcript via `badlogic/pi-skills/transcribe`
   (Groq Whisper API) and summarise
3. Spawn 3-4 `pi-subagents` to read 3-4 related papers in parallel,
   merge findings
4. Sketch a figure idea, generate with `pi-antigravity-image-gen`
   (Gemini 3 Pro produces inline images in the terminal)
5. Run training, log to W&B; the MCP server lets the agent inspect
   runs without leaving Pi

## Caveat: third-party MCP servers

`arxiv-mcp-server` and `wandb-mcp-server` are community packages —
their canonical install / package name may differ from what's
written. Adjust `~/.pi/agent/mcp.json` to point at the package you
use. If you can't find one, fall back to `pi-amplike` for arxiv
(it can extract paper text from arxiv URLs) and `agent-stuff/sentry`
for run inspection.

## Required env vars

- `WANDB_API_KEY` — only if you enable the W&B MCP
- `GROQ_API_KEY` — for `transcribe` skill
- `GEMINI_API_KEY` — for `pi-antigravity-image-gen`
