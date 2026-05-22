---
name: OpenAI provider
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: OPENAI_API_KEY
runtime_needs:
  third_party_api: "OpenAI"
  outbound_net: true
install: |
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/openai.json <<'JSON'
  {
    "name": "openai",
    "api": "openai-responses",
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "$OPENAI_API_KEY",
    "models": [
      { "id": "gpt-5",      "name": "GPT-5" },
      { "id": "gpt-5-mini", "name": "GPT-5 mini" },
      { "id": "o3",         "name": "o3" },
      { "id": "o3-mini",    "name": "o3-mini" }
    ],
    "compat": {
      "supportsReasoningEffort": true,
      "thinkingFormat": "openai"
    }
  }
  JSON
source: https://platform.openai.com/docs/api-reference
---

# OpenAI provider

OpenAI is a first-class built-in provider in Pi. Uses the `openai-responses`
API — the newer streaming + reasoning-effort variant.

## Models registered

- `gpt-5` — flagship
- `gpt-5-mini` — cheap routing target
- `o3` — heavy reasoning
- `o3-mini` — fast reasoning

## Auth

Standard `OPENAI_API_KEY`. For ChatGPT Plus / Pro / Codex
subscriptions, use the OAuth flow via `/login` in Pi — Pi has built-in
OAuth for the ChatGPT subscriber path.

## Priority tier

For paid priority service tier, install `@benvargas/pi-openai-fast`
which adds a `/fast` toggle.

## Companion: codex-usage

If you're on a ChatGPT subscription, also enable
`@narumitw/pi-codex-usage` to see remaining quota in the footer.

## Default off

Off by default because we ship Anthropic as the primary provider in
vibrator. Enable when you want OpenAI as primary or for model routing
with `--models openai/gpt-5,anthropic/claude-sonnet-4-5`.
