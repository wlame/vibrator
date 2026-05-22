---
name: OpenAI provider auth
kind: tool
default: false
size_mb: 0
category: ai-integration
auth:
  env: OPENAI_API_KEY
runtime_needs:
  third_party_api: OpenAI
  outbound_net: true
install: |
  # No install step — OpenCode picks up OPENAI_API_KEY automatically
  # via the @ai-sdk/openai provider that ships with the binary.
  true
source: https://opencode.ai/docs/providers/
---

# OpenAI provider auth

Adds GPT-4.x, GPT-5.x, and the o-series reasoning models as selectable
options inside OpenCode. Useful for tasks where you specifically want
OpenAI's models, or to A/B against Claude on the same prompts.

## How it works

Set `OPENAI_API_KEY` in the container environment. OpenCode's
`@ai-sdk/openai` provider registers the model list on launch. Pick one
with `/model`:

```
/model openai/gpt-5
```

To pin it globally, edit `~/.config/opencode/config.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "openai/gpt-5"
}
```

## Subscription alternative

If you have ChatGPT Plus/Pro/Codex, you can avoid API spend entirely
with the `opencode-openai-codex-auth` plugin (OAuth flow). Trade-off:
the API key path supports more models and higher rate limits.

## Why off by default

Most users pick one primary provider; Anthropic is the more common
default. Flip this on if OpenAI is your primary or if you switch
between providers mid-session.
