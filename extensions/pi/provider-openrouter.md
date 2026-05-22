---
name: OpenRouter provider
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: OPENROUTER_API_KEY
runtime_needs:
  third_party_api: "OpenRouter"
  outbound_net: true
install: |
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/openrouter.json <<'JSON'
  {
    "name": "openrouter",
    "api": "openai-completions",
    "baseUrl": "https://openrouter.ai/api/v1",
    "apiKey": "$OPENROUTER_API_KEY",
    "models": [
      { "id": "anthropic/claude-sonnet-4-5",    "name": "Claude Sonnet 4.5 (via OR)" },
      { "id": "openai/gpt-5",                   "name": "GPT-5 (via OR)" },
      { "id": "google/gemini-3-pro",            "name": "Gemini 3 Pro (via OR)" },
      { "id": "deepseek/deepseek-r1",           "name": "DeepSeek R1 (via OR)" },
      { "id": "meta-llama/llama-3.3-70b",       "name": "Llama 3.3 70B (via OR)" }
    ],
    "compat": {
      "openRouterRouting": {
        "order": ["Anthropic", "OpenAI", "Google"],
        "allow_fallbacks": true
      }
    }
  }
  JSON
source: https://openrouter.ai/docs
---

# OpenRouter provider

[OpenRouter](https://openrouter.ai/) is a multi-model proxy — one API
key, hundreds of models. Pi has special support: `compat.openRouterRouting`
lets you configure provider preference order, fallbacks, throughput
hints, and data-policy filters directly in `models.json`.

## Why use it

- **Single billing surface** for trying / mixing many models
- **Failover** when a provider has incident or rate limit
- **Privacy-policy filtering** — OpenRouter knows which underlying
  providers retain or train on data; filter to "noTraining" providers
  for sensitive work
- **No separate API keys** for OpenAI / Anthropic / Google / DeepSeek

## Routing config

The `compat.openRouterRouting` block in the install lets you say "try
Anthropic, then OpenAI, then Google". OpenRouter charges a small
markup vs going direct; if cost-sensitive, register the underlying
providers individually instead.

## Default off

Off by default. Best for users who want maximum flexibility without
managing many keys.
