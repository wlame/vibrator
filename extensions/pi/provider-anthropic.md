---
name: Anthropic Claude provider
kind: tool
default: true
size_mb: 0
category: ai-integration
auth:
  env: ANTHROPIC_API_KEY
runtime_needs:
  third_party_api: "Anthropic"
  outbound_net: true
install: |
  # Pi has Anthropic as a first-class built-in provider. This entry
  # writes the canonical provider snippet so the wizard surfaces it.
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/anthropic.json <<'JSON'
  {
    "name": "anthropic",
    "api": "anthropic-messages",
    "baseUrl": "https://api.anthropic.com",
    "apiKey": "$ANTHROPIC_API_KEY",
    "models": [
      { "id": "claude-sonnet-4-5", "name": "Sonnet 4.5" },
      { "id": "claude-opus-4-1", "name": "Opus 4.1" },
      { "id": "claude-haiku-4-5", "name": "Haiku 4.5" }
    ],
    "compat": {
      "thinkingFormat": "anthropic",
      "supportsReasoningEffort": true
    }
  }
  JSON
source: https://docs.anthropic.com/en/api/
---

# Anthropic Claude provider

Pi treats Anthropic as a first-class provider — no extension needed,
just an entry in `~/.pi/agent/models.json` or `~/.pi/agent/providers/`.
This package writes the canonical config.

## What it enables

- `claude-sonnet-4-5` (Sonnet 4.5) — default recommended model
- `claude-opus-4-1` (Opus 4.1) — heavy reasoning, slower
- `claude-haiku-4-5` (Haiku 4.5) — fast cheap subagent model

Thinking-level shorthand works: `pi --model claude-sonnet-4-5:high`.

## Auth

Set `ANTHROPIC_API_KEY` in the container environment. Pi's `apiKey`
field accepts:

- Literal value: `"sk-ant-..."`
- Env var: `"$ANTHROPIC_API_KEY"`
- Shell command: `"!1password read op://Personal/Anthropic/api_key"`

The wizard prompts for the API key; vibrator stores it in the
container's env and references it by name.

## OAuth alternative

For Claude Pro / Max users, the official `custom-provider-anthropic`
example uses OAuth instead of an API key — falls back to your
Claude.ai subscription. See
`packages/coding-agent/examples/extensions/custom-provider-anthropic.ts`
in the Pi monorepo if you'd rather not use a separate billing line.

## Prompt caching

Auto-enabled for Anthropic models on Amazon Bedrock and AWS-routed
inference. For direct Anthropic, Pi adds cache breakpoints
automatically — no config needed.

Default on. This is the canonical Pi provider.
