---
name: DeepSeek provider
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: DEEPSEEK_API_KEY
runtime_needs:
  third_party_api: "DeepSeek"
  outbound_net: true
install: |
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/deepseek.json <<'JSON'
  {
    "name": "deepseek",
    "api": "openai-completions",
    "baseUrl": "https://api.deepseek.com/v1",
    "apiKey": "$DEEPSEEK_API_KEY",
    "models": [
      { "id": "deepseek-chat",     "name": "DeepSeek V3" },
      { "id": "deepseek-reasoner", "name": "DeepSeek R1" },
      { "id": "deepseek-coder",    "name": "DeepSeek Coder V2" }
    ]
  }
  JSON
source: https://api-docs.deepseek.com/
---

# DeepSeek provider

DeepSeek's V3 / R1 / Coder lineup at the canonical low-cost OSS-class
price point. OpenAI-compatible API.

## Models

- `deepseek-chat` — DeepSeek V3, general purpose
- `deepseek-reasoner` — R1 reasoning model
- `deepseek-coder` — coding-specialised

## Use cases

- **Aggressive cost optimisation** — DeepSeek's per-token pricing is
  consistently the lowest among capable models
- **Subagent execution** — let the Explore / Plan agents in
  `pi-subagents` use DeepSeek while the parent stays on Sonnet
- **Council debate** — use as a third opinion in
  `n-r-w/pi-agent-suite/convene-council`

## Synthetic provider alternative

`@benvargas/pi-synthetic-provider` bundles DeepSeek with Kimi, GLM,
MiniMax, and Qwen into a single OpenAI-compatible endpoint. If you want
several of these, use the synthetic provider instead of registering
them one by one.

## Default off

Off by default. Enable when you want DeepSeek as a routing destination.
