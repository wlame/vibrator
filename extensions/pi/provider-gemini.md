---
name: Google Gemini provider
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: GEMINI_API_KEY
runtime_needs:
  third_party_api: "Google"
  outbound_net: true
install: |
  mkdir -p ~/.pi/agent/providers
  cat > ~/.pi/agent/providers/gemini.json <<'JSON'
  {
    "name": "google",
    "api": "google-generative-ai",
    "baseUrl": "https://generativelanguage.googleapis.com",
    "apiKey": "$GEMINI_API_KEY",
    "models": [
      { "id": "gemini-3-pro",      "name": "Gemini 3 Pro" },
      { "id": "gemini-2.5-pro",    "name": "Gemini 2.5 Pro" },
      { "id": "gemini-2.5-flash",  "name": "Gemini 2.5 Flash" }
    ],
    "compat": {
      "supportsReasoningEffort": false,
      "thinkingFormat": "google"
    }
  }
  JSON
source: https://ai.google.dev/gemini-api/docs
---

# Google Gemini provider

Direct Gemini API via the `google-generative-ai` API shape Pi
supports natively.

## Models

- `gemini-3-pro` — flagship multimodal, 2M-context
- `gemini-2.5-pro` — proven workhorse
- `gemini-2.5-flash` — cheap fast model for routing

## When to pick Gemini

- Very large contexts (2M tokens) — Gemini Pro models stay coherent at
  sizes other providers can't match
- Multi-modal — pair with `@benvargas/pi-antigravity-image-gen` for
  inline image generation in the terminal
- Cheap routing destination — `gemini-2.5-flash` is competitive on
  cost-per-token with the cheapest OSS-hosted options

## Vertex AI alternative

For enterprise / GCP-resident workflows, Pi has **built-in** Vertex AI
support — no provider file needed. Use `gcloud auth application-default
login` or a service account, and Pi picks it up via ADC. Configure
project + region in `models.json`.

## Default off

Off by default. Enable when you specifically want Gemini in the mix.
