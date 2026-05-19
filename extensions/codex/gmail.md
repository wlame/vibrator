---
name: Gmail
kind: plugin
default: false
size_mb: 1
install: |
  # Gmail uses OAuth — Codex prompts on first use and stores credentials
  # in ~/.codex/auth.json (mounted from host by vibrator).
  codex plugins install gmail
source: https://developers.openai.com/codex/plugins
---

# Gmail (Codex official)

Read, search, and compose emails from Codex.

## Auth

OAuth-based. Codex opens a browser window for Google sign-in on first use
and persists the refresh token to `~/.codex/auth.json` — which vibrator
mounts read-write from the host, so OAuth re-auth flows persist back.

No env var needed.

## Workflow caveats

- Composing-and-sending without explicit user confirmation is risky.
  Recommend running with the in-Codex "ask before send" toggle on.
- Read-only access ("summarize my unread from <sender>") is generally safe.

Default = off — opt-in personal-productivity feature.
