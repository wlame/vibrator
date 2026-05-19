---
name: Hugging Face
kind: plugin
default: false
size_mb: 1
auth:
  env: HF_TOKEN
install: |
  codex plugins install hugging-face
source: https://developers.openai.com/codex/plugins
---

# Hugging Face (Codex official)

Browse models, datasets, and spaces on Hugging Face from Codex.

## Auth

Set `HF_TOKEN` (Hugging Face access token from
[huggingface.co/settings/tokens](https://huggingface.co/settings/tokens)).
Read scope is sufficient for browsing; write scope only if you'll push
models or datasets.

## Common workflows

- "Find me a quantized version of Llama-3 8B that fits in 12GB VRAM"
- "List the most-downloaded BERT variants this month"
- "Compare two embedding models on the MTEB benchmark"

Default = off; opt-in for ML practitioners.
