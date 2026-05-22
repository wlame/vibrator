---
name: Anthropic provider auth
kind: tool
default: true
size_mb: 0
category: ai-integration
auth:
  env: ANTHROPIC_API_KEY
runtime_needs:
  third_party_api: Anthropic
  outbound_net: true
install: |
  # No install step — OpenCode picks up ANTHROPIC_API_KEY automatically
  # via the @ai-sdk/anthropic provider that ships with the binary.
  true
source: https://opencode.ai/docs/providers/
---

# Anthropic provider auth

OpenCode is provider-agnostic — it speaks to 75+ model providers via
the Vercel AI SDK and Models.dev. The most common starting point is
direct Anthropic API access: Claude Opus 4.x, Sonnet 4.x, Haiku 4.x.

## How it works

Set `ANTHROPIC_API_KEY` in the container environment. On launch,
OpenCode's `@ai-sdk/anthropic` provider checks the env var and
auto-registers every Claude model. Pick one with `/model`:

```
/model anthropic/claude-opus-4-6
```

Or pin it in `~/.config/opencode/config.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "anthropic/claude-opus-4-6"
}
```

## Alternatives

If you prefer a paid subscription auth (no API credits), see
`copilot-auth.md` for GitHub Copilot, or check the OpenCode docs for
the various OAuth plugins (Codex Plus/Pro, Gemini Plan, Antigravity).
The OpenCode Zen gateway also bundles Claude access without a separate
Anthropic account.

## Why on by default

Anthropic is the most common BYOK target for OpenCode users in 2026,
and the install is genuinely zero-effort once the env var is present.
The wizard surfaces this entry as a checkbox so users can opt out if
they're driving the harness purely via Copilot, Zen, or a local model.
