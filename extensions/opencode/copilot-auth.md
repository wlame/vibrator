---
name: GitHub Copilot auth
kind: tool
default: false
size_mb: 0
install: |
  # No install step — uses opencode's /connect flow. Documenting here so
  # the wizard can surface it as a checkable option.
  echo "Run /connect github-copilot inside opencode after first launch."
source: https://opencode.ai/docs/providers/
---

# GitHub Copilot auth

OpenCode supports authenticating to providers via paid subscriptions
including **GitHub Copilot**. GitHub's official Copilot partnership
(January 2026) lets all paid Copilot subscribers authenticate directly
into OpenCode without separate API keys.

## How

After launching opencode inside the container:

```
/connect github-copilot
```

The auth flow opens a browser via `~/.codex/auth.json` style OAuth and
persists tokens to `~/.local/share/opencode/auth.json`. Vibrator mounts
that path read-write from the host so the auth persists across runs.

## Why off by default

Most users default to bring-your-own-key models (Claude / GPT / Gemini via
their own API keys). Enable specifically if you want to drive opencode
through your Copilot subscription.
