---
name: Local model via LM Studio
kind: tool
default: false
size_mb: 0
install: |
  # No install step — LM Studio runs on the host; pi connects to it via
  # OpenAI-compatible HTTP endpoint. Documenting here so the wizard
  # surfaces it as an option.
  echo "Configure pi to point at http://host.docker.internal:1234/v1 (LM Studio default port)."
source: https://lmstudio.ai/
---

# Local model via LM Studio

Pi is BYOK (bring-your-own-key) and provider-agnostic. A popular 2026
workflow is running models fully locally: **LM Studio on the host +
pi-coding-agent in the container + Gemma 4 26B A4B (Q4_K_M)**.

## Setup

1. Install LM Studio on the host.
2. Load a model (Gemma 4 is the popular choice; Qwen 2.5 Coder is another).
3. Start the local server — default port `1234`, OpenAI-compatible endpoint.
4. Inside the vibrate container, configure pi:

```bash
pi config set provider openai-compatible
pi config set base-url http://host.docker.internal:1234/v1
pi config set api-key sk-no-key-required
pi config set model <model-id-from-lmstudio>
```

5. Run `pi` normally.

## Why this matters

Keeps your source code from ever touching a public third-party API. The
single most important feature for regulated-industry buyers per the
upstream documentation.

Default = off. Enable explicitly when you intend to run locally.
