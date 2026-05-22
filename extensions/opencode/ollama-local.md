---
name: Ollama (local models)
kind: tool
default: false
size_mb: 0
category: ai-integration
runtime_needs:
  outbound_net: true
install: |
  mkdir -p "$HOME/.config/opencode"
  jq --argjson provider '{"ollama":{"npm":"@ai-sdk/openai-compatible","options":{"baseURL":"http://host.docker.internal:11434/v1"},"models":{"qwen2.5-coder:32b":{"name":"Qwen 2.5 Coder 32B"},"llama3.3:70b":{"name":"Llama 3.3 70B"},"deepseek-r1:32b":{"name":"DeepSeek R1 32B"}}}}' \
     '.provider = (.provider // {}) * $provider' \
     "$HOME/.config/opencode/config.json" 2>/dev/null \
     > "$HOME/.config/opencode/config.json.tmp" \
     && mv "$HOME/.config/opencode/config.json.tmp" "$HOME/.config/opencode/config.json" \
     || echo '{"$schema":"https://opencode.ai/config.json","provider":{"ollama":{"npm":"@ai-sdk/openai-compatible","options":{"baseURL":"http://host.docker.internal:11434/v1"},"models":{"qwen2.5-coder:32b":{"name":"Qwen 2.5 Coder 32B"},"llama3.3:70b":{"name":"Llama 3.3 70B"},"deepseek-r1:32b":{"name":"DeepSeek R1 32B"}}}}}' > "$HOME/.config/opencode/config.json"
source: https://ollama.com
---

# Ollama (local models)

Routes OpenCode to a locally-running Ollama daemon on the host
machine. Lets you drive the harness with `qwen2.5-coder`, `llama3.3`,
`deepseek-r1`, or any other model you've pulled — no external API
keys, no per-token billing, no data leaving your machine.

## How it works

OpenCode treats Ollama as a generic OpenAI-compatible provider via
`@ai-sdk/openai-compatible`. The install snippet points the provider
at `http://host.docker.internal:11434/v1`, which is the Docker-on-Linux
bridge to the host (also works on Mac/Windows out of the box).

Pick a model after configuring:

```
/model ollama/qwen2.5-coder:32b
```

## Prerequisites

- Ollama running on the host (`ollama serve` listening on `:11434`).
- The model already pulled host-side (`ollama pull qwen2.5-coder:32b`).
- Docker run with `--add-host=host.docker.internal:host-gateway` on
  Linux, or the equivalent that lets the container reach the host.

## Edit the model list

Drop into `~/.config/opencode/config.json` and add/remove entries
under `provider.ollama.models` to match what you have pulled. The
key is the literal Ollama model tag (`qwen2.5-coder:32b`).

## Why off by default

Requires a host-side Ollama install and pulled weights. Niche for the
"everyone gets a Docker-isolated environment" use case, but very
high-value for users who want offline-capable coding sessions.
