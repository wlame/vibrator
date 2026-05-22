---
name: Groq provider
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: GROQ_API_KEY
runtime_needs:
  third_party_api: "Groq"
  outbound_net: true
install: |
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/groq.json <<'JSON'
  {
    "name": "groq",
    "api": "openai-completions",
    "baseUrl": "https://api.groq.com/openai/v1",
    "apiKey": "$GROQ_API_KEY",
    "models": [
      { "id": "llama-3.3-70b-versatile",   "name": "Llama 3.3 70B" },
      { "id": "llama-3.1-8b-instant",      "name": "Llama 3.1 8B (instant)" },
      { "id": "mixtral-8x7b-32768",        "name": "Mixtral 8x7B" },
      { "id": "qwen-2.5-coder-32b",        "name": "Qwen 2.5 Coder 32B" }
    ]
  }
  JSON
source: https://console.groq.com/docs/api-reference
---

# Groq provider

Groq runs open-source models on custom LPU silicon — typical throughput
is 500–800 tokens/s, an order of magnitude faster than GPU-hosted
alternatives. Pi consumes Groq via its OpenAI-compatible API.

## Models

- `llama-3.3-70b-versatile` — general-purpose, smart
- `llama-3.1-8b-instant` — extremely cheap, very fast
- `mixtral-8x7b-32768` — MoE alternative
- `qwen-2.5-coder-32b` — coding-tuned

## Best uses for Groq

- **Scoped-model routing**: `pi --models groq/llama-3.3-70b-versatile,
  anthropic/claude-sonnet-4-5` and `Ctrl+P` to swap mid-session
- **Subagent execution**: as the Explore/Plan agent model in
  `pi-subagents` for cheap parallel work
- **Speedreading transcript-style outputs** where TPS matters more
  than reasoning quality

## Speech-to-text

Groq also hosts Whisper for transcription. `badlogic/pi-skills/transcribe`
uses this if `GROQ_API_KEY` is set.

## Default off

Off by default. Enable when you specifically want a fast/cheap routing
destination.
