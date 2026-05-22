---
name: Ollama (host-local) provider
kind: tool
default: false
size_mb: 0
category: ai-integration
runtime_needs:
  outbound_net: true
install: |
  # Ollama runs on the host; the vibrator container reaches it at
  # host.docker.internal:11434. This entry only writes the Pi provider
  # snippet — install Ollama on the host yourself.
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/ollama.json <<'JSON'
  {
    "name": "ollama",
    "api": "openai-completions",
    "baseUrl": "http://host.docker.internal:11434/v1",
    "apiKey": "sk-no-key-required",
    "models": [
      { "id": "qwen2.5-coder:32b",   "name": "Qwen 2.5 Coder 32B (local)" },
      { "id": "llama3.3:70b",        "name": "Llama 3.3 70B (local)" },
      { "id": "gemma3:27b",          "name": "Gemma 3 27B (local)" },
      { "id": "deepseek-r1:32b",     "name": "DeepSeek R1 32B (local)" }
    ]
  }
  JSON
  echo "Ollama provider configured. Install Ollama on the host and pull models."
  echo "Host setup: https://ollama.com/download — then 'ollama pull qwen2.5-coder:32b'"
source: https://ollama.com/
---

# Ollama (host-local) provider

Run models entirely on your own hardware via [Ollama](https://ollama.com/).
The vibrator container reaches the host's Ollama instance at
`host.docker.internal:11434`. Pi treats Ollama as an OpenAI-compatible
provider (no extension needed).

## Why local

- **Zero data leaves your machine** — single most important property
  for regulated industries
- **No per-token cost** — fixed hardware investment, unlimited usage
- **Works offline** — set `PI_OFFLINE=1` and Pi won't try to reach
  any external endpoint

## Hardware reality

For coding-quality at speed:

| Model                | Min RAM       | Realistic on M-series? |
|----------------------|---------------|------------------------|
| `qwen2.5-coder:7b`   | 8 GB          | Yes, M1 fine           |
| `qwen2.5-coder:14b`  | 16 GB         | M2 Pro / M3            |
| `qwen2.5-coder:32b`  | 32 GB unified | M3 Pro / M3 Max        |
| `llama3.3:70b`       | 48 GB unified | M3 Max 64 GB / M4 Pro  |

## Host setup

```bash
# On the host (not inside the container):
curl -fsSL https://ollama.com/install.sh | sh
ollama pull qwen2.5-coder:32b
ollama serve  # usually already running as a service
```

## Alternative: LM Studio

For a GUI-driven local model setup with model browsing and gentler
hardware tuning, see the `local-model-via-lmstudio` entry in this
catalogue. Functionally equivalent — Pi connects the same way.

## Default off

Off by default — only enable if you have Ollama installed on the host.
