---
name: TokenScope (token-usage tracker)
kind: plugin
default: false
size_mb: 2
category: observability
deps:
  features: [node]
install: |
  mkdir -p "$HOME/.config/opencode"
  jq '.plugin = ((.plugin // []) + ["@ramtinj95/opencode-tokenscope"] | unique)' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","plugin":["@ramtinj95/opencode-tokenscope"]}' > "$HOME/.config/opencode/config.json"
source: https://github.com/ramtinJ95/opencode-tokenscope
---

# TokenScope (token-usage tracker)

Per-session token and cost tracking for OpenCode. Adds a `/tokenscope`
slash command that walks the current session (and recursively, any
sub-agent child sessions), categorizes tokens by source — system
prompts, user messages, tool outputs, cache hits — and prints a
breakdown plus an estimated cost.

Built around OpenCode's telemetry hooks (`session.idle`,
`tool.execute.after`, etc.), so it sees what the model actually
consumed rather than guessing from the chat log.

## What you get

- Token breakdown by category (system / messages / tools / files).
- Tool-usage breakdown with call counts and percentages.
- Top token-contributing files and tool calls.
- Cache-hit / cache-miss efficiency analysis.
- Cost estimate based on the model in use.

## Alternatives

- **opencode-quota** (slkiser) — quota tracking with toast
  notifications.
- **opencode-mystatus** (vbgate) — one-command view across multiple
  AI subscriptions (OpenAI Plus/Pro/Codex, Zhipu, Antigravity).
- **tokscale** (junhoyeo) — out-of-process CLI that tracks across
  OpenCode, Claude Code, Codex, Cursor, Gemini CLI.

TokenScope is the most "inside OpenCode" of the bunch — slash command,
live data, no external CLI.

## Why off by default

Useful but opt-in: most users won't think about token costs day-to-day.
Enable when you want awareness or are debugging a runaway session.
